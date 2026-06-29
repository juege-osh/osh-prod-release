#!/usr/bin/env bash
set -euo pipefail

BLUE_ENV="${BLUE_ENV:-/opt/osh/001-docker-compose/osh/osh-stack.env}"
GREEN_ENV="${GREEN_ENV:-/opt/osh-green/001-docker-compose/osh/osh-green-stack.env}"
LOG_ROOT="${LOG_ROOT:-/opt/osh-green/004-log/osh/sync}"
TS="$(date +%Y%m%d-%H%M%S)"
WORK="${LOG_ROOT}/incremental-blue-to-green-all-${TS}"
STATUS_FILE="${WORK}/status"
mkdir -p "$WORK"

log() { echo "[$(date '+%F %T')] $*" | tee -a "$WORK/run.log"; }
fail() {
  log "FAILED at line $1"
  echo failed > "$STATUS_FILE"
}
trap 'fail $LINENO' ERR

load_env_value() {
  local file="$1" key="$2"
  awk -F= -v k="$key" '$1 == k {print substr($0, length(k) + 2); exit}' "$file"
}

require_container() {
  local name="$1"
  docker ps --format '{{.Names}}' | grep -qx "$name" || {
    log "missing container: $name"
    return 1
  }
}

container_exists() {
  docker ps --format '{{.Names}}' | grep -qx "$1"
}

port80_owner() {
  docker ps --format '{{.Names}} {{.Ports}}' \
    | awk '/0\.0\.0\.0:80->|:::80->/ {print $1; exit}'
}

MYSQL_PASS="$(load_env_value "$BLUE_ENV" MYSQL_ROOT_PASSWORD)"
GREEN_MYSQL_PASS="$(load_env_value "$GREEN_ENV" MYSQL_ROOT_PASSWORD)"
ES_PASS="$(load_env_value "$BLUE_ENV" ES_PASSWORD)"
GREEN_ES_PASS="$(load_env_value "$GREEN_ENV" ES_PASSWORD)"
REDIS_PASS="$(load_env_value "$BLUE_ENV" REDIS_PASSWORD)"
GREEN_REDIS_PASS="$(load_env_value "$GREEN_ENV" REDIS_PASSWORD)"

MYSQL_PASS="${MYSQL_PASS:?MYSQL_ROOT_PASSWORD missing in blue env}"
GREEN_MYSQL_PASS="${GREEN_MYSQL_PASS:-$MYSQL_PASS}"
ES_PASS="${ES_PASS:?ES_PASSWORD missing in blue env}"
GREEN_ES_PASS="${GREEN_ES_PASS:-$ES_PASS}"
REDIS_PASS="${REDIS_PASS:?REDIS_PASSWORD missing in blue env}"
GREEN_REDIS_PASS="${GREEN_REDIS_PASS:-$REDIS_PASS}"

DBS=(backstage osh_secret xxl_job)
NACOS_TENANTS=(d4208eb8-261f-44a1-adf5-c12406fbd1a2 60a6a2f7-6a4c-40e4-8d4a-94825a47e279)

log "workdir=$WORK"
owner="$(port80_owner || true)"
log "prod_port_80_owner=${owner:-unknown}"
if [[ "$owner" != "osh-nginx" ]]; then
  log "refuse: production is not blue, skip blue-to-green copy"
  echo failed > "$STATUS_FILE"
  exit 1
fi

for c in osh-mysql osh-g-mysql osh-nacos osh-g-nacos osh-redis osh-g-redis osh-es osh-g-es osh-kafka osh-g-kafka; do
  require_container "$c"
done

