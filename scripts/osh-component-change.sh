#!/usr/bin/env bash
set -euo pipefail

PHASE=""
SLOT=""
KIND=""
RELEASE=""
ITEM=""
ACTION="apply"
REF=""
NODE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --phase) PHASE="${2:-}"; shift 2 ;;
    --slot) SLOT="${2:-}"; shift 2 ;;
    --kind) KIND="${2:-}"; shift 2 ;;
    --release) RELEASE="${2:-}"; shift 2 ;;
    --item) ITEM="${2:-}"; shift 2 ;;
    --action) ACTION="${2:-}"; shift 2 ;;
    --ref) REF="${2:-}"; shift 2 ;;
    --node) NODE="${2:-}"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

PHASE="${PHASE:?--phase required}"
SLOT="${SLOT:?--slot required}"
KIND="${KIND:?--kind required}"
RELEASE="${RELEASE:?--release required}"
ITEM="${ITEM:?--item required}"

case "$SLOT" in green|blue) ;; *) echo "bad slot: $SLOT" >&2; exit 2 ;; esac
case "$KIND" in elasticsearch) KIND="es" ;; esac

ROOT="${COMPONENT_CHANGE_LOG_ROOT:-/opt/osh-green/004-log/osh/component-change}"
WORK="${ROOT}/${RELEASE}/${ITEM}/${SLOT}"
mkdir -p "$WORK"

BLUE_ENV="${BLUE_ENV:-/opt/osh/001-docker-compose/osh/osh-stack.env}"
GREEN_ENV="${GREEN_ENV:-/opt/osh-green/001-docker-compose/osh/osh-green-stack.env}"

log() { echo "[$(date '+%F %T')] $*" | tee -a "$WORK/run.log"; }
fail() {
  local line="$1"
  log "FAILED phase=${PHASE} kind=${KIND} slot=${SLOT} line=${line}"
  echo failed > "$WORK/status"
}
trap 'fail $LINENO' ERR

load_env_value() {
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 0
  awk -F= -v k="$key" '$1 == k {print substr($0, length(k) + 2); exit}' "$file"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing command: $1" >&2; exit 127; }
}

container_exists() {
  docker ps --format '{{.Names}}' | grep -qx "$1"
}

require_container() {
  container_exists "$1" || { echo "missing container: $1" >&2; exit 12; }
}

slot_env_file() {
  if [[ "$SLOT" == "blue" ]]; then
    printf '%s' "$BLUE_ENV"
  else
    printf '%s' "$GREEN_ENV"
  fi
}

mysql_container() {
  [[ "$SLOT" == "blue" ]] && printf 'osh-mysql' || printf 'osh-g-mysql'
}

redis_container() {
  [[ "$SLOT" == "blue" ]] && printf 'osh-redis' || printf 'osh-g-redis'
}

es_url() {
  [[ "$SLOT" == "blue" ]] && printf 'http://127.0.0.1:59200' || printf 'http://127.0.0.1:29200'
}

nacos_url() {
  [[ "$SLOT" == "blue" ]] && printf 'http://127.0.0.1:58848' || printf 'http://127.0.0.1:28848'
}

kafka_container() {
  [[ "$SLOT" == "blue" ]] && printf 'osh-kafka' || printf 'osh-g-kafka'
}

kafka_bin() {
  printf '/opt/kafka/bin'
}

kafka_topics_cmd() {
  printf '%s/kafka-topics.sh' "$(kafka_bin)"
}

kafka_bootstrap() {
  [[ "$SLOT" == "blue" ]] && printf 'osh-kafka:9092' || printf 'osh-g-kafka:9092'
}

mongodb_container() {
  [[ "$SLOT" == "blue" ]] && printf 'osh-mongodb' || printf 'osh-g-mongodb'
}

hbase_container() {
  [[ "$SLOT" == "blue" ]] && printf 'osh-hbase' || printf 'osh-g-hbase'
}

mysql_password() {
  local file
  file="$(slot_env_file)"
  load_env_value "$file" MYSQL_ROOT_PASSWORD
}

