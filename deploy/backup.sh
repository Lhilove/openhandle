#!/usr/bin/env bash
#
# Dump the production database to a compressed SQL file.
#
#   sudo bash deploy/backup.sh [destination-dir]      # default /var/backups/openhandle
#
# Schedule it, e.g. nightly at 03:15:
#   (crontab -l 2>/dev/null; echo '15 3 * * * cd /opt/openhandle && bash deploy/backup.sh >/var/log/openhandle-backup.log 2>&1') | crontab -
#
# This keeps local copies only. A backup that lives on the same disk as the
# database does not survive that disk: copy these files somewhere else
# (object storage, another host) as well.
#
set -euo pipefail

ENV_FILE="${ENV_FILE:-deploy/.env}"
CONTAINER="${CONTAINER:-openhandle-db}"
DEST="${1:-/var/backups/openhandle}"
RETAIN_DAYS="${RETAIN_DAYS:-14}"

cd "$(dirname "${BASH_SOURCE[0]}")/.."

[ -f "$ENV_FILE" ] || { printf 'error: %s not found\n' "$ENV_FILE" >&2; exit 1; }

# Read only the keys we need; never source the file.
env_value() { sed -n "s/^$2=//p" "$1" | tail -1; }
DB_USER="$(env_value "$ENV_FILE" POSTGRES_USER)"; DB_USER="${DB_USER:-openhandle}"
DB_NAME="$(env_value "$ENV_FILE" POSTGRES_DB)";   DB_NAME="${DB_NAME:-openhandle}"

docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null | grep -q true ||
  { printf 'error: container %s is not running\n' "$CONTAINER" >&2; exit 1; }

mkdir -p "$DEST"
chmod 700 "$DEST"
STAMP="$(date -u '+%Y%m%dT%H%M%SZ')"
OUT="$DEST/openhandle-$STAMP.sql.gz"

# --clean --if-exists makes the dump safe to replay over an existing database.
docker exec "$CONTAINER" pg_dump --clean --if-exists -U "$DB_USER" -d "$DB_NAME" | gzip -9 > "$OUT"
chmod 600 "$OUT"

SIZE="$(du -h "$OUT" | cut -f1)"
printf 'backup written: %s (%s)\n' "$OUT" "$SIZE"

# Retention: drop dumps older than RETAIN_DAYS.
find "$DEST" -maxdepth 1 -name 'openhandle-*.sql.gz' -type f -mtime "+$RETAIN_DAYS" -print -delete

printf '\nrestore with:\n'
printf '  gunzip -c %s | docker exec -i %s psql -U %s -d %s\n' "$OUT" "$CONTAINER" "$DB_USER" "$DB_NAME"
