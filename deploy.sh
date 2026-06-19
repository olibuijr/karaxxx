#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

BINARY="karaxxx"
SERVER="root@192.168.8.4"
SERVER_DIR="/opt/karaxxx"
SERVICE="karaxxx"
VERSION_FILE="VERSION"
HEALTH_URL="http://127.0.0.1:8799/api/health"

service_unit() {
  if [[ "$SERVICE" == *.service ]]; then
    echo "$SERVICE"
  else
    echo "$SERVICE.service"
  fi
}

remote_systemctl() {
  ssh "$SERVER" "systemctl $*"
}

remote_journal_tail() {
  ssh "$SERVER" "journalctl -u $(service_unit) --no-pager -n 40" || true
}

remote_curl() {
  ssh "$SERVER" "curl -fsS --max-time 3 $*"
}

usage() {
  cat <<'EOF'
Usage: ./deploy.sh <command> [version]

Commands:
  build         Build the Go binary
  build-web     Build the React frontend
  build-all     Build Go binary + React frontend
  push          Stop service, then push binary + web build to server
  push-web      Push only web frontend to server
  restart       Restart the karaxxx service
  deploy [ver]  Full deploy: build-all + version/changelog update + push + restart
  status        Show server service status
  version       Show current version
  bump <ver>    Bump version (e.g. ./deploy.sh bump 1.2.0)
  changelog     Show recent changelog entries
  help          Show this help
EOF
}

current_version() {
  if [[ -f "$VERSION_FILE" ]]; then
    cat "$VERSION_FILE"
  else
    echo "0.0.0"
  fi
}

next_patch_version() {
  local current major minor patch
  current="$(current_version)"
  IFS='.' read -r major minor patch <<< "$current"
  major="${major:-0}"
  minor="${minor:-0}"
  patch="${patch:-0}"
  if ! [[ "$major" =~ ^[0-9]+$ && "$minor" =~ ^[0-9]+$ && "$patch" =~ ^[0-9]+$ ]]; then
    echo "0.0.1"
    return
  fi
  echo "$major.$minor.$((patch + 1))"
}

bump_version() {
  local v="$1"
  echo "$v" > "$VERSION_FILE"
  echo "Version bumped to $v"
}

changelog_has_version() {
  local v="$1"
  [[ -f CHANGELOG.md ]] && grep -q "^## \\[$v\\]" CHANGELOG.md
}

prepend_changelog_entry() {
  local v="$1"
  local notes="${KARAXXX_RELEASE_NOTES:-Release deployed through deploy.sh.}"
  if changelog_has_version "$v"; then
    echo "Changelog already contains $v"
    return
  fi

  local tmp
  tmp="$(mktemp)"
  {
    echo "# Changelog"
    echo
    echo "## [$v] — $(date +%F)"
    echo
    echo "### Changed"
    while IFS= read -r line; do
      [[ -n "$line" ]] && echo "- $line"
    done <<< "$notes"
    echo
    if [[ -f CHANGELOG.md ]]; then
      tail -n +3 CHANGELOG.md
    fi
  } > "$tmp"
  mv "$tmp" CHANGELOG.md
  echo "Changelog updated for $v"
}

build_go() {
  echo "=== Building Go binary ==="
  go build -tags "sqlite_fts5" -buildvcs=false -ldflags="-s -w" -o "$BINARY" .
  echo "  Binary: $BINARY ($(du -h "$BINARY" | cut -f1))"
}

build_web() {
  echo "=== Building React frontend ==="
  cd "$SCRIPT_DIR/web"
  bun install --frozen-lockfile --silent
  bun run build
  cd "$SCRIPT_DIR"
  echo "  Frontend built to web/dist/"
}

build_all() {
  build_go
  build_web
}

verify_systemd_service() {
  local unit
  unit="$(service_unit)"
  echo "=== Verifying systemd unit: $unit ==="
  remote_systemctl "cat $unit --no-pager" >/dev/null
  local load_state
  load_state="$(remote_systemctl "show $unit -p LoadState --value --no-pager")"
  if [[ "$load_state" != "loaded" ]]; then
    echo "Systemd unit $unit is not loaded (LoadState=$load_state)"
    remote_systemctl "status $unit --no-pager -l" || true
    exit 1
  fi
  echo "  $unit is loaded"
}

