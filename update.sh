#!/usr/bin/env bash
set -euo pipefail

# Deployment update for genieacs-relay (Go API + Svelte admin UI).
# Usage: sudo ./update.sh [--api-only]
# Steps: git pull -> stop service -> build backend -> build frontend -> install to /opt -> start service
# Service unit: genieacs-relay (install via ./install.sh first)

SERVICE="genieacs-relay"
SERVICE_USER="genieacs"
INSTALL_ROOT="/opt/genieacs-relay"
BINARY_NAME="genieacs-relay"
APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNIT_SRC="${APP_DIR}/examples/systemd/${SERVICE}.service"

BUILD_WEB=1
SERVICE_WAS_RUNNING=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-only) BUILD_WEB=0 ;;
    -h|--help)
      sed -n '4,7p' "$0" | sed 's/^# \{0,1\}//'
      echo "  --api-only    Skip admin UI build"
      exit 0
      ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
  shift
done

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "error: Linux + systemd required" >&2
  exit 1
fi

if [[ "${EUID:-0}" -ne 0 ]]; then
  echo "error: run as root: sudo $0" >&2
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "error: systemctl not found" >&2
  exit 1
fi

if [[ ! -x "${INSTALL_ROOT}/${BINARY_NAME}" ]]; then
  echo "error: ${INSTALL_ROOT}/${BINARY_NAME} missing — run sudo ./install.sh first" >&2
  exit 1
fi

# If anything fails after stopping the service, bring it back with the previous /opt binary.
trap 'if [[ "$SERVICE_WAS_RUNNING" -eq 1 ]]; then systemctl start "$SERVICE" >/dev/null 2>&1 || true; fi' EXIT

cd "$APP_DIR"

echo "==> [1/6] git pull"
git pull --ff-only

echo "==> [2/6] stop service"
if systemctl is-active --quiet "$SERVICE"; then
  SERVICE_WAS_RUNNING=1
  systemctl stop "$SERVICE"
else
  SERVICE_WAS_RUNNING=0
fi

echo "==> [3/6] build backend binary"
version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
commit="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
build_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ldflags="-w -s -X main.version=${version} -X main.commit=${commit} -X main.buildTs=${build_ts}"

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "${APP_DIR}/${BINARY_NAME}" .

if [[ "$BUILD_WEB" -eq 1 ]]; then
  echo "==> [4/6] frontend: install deps + build"
  if ! command -v pnpm >/dev/null 2>&1; then
    echo "error: pnpm not found (install Node/pnpm or use --api-only)" >&2
    exit 1
  fi
  pnpm --dir web install --frozen-lockfile 2>/dev/null || pnpm --dir web install
  pnpm --dir web build
else
  echo "==> [4/6] frontend: skipped (--api-only)"
fi

echo "==> [5/6] install artifacts to ${INSTALL_ROOT}"
install -m 0755 "${APP_DIR}/${BINARY_NAME}" "${INSTALL_ROOT}/${BINARY_NAME}"

if [[ "$BUILD_WEB" -eq 1 && -f "${APP_DIR}/web/dist/index.html" ]]; then
  rm -rf "${INSTALL_ROOT}/web"
  mkdir -p "${INSTALL_ROOT}/web"
  cp -a "${APP_DIR}/web/dist" "${INSTALL_ROOT}/web/"
fi

chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_ROOT"

if [[ -f "$UNIT_SRC" ]]; then
  install -m 0644 "$UNIT_SRC" "/etc/systemd/system/${SERVICE}.service"
  systemctl daemon-reload
fi

echo "==> [6/6] start service"
SERVICE_WAS_RUNNING=0
trap - EXIT
systemctl start "$SERVICE"
systemctl --no-pager --full status "$SERVICE" | head -n 8

echo "==> done: $(date '+%Y-%m-%d %H:%M:%S')"
