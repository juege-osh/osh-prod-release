#!/usr/bin/env bash
# Runs ON 149 after rsync. Only mutates DEPLOY_DIR (+ installs systemd unit for this service).
set -euo pipefail

DEPLOY_DIR="${DEPLOY_DIR:?DEPLOY_DIR required}"
STAGING="${DEPLOY_DIR}/.deploy-staging"

case "$DEPLOY_DIR" in
  /opt/osh-prod-release) ;;
  *)
    echo "refusing: DEPLOY_DIR must be /opt/osh-prod-release (got: $DEPLOY_DIR)" >&2
    exit 1
    ;;
esac

if [[ ! -d "$STAGING/bin" ]]; then
  echo "staging bundle missing: $STAGING/bin" >&2
  exit 1
fi

mkdir -p "$DEPLOY_DIR"/{bin,data,logs}

if [[ ! -f "$DEPLOY_DIR/config.env" ]]; then
  if [[ -f "$STAGING/config.env.149.example" ]]; then
    cp "$STAGING/config.env.149.example" "$DEPLOY_DIR/config.env"
    chmod 600 "$DEPLOY_DIR/config.env"
    echo "created $DEPLOY_DIR/config.env from example — edit secrets before deploy"
  else
    echo "warning: config.env missing and no example in staging" >&2
  fi
fi

install -m 755 "$STAGING/bin/osh-prod-release" "$DEPLOY_DIR/bin/osh-prod-release"

if [[ -d "$STAGING/migrations" ]]; then
  rsync -a --delete "$STAGING/migrations/" "$DEPLOY_DIR/migrations/"
fi

if [[ -d "$STAGING/components" ]]; then
  rsync -a "$STAGING/components/" "$DEPLOY_DIR/components/"
fi

if [[ -f "$STAGING/deploy/systemd/osh-prod-release.service" ]]; then
  install -m 644 "$STAGING/deploy/systemd/osh-prod-release.service" /etc/systemd/system/osh-prod-release.service
  systemctl daemon-reload
  systemctl enable osh-prod-release.service
fi

systemctl restart osh-prod-release.service

rm -rf "$STAGING"
echo "deploy complete: $DEPLOY_DIR"
