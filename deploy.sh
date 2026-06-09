#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

BINARY="karaxxx"
SERVER="root@192.168.8.4"
SERVER_DIR="/opt/karaxxx"
SERVICE="karaxxx"
VERSION_FILE="VERSION"

usage() {
  cat <<'EOF'
Usage: ./deploy.sh <command> [version]

Commands:
  build         Build the Go binary
  build-web     Build the React frontend
  build-all     Build Go binary + React frontend
  push          Push binary + web build to server
  push-web      Push only web frontend to server
  restart       Restart the karaxxx service
  deploy [ver]  Full deploy: build-all + push + restart (optionally bump version)
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

bump_version() {
  local v="$1"
  echo "$v" > "$VERSION_FILE"
  echo "Version bumped to $v"
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

push_binary() {
  echo "=== Pushing to $SERVER ==="
  ssh "$SERVER" "systemctl stop $SERVICE" || true
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

push_all() {
  push_binary
  push_web
}

restart_service() {
  echo "=== Restarting $SERVICE ==="
  ssh "$SERVER" "systemctl restart $SERVICE"
  echo "--- Status ---"
  ssh "$SERVER" "systemctl status $SERVICE --no-pager -l" || true
}

show_status() {
  ssh "$SERVER" "systemctl status $SERVICE --no-pager -l" || true
  echo
  ssh "$SERVER" "journalctl -u $SERVICE --no-pager -n 10" || true
}

show_changelog() {
  if [[ -f CHANGELOG.md ]]; then
    head -40 CHANGELOG.md
  else
    echo "No CHANGELOG.md found"
  fi
}

do_deploy() {
  local ver="${1:-}"
  if [[ -n "$ver" ]]; then
    bump_version "$ver"
  fi
  build_all
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
  push)        push_all ;;
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
