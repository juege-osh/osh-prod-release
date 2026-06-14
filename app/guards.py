"""Deploy guards — blue stack (/opt/osh, osh-*) must never be touched."""

from __future__ import annotations

from app.config import cfg, cfg_bool

GREEN_SYNC_SCRIPT = "/opt/osh-deploy-tools/osh-green-code-sync.sh"
GREEN_ENV_FILE = "/opt/osh-deploy-tools/osh-green-code-sync.env"


def assert_green_only_config() -> None:
    """Fail fast if platform config could route deploy to blue."""
    target = cfg("PROD_DEPLOY_TARGET", "green").strip().lower()
    if target != "green":
        raise RuntimeError(
            f"PROD_DEPLOY_TARGET={target!r} 不允许：一键部署仅允许 green（蓝项目红线）"
        )

    if cfg_bool("PROD_USE_RELEASE"):
        raise RuntimeError(
            "PROD_USE_RELEASE=true 会更新蓝项目 /opt/osh，已禁用。"
            "请保持 PROD_USE_RELEASE=false 并使用 osh-green-code-sync.sh"
        )

    sync_script = cfg("PROD_CODE_SYNC_SCRIPT", GREEN_SYNC_SCRIPT).strip()
    if sync_script != GREEN_SYNC_SCRIPT:
        raise RuntimeError(
            f"PROD_CODE_SYNC_SCRIPT 必须为 {GREEN_SYNC_SCRIPT}，当前为 {sync_script!r}"
        )

    nginx_p = cfg("PROD_NGINX_PORT", "28080").strip()
    if nginx_p.startswith("5"):
        raise RuntimeError(
            f"PROD_NGINX_PORT={nginx_p} 是蓝端口（58xxx），绿环境必须为 28xxx"
        )

    for key, val in (
        ("PROD_BACKEND_PORT", cfg("PROD_BACKEND_PORT", "28081")),
        ("PROD_NACOS_PORT", cfg("PROD_NACOS_PORT", "28848")),
    ):
        if val.strip().startswith("5"):
            raise RuntimeError(f"{key}={val} 是蓝端口，绿环境必须为 2xxxx")


def remote_green_env_check_script(env_file: str = GREEN_ENV_FILE) -> str:
    """Shell snippet: exit 0 if env points only at green stack."""
    return f"""
set -euo pipefail
f="{env_file}"
test -f "$f" || {{ echo "missing_env:$f"; exit 10; }}
fe=$(grep -E '^OSH_PATH_FRONTEND=' "$f" | head -1 | cut -d= -f2-)
jar=$(grep -E '^OSH_PATH_BACKEND_JAR=' "$f" | head -1 | cut -d= -f2-)
ctr=$(grep -E '^OSH_BACKEND_CONTAINER=' "$f" | head -1 | cut -d= -f2-)
ng=$(grep -E '^OSH_NGINX_PORT=' "$f" | head -1 | cut -d= -f2-)
case "$fe" in /opt/osh-green/*) ;; *) echo "bad_frontend:$fe"; exit 11 ;; esac
case "$jar" in /opt/osh-green/*) ;; *) echo "bad_jar:$jar"; exit 12 ;; esac
case "$ctr" in osh-g-*) ;; *) echo "bad_container:$ctr"; exit 13 ;; esac
case "$ng" in 2*) ;; *) echo "bad_nginx_port:$ng"; exit 14 ;; esac
echo green_env_ok
""".strip()
