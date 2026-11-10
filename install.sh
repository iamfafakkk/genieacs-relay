#!/usr/bin/env bash
# install.sh — build, install, and start genieacs-relay under systemd (Linux).
#
# Run from a clone of this repository on the target host:
#   sudo ./install.sh
#
# Options:
#   --api-only     Skip admin UI build (API only, no web/dist)
#   --no-start     Install and enable unit but do not start the service
#   --no-enable    Install files only (no systemctl enable/start)
#   -h, --help     Show usage
#
# Interactive config (or set env vars for CI/non-TTY):
#   GENIEACS_BASE_URL, NBI_AUTH_KEY

set -euo pipefail

readonly INSTALL_ROOT="/opt/genieacs-relay"
readonly CONFIG_DIR="/etc/genieacs-relay"
readonly DATA_DIR="/var/lib/genieacs-relay"
readonly SERVICE_USER="genieacs"
readonly SERVICE_NAME="genieacs-relay"
readonly BINARY_NAME="genieacs-relay"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYSTEMD_DIR="${REPO_ROOT}/examples/systemd"
ENV_EXAMPLE="${SYSTEMD_DIR}/env.example"
UNIT_SRC="${SYSTEMD_DIR}/${SERVICE_NAME}.service"

BUILD_WEB=1
DO_START=1
DO_ENABLE=1

usage() {
  sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
}

log() { echo -e "${GREEN}==>${NC} $*"; }
warn() { echo -e "${YELLOW}warning:${NC} $*"; }
die() { echo -e "${RED}error:${NC} $*" >&2; exit 1; }

# Replace or append KEY=VALUE in a systemd EnvironmentFile (safe for special chars).
set_env_kv() {
  local file="$1" key="$2" value="$3"
  awk -v k="$key" -v v="$value" '
    BEGIN { found = 0 }
    $0 ~ "^" k "=" { print k "=" v; found = 1; next }
    { print }
    END { if (!found) print k "=" v }
  ' "$file" > "${file}.tmp"
  mv "${file}.tmp" "$file"
}

env_file_get() {
  local file="$1" key="$2"
  grep -E "^${key}=" "$file" 2>/dev/null | head -n1 | cut -d= -f2- | tr -d '\r' || true
}

nbi_auth_enabled_in_env() {
  local file="$1" v
  v="$(env_file_get "$file" NBI_AUTH)"
  v="$(printf '%s' "$v" | tr '[:upper:]' '[:lower:]')"
  [[ "$v" == "true" || "$v" == "1" ]]
}

nbi_key_needs_input() {
  local file="$1" key
  key="$(env_file_get "$file" NBI_AUTH_KEY)"
  [[ -z "$key" || "$key" == "your-nbi-auth-key-here" ]]
}

prompt_genieacs_base_url() {
  local default_nbi="http://127.0.0.1:7557"
  local nbi_url="${GENIEACS_BASE_URL:-}"
  if [[ -z "$nbi_url" && -t 0 ]]; then
    read -r -p "GenieACS NBI URL [${default_nbi}]: " nbi_url || true
  fi
  echo "${nbi_url:-$default_nbi}"
}

prompt_nbi_auth_key() {
  local key="${NBI_AUTH_KEY:-}"
  if [[ -n "$key" ]]; then
    echo "$key"
    return 0
  fi
  if [[ ! -t 0 ]]; then
    die "NBI_AUTH_KEY belum di-set. Export dulu: NBI_AUTH_KEY='...' sudo -E ./install.sh"
  fi
  while true; do
    read -r -s -p "NBI_AUTH_KEY (API key GenieACS NBI): " key
    echo ""
    if [[ -n "${key//[[:space:]]/}" ]]; then
      echo "$key"
      return 0
    fi
    warn "NBI_AUTH_KEY wajib diisi (NBI_AUTH=true di env.example)"
  done
}

configure_env_secrets() {
  local env_file="$1"
  local nbi_url nbi_key

  log "konfigurasi /etc (interaktif)"
  nbi_url="$(prompt_genieacs_base_url)"
  set_env_kv "$env_file" GENIEACS_BASE_URL "$nbi_url"

  if nbi_auth_enabled_in_env "$env_file"; then
    if nbi_key_needs_input "$env_file" || [[ -n "${NBI_AUTH_KEY:-}" ]]; then
      nbi_key="$(prompt_nbi_auth_key)"
      set_env_kv "$env_file" NBI_AUTH_KEY "$nbi_key"
    fi
  else
    warn "NBI_AUTH=false — lewati prompt NBI_AUTH_KEY"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-only) BUILD_WEB=0 ;;
    --no-start) DO_START=0 ;;
    --no-enable) DO_ENABLE=0; DO_START=0 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
  shift
done

if [[ "$(uname -s)" != "Linux" ]]; then
  die "this installer targets Linux with systemd (got $(uname -s))"
fi

if [[ "${EUID:-0}" -ne 0 ]]; then
  die "run as root: sudo $0"