redis_password() {
  local file
  file="$(slot_env_file)"
  load_env_value "$file" REDIS_PASSWORD
}

es_password() {
  local file
  file="$(slot_env_file)"
  load_env_value "$file" ES_PASSWORD
}

run_ref_script() {
  [[ -n "$REF" && -f "$REF" ]] || return 1
  case "$REF" in
    *.sh) ;;
    *) [[ -x "$REF" ]] || return 1 ;;
  esac
  log "run custom ref script: $REF"
  COMPONENT_PHASE="$PHASE" COMPONENT_SLOT="$SLOT" COMPONENT_KIND="$KIND" \
    COMPONENT_RELEASE="$RELEASE" COMPONENT_ITEM="$ITEM" COMPONENT_ACTION="$ACTION" \
    COMPONENT_NODE="$NODE" COMPONENT_WORK="$WORK" bash "$REF"
}

phase_plan() {
  cat > "$WORK/plan.json" <<EOF
{"phase":"plan","slot":"${SLOT}","kind":"${KIND}","action":"${ACTION}","ref":"${REF}","node":"${NODE}","work":"${WORK}"}
EOF
  log "plan written: $WORK/plan.json"
}

mysql_snapshot() {
  local c pass dbs db
  c="$(mysql_container)"
  pass="$(mysql_password)"
  db="$(mysql_database)"
  dbs="${MYSQL_DATABASES:-backstage osh_secret xxl_job}"
  require_container "$c"
  [[ -n "$pass" ]] || { echo "MYSQL_ROOT_PASSWORD missing for $SLOT" >&2; exit 13; }
  docker exec "$c" mysqldump -uroot -p"${pass}" --single-transaction --routines --triggers --events --databases ${dbs} > "$WORK/mysql-before.sql"
  test -s "$WORK/mysql-before.sql"
  docker exec "$c" mysql -uroot -p"${pass}" -N -B -e \
    "SELECT table_name FROM information_schema.tables WHERE table_schema='${db}' ORDER BY 1" \
    > "$WORK/mysql-tables-before.tsv" || true
  log "mysql snapshot: $WORK/mysql-before.sql"
}

mysql_database() {
  local file db
  file="$(slot_env_file)"
  db="${MYSQL_DATABASE:-$(load_env_value "$file" MYSQL_DATABASE)}"
  [[ -n "$db" ]] || db="backstage"
  printf '%s' "$db"
}

mysql_apply() {
  local c pass sql_file db
  c="$(mysql_container)"
  pass="$(mysql_password)"
  db="$(mysql_database)"
  require_container "$c"
  [[ -n "$pass" ]] || { echo "MYSQL_ROOT_PASSWORD missing for $SLOT" >&2; exit 13; }
  [[ -n "$REF" && -f "$REF" ]] || { log "mysql apply skipped: ref SQL file not supplied"; return 0; }
  sql_file="$REF"
  case "$sql_file" in *.sql) ;; *) echo "mysql ref must be a .sql file: $sql_file" >&2; exit 14 ;; esac
  docker exec -i "$c" mysql -uroot -p"${pass}" "$db" < "$sql_file"
  log "mysql applied SQL: $sql_file db=$db"
}

mysql_rollback() {
  local c pass
  c="$(mysql_container)"
  pass="$(mysql_password)"
  require_container "$c"
  [[ -s "$WORK/mysql-before.sql" ]] || { log "mysql rollback skipped: snapshot not found"; return 0; }
  docker exec -i "$c" mysql -uroot -p"${pass}" < "$WORK/mysql-before.sql"
  log "mysql rollback restored: $WORK/mysql-before.sql"
}

mysql_test() {
  local c pass db
  c="$(mysql_container)"
  pass="$(mysql_password)"
  db="$(mysql_database)"
  require_container "$c"
  docker exec "$c" mysqladmin ping -uroot -p"${pass}" --silent
  docker exec "$c" mysql -uroot -p"${pass}" -N -B -e "SELECT 1" > "$WORK/mysql-test.tsv"
  docker exec "$c" mysql -uroot -p"${pass}" -N -B -e \
    "SELECT table_name FROM information_schema.tables WHERE table_schema='${db}' ORDER BY 1" \
    > "$WORK/mysql-tables-after.tsv" || true
  log "mysql test passed"
}

