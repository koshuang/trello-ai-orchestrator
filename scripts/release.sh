#!/usr/bin/env bash
#
# release.sh — Build locally, upload binary + .env to remote, restart via systemctl.
#
# Usage:
#   ./scripts/release.sh               # full release (build + upload + restart)
#   ./scripts/release.sh --dry-run     # show what would happen, no side effects
#   ./scripts/release.sh --build-only  # build linux binary locally, no upload
#
# Required env vars (set in .env or shell):
#   DEPLOY_HOST     Remote hostname / IP
#   DEPLOY_USER     SSH user
#   DEPLOY_PATH     Remote directory (e.g. /opt/trello-orchestrator)
#   DEPLOY_SERVICE  systemd service name (e.g. trello-orchestrator)
#
# Optional:
#   DEPLOY_BACKUPS  Number of old binaries to keep (default: 3)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BINARY="trello-orchestrator"

if [ -f "${PROJECT_DIR}/.env" ]; then
  set -a
  source "${PROJECT_DIR}/.env"
  set +a
fi

# ---------- Colors ----------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[info]${NC}  $*"; }
ok()    { echo -e "${GREEN}[ok]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $*"; }
err()   { echo -e "${RED}[err]${NC}   $*" >&2; }

# ---------- Parse flags ----------
DRY_RUN=false
BUILD_ONLY=false
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --build-only) BUILD_ONLY=true ;;
    *) err "Unknown argument: $arg"; exit 1 ;;
  esac
done

# ---------- Version info ----------
GIT_COMMIT="$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
GIT_BRANCH="$(git -C "$PROJECT_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")"
BUILD_TIME="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

echo ""
info "========================================"
info "Trello AI Orchestrator — Release"
info "  Branch : ${GIT_BRANCH}"
info "  Commit : ${GIT_COMMIT}"
info "  Time   : ${BUILD_TIME}"
info "========================================"
echo ""

# ---------- Build ----------
info "Building linux/amd64 binary..."
BUILD_CMD="GOOS=linux GOARCH=amd64 go build -ldflags=\"-X main.commit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}\" -o ${BINARY}.linux ${PROJECT_DIR}/."
echo "  $ cd ${PROJECT_DIR} && ${BUILD_CMD}"

if [ "$DRY_RUN" = false ]; then
  cd "$PROJECT_DIR"
  GOOS=linux GOARCH=amd64 go build \
    -ldflags="-X main.commit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o "${BINARY}.linux" .
  ok "Binary built: ${PROJECT_DIR}/${BINARY}.linux ($(du -sh "${BINARY}.linux" | cut -f1))"
else
  warn "[dry-run] Skipping build"
fi

if [ "$BUILD_ONLY" = true ]; then
  info "Build only — done. Binary: ${PROJECT_DIR}/${BINARY}.linux"
  exit 0
fi

# ---------- Validate deploy config ----------
HOST="${DEPLOY_HOST:-}"
USER="${DEPLOY_USER:-}"
REMOTE_PATH="${DEPLOY_PATH:-}"
SERVICE="${DEPLOY_SERVICE:-}"
BACKUPS="${DEPLOY_BACKUPS:-3}"

MISSING=""
[ -z "$HOST" ]       && MISSING="${MISSING}  DEPLOY_HOST"
[ -z "$USER" ]       && MISSING="${MISSING}  DEPLOY_USER"
[ -z "$REMOTE_PATH" ] && MISSING="${MISSING}  DEPLOY_PATH"
[ -z "$SERVICE" ]    && MISSING="${MISSING}  DEPLOY_SERVICE"

if [ -n "$MISSING" ]; then
  if [ "$DRY_RUN" = true ]; then
    warn "Dry-run: missing deploy config${MISSING} — skipping upload steps"
    echo ""
    exit 0
  fi
  err "Missing required env vars:${MISSING}"
  echo ""
  warn "Set them in .env or export them before running:"
  echo "  DEPLOY_HOST=your.server.com"
  echo "  DEPLOY_USER=deploy"
  echo "  DEPLOY_PATH=/opt/trello-orchestrator"
  echo "  DEPLOY_SERVICE=trello-orchestrator"
  echo ""
  info "You can also pass --build-only to just build the binary."
  exit 1
