#!/usr/bin/env bash
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
: "${SPACES_ENDPOINT:?SPACES_ENDPOINT is required}"
: "${SPACES_REGION:?SPACES_REGION is required}"
: "${BACKUP_BUCKET:?BACKUP_BUCKET is required}"
: "${AGE_RECIPIENT:?AGE_RECIPIENT is required}"

export AWS_ACCESS_KEY_ID="${SPACES_ACCESS_KEY_ID:?SPACES_ACCESS_KEY_ID is required}"
export AWS_SECRET_ACCESS_KEY="${SPACES_SECRET_ACCESS_KEY:?SPACES_SECRET_ACCESS_KEY is required}"
export AWS_DEFAULT_REGION="$SPACES_REGION"

backup_date="$(date -u +%F)"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

dump_path="$workdir/ats-${backup_date}.dump"
encrypted_path="${dump_path}.age"

if [[ -n "${POSTGRES_CLIENT_IMAGE:-}" ]]; then
  export DUMP_FILE="$(basename "$dump_path")"
  docker run --rm \
    --env DATABASE_URL \
    --env DUMP_FILE \
    --volume "$workdir:/backup" \
    "$POSTGRES_CLIENT_IMAGE" \
    sh -euc 'pg_dump "$DATABASE_URL" --schema=ats --format=custom --compress=9 --no-owner --no-acl --file="/backup/${DUMP_FILE}"'
else
  pg_dump "$DATABASE_URL" \
    --schema=ats \
    --format=custom \
    --compress=9 \
    --no-owner \
    --no-acl \
    --file="$dump_path"
fi
test -s "$dump_path"

age --recipient "$AGE_RECIPIENT" --output "$encrypted_path" "$dump_path"
rm -f "$dump_path"
test -s "$encrypted_path"

daily_key="daily/ats-${backup_date}.dump.age"
aws --endpoint-url "$SPACES_ENDPOINT" s3 cp "$encrypted_path" "s3://${BACKUP_BUCKET}/${daily_key}" \
  --only-show-errors \
  --metadata "schema=ats,created=${backup_date}"
uploaded_size="$(aws --endpoint-url "$SPACES_ENDPOINT" s3api head-object \
  --bucket "$BACKUP_BUCKET" --key "$daily_key" --query ContentLength --output text)"
if [[ ! "$uploaded_size" =~ ^[1-9][0-9]*$ ]]; then
  echo "Uploaded backup object is missing or empty" >&2
  exit 1
fi

if [[ "$(date -u +%u)" == "7" ]]; then
  aws --endpoint-url "$SPACES_ENDPOINT" s3 cp "$encrypted_path" "s3://${BACKUP_BUCKET}/weekly/ats-${backup_date}.dump.age" --only-show-errors
fi
if [[ "$(date -u +%d)" == "01" ]]; then
  aws --endpoint-url "$SPACES_ENDPOINT" s3 cp "$encrypted_path" "s3://${BACKUP_BUCKET}/monthly/ats-${backup_date}.dump.age" --only-show-errors
fi

prune_prefix() {
  local prefix="$1"
  local retention_days="$2"
  local cutoff
  cutoff="$(date -u -d "-${retention_days} days" +%s)"
  while IFS=$'\t' read -r modified key; do
    [[ -z "${key:-}" || "$key" == "None" ]] && continue
    if (( $(date -u -d "$modified" +%s) < cutoff )); then
      aws --endpoint-url "$SPACES_ENDPOINT" s3 rm "s3://${BACKUP_BUCKET}/${key}" --only-show-errors
    fi
  done < <(aws --endpoint-url "$SPACES_ENDPOINT" s3api list-objects-v2 \
    --bucket "$BACKUP_BUCKET" --prefix "$prefix/" \
    --query 'Contents[].[LastModified,Key]' --output text)
}

prune_prefix daily 14
prune_prefix weekly 56
prune_prefix monthly 186

checksum="$(sha256sum "$encrypted_path" | cut -d ' ' -f 1)"
echo "Encrypted ATS backup uploaded: ${daily_key} (${checksum})"