run_mysql_counts() {
  local container="$1" pass="$2" out="$3"
  python3 - "$container" "$pass" "$out" "${DBS[@]}" <<'PY'
import subprocess
import sys

container, password, out = sys.argv[1], sys.argv[2], sys.argv[3]
dbs = sys.argv[4:]

def mysql(sql):
    p = subprocess.run(
        ["docker", "exec", container, "mysql", "-uroot", "-p" + password, "-N", "-B", "-e", sql],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        universal_newlines=True,
    )
    if p.returncode:
        print(p.stderr, file=sys.stderr)
        raise SystemExit(p.returncode)
    return p.stdout

with open(out, "w") as f:
    for db in dbs:
        tables = [
            line.strip()
            for line in mysql(
                "SELECT table_name FROM information_schema.tables "
                f"WHERE table_schema='{db}' AND table_type='BASE TABLE' ORDER BY table_name"
            ).splitlines()
            if line.strip()
        ]
        for table in tables:
            count = mysql(f"SELECT COUNT(*) FROM `{db}`.`{table}`").strip()
            f.write(f"{db}\t{table}\t{count}\n")
PY
}

sync_mysql() {
  log "mysql: count before"
  run_mysql_counts osh-mysql "$MYSQL_PASS" "$WORK/mysql-blue-before.tsv"
  run_mysql_counts osh-g-mysql "$GREEN_MYSQL_PASS" "$WORK/mysql-green-before.tsv"

  log "mysql: backup green target before upsert"
  docker exec osh-g-mysql mysqldump -uroot -p"${GREEN_MYSQL_PASS}" \
    --single-transaction --routines --triggers --events --databases "${DBS[@]}" \
    > "$WORK/mysql-green-before.sql"
  test -s "$WORK/mysql-green-before.sql"

  log "mysql: generate schema delta blue -> green"
  python3 - "$WORK/mysql-schema-delta.sql" "$MYSQL_PASS" "$GREEN_MYSQL_PASS" "${DBS[@]}" <<'PY'
import re
import subprocess
import sys

out, blue_pwd, green_pwd = sys.argv[1], sys.argv[2], sys.argv[3]
dbs = sys.argv[4:]

def q(name):
    return "`" + name.replace("`", "``") + "`"

def mysql(container, password, sql):
    p = subprocess.run(
        ["docker", "exec", container, "mysql", "-uroot", "-p" + password, "-N", "-B", "-e", sql],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        universal_newlines=True,
    )
    if p.returncode:
        print("ERR", container, sql, p.stderr, file=sys.stderr)
        raise SystemExit(p.returncode)
    return p.stdout

stmts = []
report = []
for db in dbs:
    blue_tables = set(
        line.strip()
        for line in mysql(
            "osh-mysql",
            blue_pwd,
            f"SELECT table_name FROM information_schema.tables WHERE table_schema='{db}' AND table_type='BASE TABLE'",
        ).splitlines()
        if line.strip()
    )
    green_tables = set(
        line.strip()
        for line in mysql(
            "osh-g-mysql",
            green_pwd,
            f"SELECT table_name FROM information_schema.tables WHERE table_schema='{db}' AND table_type='BASE TABLE'",
        ).splitlines()
        if line.strip()
    )
    for table in sorted(blue_tables - green_tables):
        row = mysql("osh-mysql", blue_pwd, f"SHOW CREATE TABLE {q(db)}.{q(table)}").split("\t", 1)
        create = row[1].strip() if len(row) > 1 else ""
        create = re.sub(r"^CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", create, count=1)
        stmts.append(f"USE {q(db)};\n{create};\n")
        report.append(("create_table", db, table, ""))
    for table in sorted(blue_tables & green_tables):
        blue_cols = [
            line.split("\t")
            for line in mysql(
                "osh-mysql",
                blue_pwd,
                f"SELECT column_name, ordinal_position FROM information_schema.columns "
                f"WHERE table_schema='{db}' AND table_name='{table}' ORDER BY ordinal_position",
            ).splitlines()
            if line
        ]
        green_cols = set(
            line.strip()
            for line in mysql(
                "osh-g-mysql",
                green_pwd,
                f"SELECT column_name FROM information_schema.columns WHERE table_schema='{db}' AND table_name='{table}'",
            ).splitlines()
            if line.strip()
        )
        missing = [col for col, _ in blue_cols if col not in green_cols]
        if not missing:
            continue
        create_row = mysql("osh-mysql", blue_pwd, f"SHOW CREATE TABLE {q(db)}.{q(table)}").split("\t", 1)
        create = create_row[1] if len(create_row) > 1 else ""
        col_defs = {}
        for line in create.split("\n"):
            stripped = line.strip().rstrip(",")
            if stripped.startswith("`"):
                name = stripped.split("`", 2)[1]
                col_defs[name] = stripped
        order = [col for col, _ in blue_cols]
        for col in missing:
            definition = col_defs.get(col)
            if not definition:
                continue
            idx = order.index(col)
            prev = None
            for prev_col in reversed(order[:idx]):
                if prev_col in green_cols or prev_col not in missing:
                    prev = prev_col
                    break
            after = f" AFTER {q(prev)}" if prev else " FIRST"
            stmts.append(f"ALTER TABLE {q(db)}.{q(table)} ADD COLUMN {definition}{after};\n")
            report.append(("add_column", db, table, col))
            green_cols.add(col)

with open(out, "w") as f:
    f.write("SET FOREIGN_KEY_CHECKS=0;\n")
    for stmt in stmts:
        f.write(stmt + "\n")
    f.write("SET FOREIGN_KEY_CHECKS=1;\n")
with open(out.replace(".sql", ".tsv"), "w") as f:
    for row in report:
        f.write("\t".join(row) + "\n")
print("schema_changes", len(report))
PY

  log "mysql: apply schema delta"
  docker exec -i osh-g-mysql mysql -uroot -p"${GREEN_MYSQL_PASS}" < "$WORK/mysql-schema-delta.sql"

  log "mysql: dump blue data as REPLACE upsert"
  docker exec osh-mysql mysqldump -uroot -p"${MYSQL_PASS}" \
    --single-transaction --skip-triggers --no-create-info --skip-add-drop-table \
    --complete-insert --replace --databases "${DBS[@]}" \
    > "$WORK/mysql-blue-replace.sql"
  test -s "$WORK/mysql-blue-replace.sql"

  log "mysql: import into green target"
  docker exec -i osh-g-mysql mysql -uroot -p"${GREEN_MYSQL_PASS}" < "$WORK/mysql-blue-replace.sql"

  log "mysql: count after"
  run_mysql_counts osh-g-mysql "$GREEN_MYSQL_PASS" "$WORK/mysql-green-after.tsv"
}

