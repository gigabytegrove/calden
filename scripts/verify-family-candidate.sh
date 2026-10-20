#!/usr/bin/env bash
# Verify development candidates without changing a running Calden installation.
set -Eeuo pipefail
cd "$(dirname "$0")/.."
echo "=== Calden family platform: static client and Go unit validation ==="
for executable in go node npm; do
  if ! command -v "$executable" >/dev/null 2>&1; then
    echo "Required tool not installed: $executable" >&2
    exit 2
  fi
done
echo "Go: $(go version)"
echo "Node: $(node --version)"
echo "NPM: $(npm --version)"
echo "=== Go tests ==="
go test ./...
echo "=== JS regression tests ==="
npm install --no-audit --no-fund
npm run test:web
echo "=== Tailwind web build ==="
npm run build:css
echo "PASS: source/unit checks completed"
echo "IMPORTANT: PostgreSQL migration, browser automation, Android and production smoke tests remain separate acceptance gates."