redis_snapshot() {
  local c pass
  c="$(redis_container)"
  pass="$(redis_password)"
  require_container "$c"
  docker exec "$c" redis-cli -a "$pass" --no-auth-warning INFO keyspace > "$WORK/redis-keyspace-before.txt"
  log "redis snapshot metadata: $WORK/redis-keyspace-before.txt"
}

redis_apply() {
  local c pass
  c="$(redis_container)"
  pass="$(redis_password)"
  require_container "$c"
  [[ -n "$REF" && -f "$REF" ]] || { log "redis apply skipped: ref command file not supplied"; return 0; }
  while IFS= read -r line; do
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    docker exec "$c" redis-cli -a "$pass" --no-auth-warning ${line}
  done < "$REF"
  log "redis commands applied: $REF"
}

redis_test() {
  local c pass
  c="$(redis_container)"
  pass="$(redis_password)"
  require_container "$c"
  docker exec "$c" redis-cli -a "$pass" --no-auth-warning PING | grep -qx PONG
  docker exec "$c" redis-cli -a "$pass" --no-auth-warning INFO keyspace > "$WORK/redis-keyspace-after.txt" || true
  log "redis test passed"
}

nacos_snapshot() {
  local url
  url="$(nacos_url)"
  curl -sf "$url/nacos/v1/ns/operator/metrics" > "$WORK/nacos-metrics-before.json" || true
  log "nacos snapshot metadata written"
}

nacos_apply() {
  [[ -n "$REF" && -f "$REF" ]] || { log "nacos apply skipped: ref file not supplied"; return 0; }
  if run_ref_script; then return 0; fi
  log "nacos ref is not an executable script, no built-in config mutation performed"
}

nacos_test() {
  local url
  url="$(nacos_url)"
  curl -sf "$url/nacos/v1/ns/operator/metrics" > "$WORK/nacos-metrics-after.json"
  cp "$WORK/nacos-metrics-after.json" "$WORK/nacos-test.json"
  log "nacos test passed"
}

es_snapshot() {
  local url pass
  url="$(es_url)"
  pass="$(es_password)"
  curl -sf -u "elastic:${pass}" "$url/_cluster/health" > "$WORK/es-health-before.json"
  curl -sf -u "elastic:${pass}" "$url/_cat/indices?h=index,docs.count,store.size" > "$WORK/es-indices-before.tsv" || true
  log "es snapshot metadata written"
}

es_apply() {
  if run_ref_script; then return 0; fi
  log "es apply skipped: use ref executable script for index/mapping mutations"
}

es_test() {
  local url pass
  url="$(es_url)"
  pass="$(es_password)"
  curl -sf -u "elastic:${pass}" "$url/_cluster/health" > "$WORK/es-health-test.json"
  curl -sf -u "elastic:${pass}" "$url/_cat/indices?h=index,docs.count,store.size" > "$WORK/es-indices-after.tsv" || true
  log "es test passed"
}

kafka_snapshot() {
  local c bootstrap
  c="$(kafka_container)"
  bootstrap="$(kafka_bootstrap)"
  require_container "$c"
  docker exec "$c" bash -lc "$(kafka_topics_cmd) --bootstrap-server '$bootstrap' --list" | sort -u > "$WORK/kafka-topics-before.txt"
  log "kafka topic snapshot written"
}

kafka_apply() {
  local c bootstrap topic partitions
  c="$(kafka_container)"
  bootstrap="$(kafka_bootstrap)"
  require_container "$c"
  if run_ref_script; then return 0; fi
  case "$ACTION" in
    create-topic|create_topic)
      topic="${REF:?topic ref required}"
      partitions="${NODE:-1}"
      if grep -qxF "$topic" "$WORK/kafka-topics-before.txt" 2>/dev/null; then
        log "kafka topic already existed: $topic"
        echo existed > "$WORK/kafka-topic-${topic}.state"
      else
        docker exec "$c" bash -lc "$(kafka_topics_cmd) --bootstrap-server '$bootstrap' --create --if-not-exists --topic '$topic' --partitions '$partitions' --replication-factor 1"
        echo created > "$WORK/kafka-topic-${topic}.state"
        log "kafka topic created: $topic partitions=$partitions"
      fi
      ;;
    *)
      log "kafka apply skipped: unsupported action=$ACTION"
      ;;
  esac
}