rewrite_for_green() {
  sed \
    -e 's/osh-mysql/osh-g-mysql/g' \
    -e 's/osh-redis/osh-g-redis/g' \
    -e 's/osh-es/osh-g-es/g' \
    -e 's/osh-kafka/osh-g-kafka/g' \
    -e 's/osh-zookeeper/osh-g-zookeeper/g' \
    -e 's/osh-secret-manager/osh-g-secret-manager/g' \
    -e 's/osh-otel-collector/osh-g-otel-collector/g' \
    -e 's/osh-backend/osh-g-backend/g' \
    -e 's/43\.242\.200\.25:53306/osh-g-mysql:3306/g' \
    -e 's/43\.242\.200\.25:56379/osh-g-redis:6379/g' \
    -e 's/43\.242\.200\.25:59200/osh-g-es:9200/g' \
    -e 's/43\.242\.200\.25:59092/osh-g-kafka:9092/g' \
    -e 's|http://127\.0\.0\.1:58081|http://127.0.0.1:28081|g' \
    -e 's|notify-url: http://43\.242\.200\.25:58081|notify-url: http://127.0.0.1:28081|g'
}

sync_nacos() {
  log "nacos: blue -> green with green component rewrite"
  for tenant in "${NACOS_TENANTS[@]}"; do
    items="$(curl -sf "http://127.0.0.1:58848/nacos/v1/cs/configs?tenant=${tenant}&search=accurate&dataId=&group=&pageNo=1&pageSize=500" \
      | python3 -c "import sys,json; d=json.load(sys.stdin); print('\n'.join(f\"{i['dataId']}|{i['group']}\" for i in d.get('pageItems',[])))" || true)"
    [[ -z "$items" ]] && continue
    while IFS='|' read -r data_id group; do
      [[ -z "$data_id" ]] && continue
      content="$(curl -sf "http://127.0.0.1:58848/nacos/v1/cs/configs?tenant=${tenant}&dataId=${data_id}&group=${group}" || true)"
      [[ -z "$content" ]] && continue
      new_content="$(printf '%s' "$content" | rewrite_for_green)"
      curl -sf -X POST "http://127.0.0.1:28848/nacos/v1/cs/configs" \
        --data-urlencode "tenant=${tenant}" \
        --data-urlencode "dataId=${data_id}" \
        --data-urlencode "group=${group}" \
        --data-urlencode "content=${new_content}" \
        --data-urlencode "type=yaml" >/dev/null
      log "nacos: synced ${tenant}/${group}/${data_id}"
    done <<< "$items"
  done
}

