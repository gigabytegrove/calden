#!/bin/sh
set -eu

RUNTIME_ROOT="${CALDEN_RUNTIME_DIR:-/data/.calden-runtime}"
CURRENT="$RUNTIME_ROOT/current"

if [ -x "$CURRENT/calden" ] && [ -d "$CURRENT/web" ]; then
  export CALDEN_WEB_DIR="$CURRENT/web"
  exec "$CURRENT/calden" "$@"
fi

exec /app/calden "$@"
