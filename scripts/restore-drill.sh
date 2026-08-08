#!/usr/bin/env bash
set -euo pipefail

: "${RESTORE_DATABASE_URL:?RESTORE_DATABASE_URL is required}"
: "${SPACES_ENDPOINT:?SPACES_ENDPOINT is required}"
: "${SPACES_REGION:?SPACES_REGION is required}"
: "${BACKUP_BUCKET:?BACKUP_BUCKET is required}"
: "${AGE_IDENTITY:?AGE_IDENTITY is required}"

export AWS_ACCESS_KEY_ID="${SPACES_ACCESS_KEY_ID:?SPACES_ACCESS_KEY_ID is required}"
export AWS_SECRET_ACCESS_KEY="${SPACES_SECRET_ACCESS_KEY:?SPACES_SECRET_ACCESS_KEY is required}"
export AWS_DEFAULT_REGION="$SPACES_REGION"

started="$(date +%s)"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

latest_key="$(aws --endpoint-url "$SPACES_ENDPOINT" s3api list-objects-v2 \
  --bucket "$BACKUP_BUCKET" --prefix daily/ \
  --query 'reverse(sort_by(Contents,&LastModified))[0].Key' --output text)"
if [[ -z "$latest_key" || "$latest_key" == "None" ]]; then
  echo "No daily backup is available" >&2
  exit 1
fi

encrypted="$workdir/backup.dump.age"
dump="$workdir/backup.dump"
identity="$workdir/identity.txt"
printf '%s\n' "$AGE_IDENTITY" > "$identity"
chmod 600 "$identity"

aws --endpoint-url "$SPACES_ENDPOINT" s3 cp "s3://${BACKUP_BUCKET}/${latest_key}" "$encrypted" --only-show-errors
age --decrypt --identity "$identity" --output "$dump" "$encrypted"

pg_restore --clean --if-exists --no-owner --no-acl --dbname="$RESTORE_DATABASE_URL" "$dump"
DATABASE_URL="$RESTORE_DATABASE_URL" go -C api run ./cmd/migrate

schema_exists="$(psql "$RESTORE_DATABASE_URL" -Atqc "SELECT count(*) FROM information_schema.schemata WHERE schema_name = 'ats'")"
table_count="$(psql "$RESTORE_DATABASE_URL" -Atqc "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'ats'")"
migration_count="$(psql "$RESTORE_DATABASE_URL" -Atqc "SELECT count(*) FROM ats.schema_migrations")"
if [[ "$schema_exists" != "1" || "$table_count" -lt "15" || "$migration_count" -lt "11" ]]; then
  echo "Restore integrity validation failed" >&2
  exit 1
fi

elapsed="$(( $(date +%s) - started ))"
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  printf 'rto_seconds=%s\n' "$elapsed" >> "$GITHUB_OUTPUT"
fi
echo "Restore drill passed for ${latest_key}: ${table_count} ATS tables and ${migration_count} migrations in ${elapsed}s"
