#!/usr/bin/env bash
# 触发 deploy-149.yml；前后端可使用不同分支（如 release/* vs prod/*）
set -euo pipefail

SLOT="${SLOT:-green}"
RELEASE_ID="${RELEASE_ID:-manual-test}"
TARGET="${TARGET:-both}" # both | backend | frontend
BACKEND_REF=""
FRONTEND_REF=""
BACKEND_DISPATCH_REF=""
FRONTEND_DISPATCH_REF=""
CLI_REF=0

usage() {
  cat <<EOF
用法:
  GITHUB_TOKEN=ghp_xxx $0 [backend_ref] [frontend_ref] [slot] [release_id]
  GITHUB_TOKEN=ghp_xxx $0 --backend-only  [backend_ref] [slot] [release_id]
  GITHUB_TOKEN=ghp_xxx $0 --frontend-only [frontend_ref] [slot] [release_id]

环境变量（可选）:
  BACKEND_REF / FRONTEND_REF
  BACKEND_DISPATCH_REF / FRONTEND_DISPATCH_REF  (显式指定 workflow 所在分支；默认与 git_ref 相同)
  GITHUB_BACKEND_GIT_REF / GITHUB_FRONTEND_GIT_REF  (未传参时的默认分支)

默认分支（GitHub 上当前可用）:
  backend  release/20260708
  frontend prod/20260708
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

if [[ "${1:-}" == "--frontend-only" ]]; then
  TARGET=frontend
  shift
  if [[ -n "${1:-}" ]]; then
    FRONTEND_REF="$1"
    FRONTEND_DISPATCH_REF="$1"
    CLI_REF=1
    shift
  fi
  SLOT="${1:-$SLOT}"
  RELEASE_ID="${2:-$RELEASE_ID}"
elif [[ "${1:-}" == "--backend-only" ]]; then
  TARGET=backend
  shift
  if [[ -n "${1:-}" ]]; then
    BACKEND_REF="$1"
    BACKEND_DISPATCH_REF="$1"
    CLI_REF=1
    shift
  fi
  SLOT="${1:-$SLOT}"
  RELEASE_ID="${2:-$RELEASE_ID}"
elif [[ -n "${1:-}" && "${1:-}" != --* ]]; then
  BACKEND_REF="$1"
  BACKEND_DISPATCH_REF="$1"
  FRONTEND_REF="${2:-}"
  FRONTEND_DISPATCH_REF="${2:-}"
  CLI_REF=1
  SLOT="${3:-$SLOT}"
  RELEASE_ID="${4:-$RELEASE_ID}"
fi

# 默认值：命令行未指定时才读 GITHUB_* 环境变量
BACKEND_REF="${BACKEND_REF:-${GITHUB_BACKEND_GIT_REF:-release/20260708}}"
FRONTEND_REF="${FRONTEND_REF:-${GITHUB_FRONTEND_GIT_REF:-prod/20260708}}"

# dispatch_ref 默认与 git_ref 相同
BACKEND_DISPATCH_REF="${BACKEND_DISPATCH_REF:-$BACKEND_REF}"
FRONTEND_DISPATCH_REF="${FRONTEND_DISPATCH_REF:-$FRONTEND_REF}"
# 命令行传了分支时，强制 dispatch = git（忽略 config 里过期的 DISPATCH_REF）
if [[ "$CLI_REF" -eq 1 ]]; then
  BACKEND_DISPATCH_REF="$BACKEND_REF"
  if [[ -n "${FRONTEND_REF:-}" ]]; then
    FRONTEND_DISPATCH_REF="$FRONTEND_REF"
  fi
fi

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  usage
  exit 1
fi

trigger_repo() {
  local repo="$1"
  local dispatch_ref="$2"
  local git_ref="$3"
  local url="https://api.github.com/repos/juege-osh/${repo}/actions/workflows/deploy-149.yml/dispatches"
  echo "→ 触发 juege-osh/${repo}  dispatch_ref=${dispatch_ref} git_ref=${git_ref} slot=${SLOT}"
  local resp http_code body
  resp=$(curl -sS -w "\n%{http_code}" -X POST \
    -H "Authorization: Bearer ${GITHUB_TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "${url}" \
    -d "{\"ref\":\"${dispatch_ref}\",\"inputs\":{\"git_ref\":\"${git_ref}\",\"slot\":\"${SLOT}\",\"release_id\":\"${RELEASE_ID}\"}}")
  http_code="${resp##*$'\n'}"
  body="${resp%$'\n'*}"
  if [[ "$http_code" == "204" ]]; then
    echo "  OK (204)"
  else
    echo "  失败 HTTP ${http_code}: ${body}"
    return 1
  fi
}

case "$TARGET" in
  backend)
    trigger_repo osh-backend "$BACKEND_DISPATCH_REF" "$BACKEND_REF"
    ;;
  frontend)
    trigger_repo osh-frontend "$FRONTEND_DISPATCH_REF" "$FRONTEND_REF"
    ;;
  both)
    trigger_repo osh-backend "$BACKEND_DISPATCH_REF" "$BACKEND_REF" || {
      echo "⚠️ backend 触发失败，继续 frontend"
      true
    }
    trigger_repo osh-frontend "$FRONTEND_DISPATCH_REF" "$FRONTEND_REF"
    ;;
  *)
    echo "unknown TARGET=$TARGET"
    exit 1
    ;;
esac

echo "完成。请到 GitHub Actions 查看 job"
echo "验收: http://149.88.92.159:28080/"