sync_redis() {
  log "redis: upsert keys blue -> green"
  local total=0
  for db in $(seq 0 15); do
    while IFS= read -r key; do
      [[ -z "$key" ]] && continue
      ttl="$(docker exec osh-redis redis-cli -n "$db" -a "$REDIS_PASS" --no-auth-warning PTTL "$key" 2>/dev/null || echo -2)"
      [[ "$ttl" == "-2" ]] && continue
      [[ "$ttl" == "-1" ]] && ttl=0
      docker exec osh-redis redis-cli -n "$db" -a "$REDIS_PASS" --no-auth-warning --raw DUMP "$key" \
        | docker exec -i osh-g-redis redis-cli -n "$db" -a "$GREEN_REDIS_PASS" --no-auth-warning -x RESTORE "$key" "$ttl" REPLACE >/dev/null
      total=$((total + 1))
    done < <(docker exec osh-redis redis-cli -n "$db" -a "$REDIS_PASS" --no-auth-warning --scan)
  done
  log "redis: upserted_keys=${total}"
}

sync_es() {
  log "es: upsert osh_* indices blue -> green"
  docker pull elasticdump/elasticsearch-dump:v6.111.0 >/dev/null 2>&1 || true
  local indices
  indices="$(curl -sf -u "elastic:${ES_PASS}" "http://127.0.0.1:59200/_cat/indices/osh_*?h=index" | sort -u || true)"
  [[ -z "$indices" ]] && { log "es: no osh_* indices"; return 0; }
  while IFS= read -r idx; do
    [[ -z "$idx" ]] && continue
    log "es: sync index ${idx}"
    docker run --rm --network host elasticdump/elasticsearch-dump:v6.111.0 \
      --input="http://elastic:${ES_PASS}@127.0.0.1:59200/${idx}" \
      --output="http://elastic:${GREEN_ES_PASS}@127.0.0.1:29200/${idx}" \
      --type=mapping --quiet 2>/dev/null || true
    docker run --rm --network host elasticdump/elasticsearch-dump:v6.111.0 \
      --input="http://elastic:${ES_PASS}@127.0.0.1:59200/${idx}" \
      --output="http://elastic:${GREEN_ES_PASS}@127.0.0.1:29200/${idx}" \
      --type=data --quiet
  done <<< "$indices"
}

kafka_topics() {
  local container="$1" bootstrap="$2"
  docker exec "$container" bash -lc \
    "kafka-topics.sh --bootstrap-server '${bootstrap}' --list"
}

kafka_describe_topic() {
  local container="$1" bootstrap="$2" topic="$3"
  docker exec "$container" bash -lc \
    "kafka-topics.sh --bootstrap-server '${bootstrap}' --describe --topic '$topic'"
}