fi

if ! command -v systemctl >/dev/null 2>&1; then
  die "systemd (systemctl) not found"
fi

[[ -f "${REPO_ROOT}/go.mod" ]] || die "go.mod not found; run this script from the repository root"
[[ -f "$UNIT_SRC" ]] || die "missing ${UNIT_SRC}"
[[ -f "$ENV_EXAMPLE" ]] || die "missing ${ENV_EXAMPLE}"

readonly GO_INSTALL_ROOT="/usr/local"
readonly GO_PROFILE_SNIPPET="/etc/profile.d/genieacs-relay-go.sh"

go_mod_version() {
  grep '^go ' "${REPO_ROOT}/go.mod" | awk '{print $2}'
}

go_linux_arch() {
  case "$(uname -m)" in
    x86_64) echo "amd64" ;;
    aarch64) echo "arm64" ;;
    armv7l) echo "armv6l" ;;
    *) die "unsupported CPU for Go install: $(uname -m)" ;;
  esac
}

export_go_path() {
  export PATH="${GO_INSTALL_ROOT}/go/bin:${PATH}"
  export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
}

write_go_profile_snippet() {
  cat >"$GO_PROFILE_SNIPPET" <<'EOF'
# Added by genieacs-relay install.sh (official Go tarball from go.dev)
export PATH=/usr/local/go/bin:$PATH
export GOTOOLCHAIN=auto
EOF
  chmod 0644 "$GO_PROFILE_SNIPPET"
}