fi

SSH_DEST="${USER}@${HOST}"

info "Deploy target: ${SSH_DEST}:${REMOTE_PATH}"
info "Service:       ${SERVICE}"
echo ""

# ---------- Rsync binary ----------
info "Uploading binary..."
RSYNC_CMD="rsync -az --no-owner --no-group ${PROJECT_DIR}/${BINARY}.linux ${SSH_DEST}:${REMOTE_PATH}/${BINARY}.new"
echo "  $ ${RSYNC_CMD}"

if [ "$DRY_RUN" = false ]; then
  rsync -az --no-owner --no-group \
    "${PROJECT_DIR}/${BINARY}.linux" \
    "${SSH_DEST}:${REMOTE_PATH}/${BINARY}.new"
  ok "Binary uploaded"
else
  warn "[dry-run] Skipping upload"
fi

# ---------- Rsync .env ----------
if [ -f "${PROJECT_DIR}/.env" ]; then
  info "Uploading .env..."
  ENV_CMD="rsync -az --no-owner --no-group ${PROJECT_DIR}/.env ${SSH_DEST}:${REMOTE_PATH}/.env"
  echo "  $ ${ENV_CMD}"

  if [ "$DRY_RUN" = false ]; then
    rsync -az --no-owner --no-group \
      "${PROJECT_DIR}/.env" \
      "${SSH_DEST}:${REMOTE_PATH}/.env"
    ok ".env uploaded"
  else
    warn "[dry-run] Skipping .env upload"
  fi
else
  warn "No .env file found locally — skipping upload"
fi

# ---------- Remote deploy ----------
REMOTE_SCRIPT=$(cat <<'SCRIPT'
set -e
BINARY="{binary}"
REMOTE_PATH="{remote_path}"
SERVICE="{service}"
BACKUPS="{backups}"

# Rotate old binaries (keep last N)
if [ "$BACKUPS" -gt 0 ]; then
  for i in $(seq $((BACKUPS - 1)) -1 1); do
    [ -f "${REMOTE_PATH}/${BINARY}.$i" ] && mv "${REMOTE_PATH}/${BINARY}.$i" "${REMOTE_PATH}/${BINARY}.$((i + 1))"
  done
  [ -f "${REMOTE_PATH}/${BINARY}" ] && mv "${REMOTE_PATH}/${BINARY}" "${REMOTE_PATH}/${BINARY}.1"
fi

# Swap new binary in
mv "${REMOTE_PATH}/${BINARY}.new" "${REMOTE_PATH}/${BINARY}"
chmod +x "${REMOTE_PATH}/${BINARY}"

# Restart service
systemctl daemon-reload 2>/dev/null || true
systemctl restart "${SERVICE}"

# Show status
sleep 2
systemctl is-active "${SERVICE}" >/dev/null && echo "Service is active" || echo "Service is NOT active"
systemctl status "${SERVICE}" --no-pager -l | head -10
SCRIPT
)

REMOTE_SCRIPT="${REMOTE_SCRIPT//\{binary\}/$BINARY}"
REMOTE_SCRIPT="${REMOTE_SCRIPT//\{remote_path\}/$REMOTE_PATH}"
REMOTE_SCRIPT="${REMOTE_SCRIPT//\{service\}/$SERVICE}"
REMOTE_SCRIPT="${REMOTE_SCRIPT//\{backups\}/$BACKUPS}"

info "Running remote deploy via SSH..."
if [ "$DRY_RUN" = false ]; then
  if ssh "${SSH_DEST}" bash -s <<<"${REMOTE_SCRIPT}"; then
    ok "Deploy complete — ${SERVICE} restarted successfully"
  else
    err "Remote deploy failed — check the output above"
    exit 1
  fi
else
  warn "[dry-run] Would run the following on ${SSH_DEST}:"
  echo ""
  echo "${REMOTE_SCRIPT}"
fi

# ---------- Cleanup ----------
if [ "$DRY_RUN" = false ]; then
  rm -f "${PROJECT_DIR}/${BINARY}.linux"
  info "Local binary cleaned up"
fi

echo ""
ok "Release finished"