kafka_rollback() {
  local c bootstrap topic
  c="$(kafka_container)"
  bootstrap="$(kafka_bootstrap)"
  require_container "$c"
  case "$ACTION" in
    create-topic|create_topic)
      topic="${REF:?topic ref required}"
      if [[ -f "$WORK/kafka-topic-${topic}.state" ]] && grep -qx created "$WORK/kafka-topic-${topic}.state"; then
        docker exec "$c" bash -lc "$(kafka_topics_cmd) --bootstrap-server '$bootstrap' --delete --topic '$topic'"
        log "kafka topic rollback deleted: $topic"
      else
        log "kafka rollback skipped: topic was pre-existing"
      fi
      ;;
    *) log "kafka rollback skipped: unsupported action=$ACTION" ;;
  esac
}

kafka_test() {
  local c bootstrap
  c="$(kafka_container)"
  bootstrap="$(kafka_bootstrap)"
  require_container "$c"
  docker exec "$c" bash -lc "$(kafka_topics_cmd) --bootstrap-server '$bootstrap' --list" > "$WORK/kafka-test-topics.txt"
  log "kafka test passed"
}

mongodb_snapshot() {
  local c
  c="$(mongodb_container)"
  if ! container_exists "$c"; then log "mongodb not deployed on $SLOT"; return 0; fi
  docker exec "$c" sh -lc 'command -v mongodump >/dev/null && mongodump --archive' > "$WORK/mongodb-before.archive" || true
  log "mongodb snapshot best-effort complete"
}

mongodb_apply() {
  if run_ref_script; then return 0; fi
  log "mongodb apply skipped: use ref executable script for collection mutations"
}

mongodb_rollback() {
  local c
  c="$(mongodb_container)"
  [[ -s "$WORK/mongodb-before.archive" ]] || { log "mongodb rollback skipped: snapshot not found"; return 0; }
  docker exec -i "$c" sh -lc 'command -v mongorestore >/dev/null && mongorestore --archive --drop' < "$WORK/mongodb-before.archive"
  log "mongodb rollback restored archive"
}

mongodb_test() {
  local c
  c="$(mongodb_container)"
  if ! container_exists "$c"; then log "mongodb not deployed on $SLOT"; return 0; fi
  docker exec "$c" sh -lc 'mongosh --quiet --eval "db.adminCommand({ ping: 1 }).ok" 2>/dev/null || mongo --quiet --eval "db.adminCommand({ ping: 1 }).ok"' > "$WORK/mongodb-test.txt"
  grep -q 1 "$WORK/mongodb-test.txt"
  log "mongodb test passed"
}

hbase_snapshot() {
  local c
  c="$(hbase_container)"
  if ! container_exists "$c"; then log "hbase not deployed on $SLOT"; return 0; fi
  docker exec "$c" sh -lc "echo 'list' | hbase shell -n" > "$WORK/hbase-list-before.txt" || true
  log "hbase snapshot metadata written"
}

hbase_apply() {
  if run_ref_script; then return 0; fi
  log "hbase apply skipped: use ref executable script for table mutations"
}

hbase_test() {
  local c
  c="$(hbase_container)"
  if ! container_exists "$c"; then log "hbase not deployed on $SLOT"; return 0; fi
  docker exec "$c" sh -lc "echo 'status' | hbase shell -n" > "$WORK/hbase-test.txt"
  log "hbase test passed"
}

phase_snapshot() {
  case "$KIND" in
    mysql) mysql_snapshot ;;
    redis) redis_snapshot ;;
    nacos) nacos_snapshot ;;
    es) es_snapshot ;;
    kafka) kafka_snapshot ;;
    mongodb) mongodb_snapshot ;;
    hbase) hbase_snapshot ;;
    *) log "snapshot skipped: extension kind=$KIND" ;;
  esac
}

