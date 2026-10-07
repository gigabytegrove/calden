#!/bin/sh
set -eu

DATA_DIR="${CALDEN_DATA_DIR:-/data}"
RUNTIME_ROOT="${CALDEN_RUNTIME_DIR:-$DATA_DIR/.calden-runtime}"
CURRENT="$RUNTIME_ROOT/current"
PENDING="$DATA_DIR/.calden-restore-pending.tar.gz"
RESULT="$DATA_DIR/.calden-restore-result.json"
BACKUPS="$DATA_DIR/backups"

DB_HOST="${CALDEN_DB_HOST:-localhost}"
DB_PORT="${CALDEN_DB_PORT:-5432}"
DB_NAME="${CALDEN_DB_NAME:-calden}"
DB_USER="${CALDEN_DB_USER:-calden}"

database_password() {
  if [ -n "${CALDEN_DB_PASSWORD_FILE:-}" ] && [ -f "$CALDEN_DB_PASSWORD_FILE" ]; then
    cat "$CALDEN_DB_PASSWORD_FILE"
    return
  fi
  printf '%s' "${CALDEN_DB_PASSWORD:-}"
}

write_restore_result() {
  status="$1"
  safety="${2:-}"
  completed="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf '{"status":"%s","completed_at":"%s","safety_backup":"%s"}\n'     "$status" "$completed" "$safety" > "$RESULT"
  chmod 0600 "$RESULT" || true
}

apply_pending_restore() {
  [ -f "$PENDING" ] || return 0

  echo "CalDen: staged restore found; validating before application."
  mkdir -p "$BACKUPS"
  chmod 0700 "$BACKUPS" || true

  entries="$(tar -tzf "$PENDING")" || {
    echo "CalDen: restore archive could not be read; leaving current database untouched." >&2
    mv "$PENDING" "$BACKUPS/failed-restore-invalid-$(date -u '+%Y%m%d-%H%M%S').tar.gz" || true
    write_restore_result "failed_validation"
    return 0
  }

  found_manifest=0
  found_dump=0
  old_ifs="$IFS"
  IFS='
'
  for entry in $entries; do
    case "$entry" in
      /*|../*|*/../*|*/..)
        echo "CalDen: restore archive contains an unsafe path; refusing restore." >&2
        mv "$PENDING" "$BACKUPS/failed-restore-unsafe-$(date -u '+%Y%m%d-%H%M%S').tar.gz" || true
        write_restore_result "failed_validation"
        IFS="$old_ifs"
        return 0
        ;;
    esac
    [ "$entry" = "manifest.json" ] && found_manifest=1
    [ "$entry" = "calden.dump" ] && found_dump=1
  done
  IFS="$old_ifs"

  if [ "$found_manifest" -ne 1 ] || [ "$found_dump" -ne 1 ]; then
    echo "CalDen: restore archive is missing required files; refusing restore." >&2
    mv "$PENDING" "$BACKUPS/failed-restore-incomplete-$(date -u '+%Y%m%d-%H%M%S').tar.gz" || true
    write_restore_result "failed_validation"
    return 0
  fi

  work="$(mktemp -d "$DATA_DIR/.restore-work-XXXXXX")"
  trap 'rm -rf "$work"' EXIT INT TERM
  tar -xzf "$PENDING" -C "$work"

  if ! pg_restore --list "$work/calden.dump" >/dev/null 2>&1; then
    echo "CalDen: staged PostgreSQL dump failed validation; refusing restore." >&2
    mv "$PENDING" "$BACKUPS/failed-restore-dump-$(date -u '+%Y%m%d-%H%M%S').tar.gz" || true
    write_restore_result "failed_validation"
    rm -rf "$work"
    trap - EXIT INT TERM
    return 0
  fi

  stamp="$(date -u '+%Y%m%d-%H%M%S')"
  safety="$BACKUPS/pre-restore-$stamp.dump"
  password="$(database_password)"

  echo "CalDen: creating pre-restore PostgreSQL safety backup."
  if ! PGPASSWORD="$password" pg_dump     -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME"     -Fc -f "$safety"; then
    echo "CalDen: safety backup failed; refusing restore." >&2
    rm -rf "$work"
    trap - EXIT INT TERM
    write_restore_result "failed_safety_backup"
    return 0
  fi
  chmod 0600 "$safety" || true

  echo "CalDen: applying staged PostgreSQL restore."
  if PGPASSWORD="$password" pg_restore     -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME"     --clean --if-exists --no-owner --no-privileges "$work/calden.dump"; then

    if [ -d "$work/data" ]; then
      echo "CalDen: restoring persistent application data."
      cp -a "$work/data/." "$DATA_DIR/"
    fi
    rm -f "$PENDING"
    write_restore_result "success" "$(basename "$safety")"
    echo "CalDen: restore completed successfully."
  else
    echo "CalDen: restore failed; rolling PostgreSQL back to the safety backup." >&2
    if PGPASSWORD="$password" pg_restore       -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME"       --clean --if-exists --no-owner --no-privileges "$safety"; then
      mv "$PENDING" "$BACKUPS/failed-restore-$stamp.tar.gz" || true
      write_restore_result "failed_rolled_back" "$(basename "$safety")"
      echo "CalDen: original database restored after failed restore." >&2
    else
      write_restore_result "failed_rollback" "$(basename "$safety")"
      echo "CalDen: CRITICAL: restore and automatic rollback both failed. Safety backup: $safety" >&2
      exit 1
    fi
  fi

  rm -rf "$work"
  trap - EXIT INT TERM
}

apply_pending_restore

if [ -x "$CURRENT/calden" ] && [ -d "$CURRENT/web" ]; then
  export CALDEN_WEB_DIR="$CURRENT/web"
  exec "$CURRENT/calden" "$@"
fi

exec /app/calden "$@"