ensure_download_tools() {
  local missing=()
  command -v curl >/dev/null 2>&1 || missing+=(curl)
  command -v tar >/dev/null 2>&1 || missing+=(tar)
  [[ ${#missing[@]} -eq 0 ]] && return 0
  if command -v apt-get >/dev/null 2>&1; then
    log "installing ${missing[*]} (bukan golang — hanya untuk unduh Go)"
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ca-certificates "${missing[@]}"
    return 0
  fi
  die "butuh: ${missing[*]} (untuk unduh Go dari go.dev)"
}

install_go_from_go_dev() {
  local version arch tarball url tmp
  version="$(go_mod_version)"
  [[ -n "$version" ]] || version="1.22.0"
  arch="$(go_linux_arch)"
  tarball="go${version}.linux-${arch}.tar.gz"
  url="https://go.dev/dl/${tarball}"

  ensure_download_tools
  log "mengunduh Go ${version} (curl, bukan apt): ${url}"
  tmp="$(mktemp -d)"
  if ! curl -fsSL "$url" -o "${tmp}/${tarball}"; then
    rm -rf "$tmp"
    die "gagal unduh ${url} — cek versi di go.mod atau koneksi internet"
  fi

  log "memasang Go ke ${GO_INSTALL_ROOT}/go"
  rm -rf "${GO_INSTALL_ROOT}/go"
  tar -C "$GO_INSTALL_ROOT" -xzf "${tmp}/${tarball}"
  rm -rf "$tmp"

  write_go_profile_snippet
  export_go_path
  hash -r 2>/dev/null || true
  command -v go >/dev/null 2>&1 || die "Go terpasang tapi tidak ada di PATH"
  log "$(go version)"
}

ensure_go() {
  export_go_path
  if command -v go >/dev/null 2>&1; then
    log "$(go version)"
    return 0
  fi
  if [[ -x "${GO_INSTALL_ROOT}/go/bin/go" ]]; then
    export_go_path
    log "$(go version)"
    return 0
  fi
  install_go_from_go_dev
}

install_build_deps() {
  ensure_download_tools
  if command -v apt-get >/dev/null 2>&1; then
    if ! dpkg -s git >/dev/null 2>&1; then
      log "installing git (apt)"
      apt-get update -qq
      DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git
    fi
  fi
}

ensure_service_user() {
  if ! id "$SERVICE_USER" &>/dev/null; then
    log "creating system user: ${SERVICE_USER}"
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
  fi
}

ensure_node_for_web() {
  [[ "$BUILD_WEB" -eq 1 ]] || return 0

  if command -v pnpm >/dev/null 2>&1; then
    return 0
  fi

  if command -v corepack >/dev/null 2>&1; then
    log "enabling pnpm via corepack"
    corepack enable
    corepack prepare pnpm@latest --activate
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    if ! command -v node >/dev/null 2>&1; then
      log "installing nodejs for admin UI build (apt)"
      apt-get update -qq
      DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nodejs npm
    fi
    if command -v corepack >/dev/null 2>&1; then
      corepack enable
      corepack prepare pnpm@latest --activate
      return 0
    fi
  fi

  if ! command -v pnpm >/dev/null 2>&1; then
    warn "pnpm not available; skipping admin UI (re-run without --api-only after installing Node 20+ and pnpm)"
    BUILD_WEB=0
  fi
}

build_web() {
  [[ "$BUILD_WEB" -eq 1 ]] || return 0
  log "building admin UI (web/dist)"
  (
    cd "$REPO_ROOT"
    pnpm --dir web install --frozen-lockfile 2>/dev/null || pnpm --dir web install
    pnpm --dir web build
  )
}

build_binary() {
  log "building ${BINARY_NAME}"
  local version commit build_ts ldflags
  version="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
  commit="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo none)"
  build_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  ldflags="-w -s -X main.version=${version} -X main.commit=${commit} -X main.buildTs=${build_ts}"

  (
    cd "$REPO_ROOT"
    export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
    CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "${REPO_ROOT}/${BINARY_NAME}" .
  )
}

install_files() {
  log "installing to ${INSTALL_ROOT}"
  mkdir -p "$INSTALL_ROOT" "$CONFIG_DIR" "$DATA_DIR"

  install -m 0755 "${REPO_ROOT}/${BINARY_NAME}" "${INSTALL_ROOT}/${BINARY_NAME}"

  if [[ "$BUILD_WEB" -eq 1 && -f "${REPO_ROOT}/web/dist/index.html" ]]; then
    rm -rf "${INSTALL_ROOT}/web"
    mkdir -p "${INSTALL_ROOT}/web"
    cp -a "${REPO_ROOT}/web/dist" "${INSTALL_ROOT}/web/"
  else
    warn "no web/dist — service will run API-only"
  fi

  chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_ROOT" "$DATA_DIR"
  chmod 0750 "$DATA_DIR"

  if [[ ! -f "${CONFIG_DIR}/env" ]]; then
    log "creating ${CONFIG_DIR}/env from env.example"
    install -m 0640 -o root -g "$SERVICE_USER" "$ENV_EXAMPLE" "${CONFIG_DIR}/env"
    configure_env_secrets "${CONFIG_DIR}/env"
  else
    log "env sudah ada: ${CONFIG_DIR}/env"
    if nbi_auth_enabled_in_env "${CONFIG_DIR}/env" && nbi_key_needs_input "${CONFIG_DIR}/env"; then
      warn "NBI_AUTH_KEY masih default/kosong — isi sekarang"
      set_env_kv "${CONFIG_DIR}/env" NBI_AUTH_KEY "$(prompt_nbi_auth_key)"
    elif [[ -n "${NBI_AUTH_KEY:-}" ]]; then
      set_env_kv "${CONFIG_DIR}/env" NBI_AUTH_KEY "$NBI_AUTH_KEY"
    fi
    if [[ -n "${GENIEACS_BASE_URL:-}" ]]; then
      set_env_kv "${CONFIG_DIR}/env" GENIEACS_BASE_URL "$GENIEACS_BASE_URL"
    fi
  fi
  chmod 0640 "${CONFIG_DIR}/env"
  chown root:"$SERVICE_USER" "${CONFIG_DIR}/env"
}

install_systemd_unit() {
  log "installing systemd unit"
  install -m 0644 "$UNIT_SRC" "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload
}

enable_and_start() {
  if [[ "$DO_ENABLE" -eq 1 ]]; then
    log "enabling ${SERVICE_NAME}"
    systemctl enable "$SERVICE_NAME"
  fi
  if [[ "$DO_START" -eq 1 ]]; then
    log "starting ${SERVICE_NAME}"
    systemctl restart "$SERVICE_NAME"
    wait_for_health
  fi
}

wait_for_health() {
  local addr port url i
  # shellcheck source=/dev/null
  addr="$(grep -E '^SERVER_ADDR=' "${CONFIG_DIR}/env" | cut -d= -f2- | tr -d '"')"
  addr="${addr:-:8080}"
  if [[ "$addr" == :* ]]; then
    port="${addr#:}"
    url="http://127.0.0.1:${port}/healthz"
  else
    url="http://${addr}/healthz"
  fi
  log "waiting for ${url}"
  for i in $(seq 1 30); do
    if curl -sf --max-time 2 "$url" >/dev/null; then
      echo -e "${GREEN}=== genieacs-relay is running ===${NC}"
      echo "  Health:  ${url}"
      echo "  Config:  ${CONFIG_DIR}/env"
      echo "  Logs:    journalctl -u ${SERVICE_NAME} -f"
      systemctl --no-pager status "$SERVICE_NAME" || true
      return 0
    fi
    sleep 1
  done
  warn "health check timed out — inspect: journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
  systemctl --no-pager status "$SERVICE_NAME" || true
  return 1
}

main() {
  echo -e "${GREEN}=== GenieACS Relay installer ===${NC}"
  ensure_go
  install_build_deps
  ensure_service_user
  ensure_node_for_web
  build_web
  build_binary
  install_files
  install_systemd_unit
  enable_and_start
  echo ""
  echo "Useful commands:"
  echo "  sudo systemctl status ${SERVICE_NAME}"
  echo "  sudo systemctl restart ${SERVICE_NAME}"
  echo "  sudo nano ${CONFIG_DIR}/env && sudo systemctl restart ${SERVICE_NAME}"
}

main