phase_apply() {
  case "$KIND" in
    mysql) mysql_apply ;;
    redis) redis_apply ;;
    nacos) nacos_apply ;;
    es) es_apply ;;
    kafka) kafka_apply ;;
    mongodb) mongodb_apply ;;
    hbase) hbase_apply ;;
    *) if run_ref_script; then :; else log "apply skipped: extension kind=$KIND"; fi ;;
  esac
}

phase_rollback() {
  case "$KIND" in
    mysql) mysql_rollback ;;
    kafka) kafka_rollback ;;
    mongodb) mongodb_rollback ;;
    *) if run_ref_script; then :; else log "rollback skipped: no built-in rollback for kind=$KIND"; fi ;;
  esac
}

phase_test() {
  case "$KIND" in
    mysql) mysql_test ;;
    redis) redis_test ;;
    nacos) nacos_test ;;
    es) es_test ;;
    kafka) kafka_test ;;
    mongodb) mongodb_test ;;
    hbase) hbase_test ;;
    *) if run_ref_script; then :; else log "test skipped: extension kind=$KIND"; fi ;;
  esac
}

comm_lines() {
  # comm_lines mode before after  (mode: added|removed)
  local mode="$1" before="$2" after="$3"
  [[ -f "$before" && -f "$after" ]] || return 0
  case "$mode" in
    added) comm -13 "$before" "$after" | sed '/^$/d' ;;
    removed) comm -23 "$before" "$after" | sed '/^$/d' ;;
  esac
}

redis_key_count() {
  local file="$1"
  [[ -f "$file" ]] || { echo 0; return; }
  awk -F: '/^db[0-9]+:keys=/ { s += $2 } END { print s+0 }' "$file" 2>/dev/null || echo 0
}

emit_data_diff_json() {
  python3 -c 'import json,sys; print("__OSH_DATA_DIFF__"+json.dumps(json.loads(sys.argv[1]), ensure_ascii=False))' "$1"
}

mysql_diff_report() {
  local db added removed
  db="$(mysql_database)"
  added="$(comm_lines added "$WORK/mysql-tables-before.tsv" "$WORK/mysql-tables-after.tsv" | tr '\n' ',' | sed 's/,$//')"
  removed="$(comm_lines removed "$WORK/mysql-tables-before.tsv" "$WORK/mysql-tables-after.tsv" | tr '\n' ',' | sed 's/,$//')"
  local payload
  payload=$(WORK="$WORK" DB="$db" ADDED="$added" REMOVED="$removed" python3 - <<'PY'
import json, os
added=[x for x in os.environ.get("ADDED","").split(",") if x]
removed=[x for x in os.environ.get("REMOVED","").split(",") if x]
print(json.dumps({
  "component":"mysql","database":os.environ.get("DB","backstage"),
  "added":added,"removed":removed,"modified":[],
  "summary":{"added_count":len(added),"removed_count":len(removed),"modified_count":0}
}, ensure_ascii=False))
PY
)
  emit_data_diff_json "$payload"
}

redis_diff_report() {
  local payload
  payload=$(WORK="$WORK" python3 - <<'PY'
import json, os, re
work=os.environ.get("WORK",".")
def key_count(path):
    try:
        text=open(path).read()
    except OSError:
        return 0
    return sum(int(m.group(1)) for m in re.finditer(r'keys=(\d+)', text))
before=key_count(f"{work}/redis-keyspace-before.txt")
after=key_count(f"{work}/redis-keyspace-after.txt")
added, removed = [], []
if after > before: added.append(f"keys +{after-before} (total {after})")
elif after < before: removed.append(f"keys -{before-after} (total {after})")
print(json.dumps({
  "component":"redis","added":added,"removed":removed,"modified":[],
  "summary":{"keys_before":before,"keys_after":after,
    "added_count":max(0,after-before),"removed_count":max(0,before-after),"modified_count":0}
}, ensure_ascii=False))
PY
)
  emit_data_diff_json "$payload"
}