kafka_create_topic() {
  local topic="$1" partitions="$2"
  docker exec osh-g-kafka bash -lc \
    "kafka-topics.sh --bootstrap-server 'osh-g-kafka:9092' --create --if-not-exists --topic '$topic' --partitions '$partitions' --replication-factor 1"
}

sync_kafka() {
  log "kafka: sync topic metadata blue -> green"
  if ! kafka_topics osh-kafka "osh-kafka:9092" 2>"$WORK/kafka-blue-topics.err" | sort -u > "$WORK/kafka-blue-topics.txt"; then
    log "WARN kafka: cannot list blue topics, skipped"
    return 0
  fi
  if ! kafka_topics osh-g-kafka "osh-g-kafka:9092" 2>"$WORK/kafka-green-topics.err" | sort -u > "$WORK/kafka-green-topics-before.txt"; then
    log "WARN kafka: cannot list green topics, skipped"
    return 0
  fi
  local created=0
  while IFS= read -r topic; do
    [[ -z "$topic" || "$topic" == "__consumer_offsets" ]] && continue
    if grep -qxF "$topic" "$WORK/kafka-green-topics-before.txt"; then
      continue
    fi
    desc="$(kafka_describe_topic osh-kafka "osh-kafka:9092" "$topic" 2>/dev/null | tr '\n' ' ' || true)"
    partitions="$(printf '%s' "$desc" | awk 'match($0, /PartitionCount: *([0-9]+)/, a) {print a[1]; exit}')"
    partitions="${partitions:-1}"
    if kafka_create_topic "$topic" "$partitions" >/dev/null 2>"$WORK/kafka-create-${topic}.err"; then
      log "kafka: created topic=${topic} partitions=${partitions}"
      created=$((created + 1))
    else
      log "WARN kafka: create topic failed topic=${topic}"
    fi
  done < "$WORK/kafka-blue-topics.txt"
  log "kafka: created_topics=${created}"
}

sync_optional_components() {
  if container_exists osh-mongodb && container_exists osh-g-mongodb; then
    log "mongodb: detected but not enabled in this script, skipped"
  else
    log "mongodb: not deployed, skipped"
  fi
  if container_exists osh-hbase && container_exists osh-g-hbase; then
    log "hbase: detected but not enabled in this script, skipped"
  else
    log "hbase: not deployed, skipped"
  fi
}

write_summary() {
  python3 - "$WORK" <<'PY'
from pathlib import Path
import sys

work = Path(sys.argv[1])
summary = []
def load_counts(name):
    p = work / name
    data = {}
    if not p.exists():
        return data
    for line in p.read_text().splitlines():
        if not line.strip():
            continue
        db, table, count = line.split("\t")
        data[(db, table)] = int(count)
    return data

blue = load_counts("mysql-blue-before.tsv")
before = load_counts("mysql-green-before.tsv")
after = load_counts("mysql-green-after.tsv")
changed = []
for key in sorted(after):
    b = before.get(key, 0)
    a = after[key]
    src = blue.get(key)
    if a != b:
        changed.append((key[0], key[1], src if src is not None else -1, b, a, a - b))
with (work / "mysql-delta.tsv").open("w") as f:
    f.write("db\ttable\tblue_count\tgreen_before\tgreen_after\tdelta\n")
    for row in changed:
        f.write("\t".join(map(str, row)) + "\n")
summary.append(f"mysql_changed_tables={len(changed)}")
summary.append(f"mysql_total_count_delta={sum(row[5] for row in changed)}")
schema_report = work / "mysql-schema-delta.tsv"
summary.append(f"mysql_schema_changes={len(schema_report.read_text().splitlines()) if schema_report.exists() else 0}")
(work / "summary.txt").write_text("\n".join(summary) + "\n")
print("\n".join(summary))
PY
}

sync_mysql
sync_nacos
sync_redis
sync_es
sync_kafka
sync_optional_components
write_summary

log "complete: all-components blue -> green incremental sync"
echo success > "$STATUS_FILE"