stop_service() {
  local unit
  unit="$(service_unit)"
  echo "=== Stopping $unit ==="
  if ! remote_systemctl "stop $unit"; then
    echo "Failed to stop $unit"
    remote_systemctl "status $unit --no-pager -l" || true
    remote_journal_tail
    exit 1
  fi
  echo "  $unit stopped"
}

push_binary() {
  echo "=== Pushing to $SERVER ==="
  scp "$BINARY" "$SERVER:$SERVER_DIR/"
  echo "  Binary pushed"
}

push_web() {
  echo "=== Pushing web frontend ==="
  ssh "$SERVER" "rm -rf $SERVER_DIR/web/dist"
  ssh "$SERVER" "mkdir -p $SERVER_DIR/web"
  scp -r "$SCRIPT_DIR/web/dist" "$SERVER:$SERVER_DIR/web/"
  echo "  Frontend pushed to $SERVER_DIR/web/dist/"
}

push_release_files() {
  echo "=== Pushing release metadata ==="
  ssh "$SERVER" "mkdir -p $SERVER_DIR"
  scp "$VERSION_FILE" CHANGELOG.md "$SERVER:$SERVER_DIR/"
  echo "  VERSION and CHANGELOG.md pushed"
}

ensure_remote_tools() {
  echo "=== Ensuring remote scraper tools ==="
  ssh "$SERVER" "if ! command -v yt-dlp >/dev/null 2>&1; then apt-get update && apt-get install -y yt-dlp; fi"
  echo "  yt-dlp available on target"
}

push_all() {
  push_binary
  push_web
  push_release_files
}

restart_service() {
  local unit
  unit="$(service_unit)"
  echo "=== Restarting $unit ==="
  if ! remote_systemctl "restart $unit"; then
    echo "Failed to restart $unit"
    remote_systemctl "status $unit --no-pager -l" || true
    remote_journal_tail
    exit 1
  fi
  for _ in {1..30}; do
    if remote_systemctl "is-active --quiet $unit" && remote_curl "$HEALTH_URL" >/dev/null 2>&1; then
      echo "  $unit is active and HTTP-ready"
      echo "--- Status ---"
      remote_systemctl "status $unit --no-pager -l"
      return
    fi
    sleep 1
  done
  echo "$unit did not become HTTP-ready after restart"
  echo "--- Status ---"
  remote_systemctl "status $unit --no-pager -l" || true
  echo "--- Health probe ---"
  remote_curl "-i $HEALTH_URL" || true
  remote_journal_tail
  exit 1
}

show_status() {
  local unit
  unit="$(service_unit)"
  remote_systemctl "status $unit --no-pager -l" || true
  echo
  remote_journal_tail
}

show_changelog() {
  if [[ -f CHANGELOG.md ]]; then
    head -40 CHANGELOG.md
  else
    echo "No CHANGELOG.md found"
  fi
}

do_deploy() {
  local ver="${1:-$(next_patch_version)}"
  build_all
  bump_version "$ver"
  prepend_changelog_entry "$ver"
  verify_systemd_service
  ensure_remote_tools
  stop_service
  push_all
  restart_service
  echo
  echo "=== Deploy complete ==="
  echo "  Version: $(current_version)"
  echo "  URL:     https://adult.olibuijr.com"
}

CMD="${1:-help}"
shift || true

case "$CMD" in
  build)       build_go ;;
  build-web)   build_web ;;
  build-all)   build_all ;;
  push)        verify_systemd_service; stop_service; push_all ;;
  push-binary) push_binary ;;
  push-web)    push_web ;;
  restart)     restart_service ;;
  status)      show_status ;;
  version)     current_version ;;
  bump)        bump_version "${1:-$(current_version)}" ;;
  changelog)   show_changelog ;;
  deploy)      do_deploy "${1:-}" ;;
  help|--help|-h) usage ;;
  *)           echo "Unknown command: $CMD"; usage; exit 1 ;;
esac