kafka_diff_report() {
  cp "$WORK/kafka-test-topics.txt" "$WORK/kafka-topics-after.txt" 2>/dev/null || true
  local added removed
  added="$(comm_lines added "$WORK/kafka-topics-before.txt" "$WORK/kafka-topics-after.txt" | tr '\n' ',' | sed 's/,$//')"
  removed="$(comm_lines removed "$WORK/kafka-topics-before.txt" "$WORK/kafka-topics-after.txt" | tr '\n' ',' | sed 's/,$//')"
  local payload
  payload=$(ADDED="$added" REMOVED="$removed" python3 - <<'PY'
import json, os
added=[x for x in os.environ.get("ADDED","").split(",") if x]
removed=[x for x in os.environ.get("REMOVED","").split(",") if x]
print(json.dumps({
  "component":"kafka","added":added,"removed":removed,"modified":[],
  "summary":{"added_count":len(added),"removed_count":len(removed),"modified_count":0}
}, ensure_ascii=False))
PY
)
  emit_data_diff_json "$payload"
}

es_diff_report() {
  local added removed
  added="$(comm -13 <(cut -f1 "$WORK/es-indices-before.tsv" 2>/dev/null | sort -u) <(cut -f1 "$WORK/es-indices-after.tsv" 2>/dev/null | sort -u) 2>/dev/null | tr '\n' ',' | sed 's/,$//')"
  removed="$(comm -23 <(cut -f1 "$WORK/es-indices-before.tsv" 2>/dev/null | sort -u) <(cut -f1 "$WORK/es-indices-after.tsv" 2>/dev/null | sort -u) 2>/dev/null | tr '\n' ',' | sed 's/,$//')"
  local payload
  payload=$(ADDED="$added" REMOVED="$removed" python3 - <<'PY'
import json, os
added=[x for x in os.environ.get("ADDED","").split(",") if x]
removed=[x for x in os.environ.get("REMOVED","").split(",") if x]
print(json.dumps({
  "component":"es","added":added,"removed":removed,"modified":[],
  "summary":{"added_count":len(added),"removed_count":len(removed),"modified_count":0}
}, ensure_ascii=False))
PY
)
  emit_data_diff_json "$payload"
}

nacos_diff_report() {
  local payload='{"component":"nacos","added":[],"removed":[],"modified":[],"summary":{"added_count":0,"removed_count":0,"modified_count":0,"note":"nacos config diff requires dataId-level snapshot"}}'
  if [[ -f "$WORK/nacos-metrics-before.json" && -f "$WORK/nacos-metrics-after.json" ]]; then
    if ! cmp -s "$WORK/nacos-metrics-before.json" "$WORK/nacos-metrics-after.json" 2>/dev/null; then
      payload='{"component":"nacos","added":["metrics changed"],"removed":[],"modified":["operator metrics"],"summary":{"added_count":1,"removed_count":0,"modified_count":1}}'
    fi
  fi
  emit_data_diff_json "$payload"
}

phase_diff_report() {
  case "$KIND" in
    mysql) mysql_diff_report ;;
    redis) redis_diff_report ;;
    nacos) nacos_diff_report ;;
    es) es_diff_report ;;
    kafka) kafka_diff_report ;;
    *) emit_data_diff_json '{"component":"'"$KIND"'","added":[],"removed":[],"modified":[],"summary":{"added_count":0,"removed_count":0,"modified_count":0}}' ;;
  esac
  log "data diff report emitted for kind=$KIND"
}

log "start phase=${PHASE} slot=${SLOT} kind=${KIND} release=${RELEASE} item=${ITEM} action=${ACTION} ref=${REF} node=${NODE}"
require_cmd docker

case "$PHASE" in
  plan) phase_plan ;;
  snapshot) phase_snapshot ;;
  apply-green|apply-blue) phase_apply ;;
  rollback-green|rollback-blue) phase_rollback ;;
  test) phase_test ;;
  diff-report) phase_diff_report ;;
  *) echo "unsupported phase: $PHASE" >&2; exit 2 ;;
esac

echo success > "$WORK/status"
log "done phase=${PHASE} kind=${KIND} slot=${SLOT}"
