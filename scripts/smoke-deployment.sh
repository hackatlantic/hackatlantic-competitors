#!/usr/bin/env bash
set -euo pipefail

: "${API_BASE_URL:?API_BASE_URL is required}"
: "${EXPECTED_GIT_SHA:?EXPECTED_GIT_SHA is required}"

for attempt in {1..30}; do
  if ready="$(curl --fail --silent --show-error --max-time 5 "${API_BASE_URL}/readyz" 2>/dev/null)" \
    && [[ "${ready}" == *'"status":"ready"'* ]]; then
    break
  fi
  if [[ "${attempt}" == 30 ]]; then
    echo "API did not become ready within five minutes" >&2
    exit 1
  fi
  sleep 10
done

version="$(curl --fail --silent --show-error --max-time 5 "${API_BASE_URL}/versionz")"
actual_sha="$(jq -r '.gitSha' <<<"${version}")"
if [[ "${actual_sha}" != "${EXPECTED_GIT_SHA}" ]]; then
  echo "deployed SHA mismatch: expected ${EXPECTED_GIT_SHA}, got ${actual_sha}" >&2
  exit 1
fi

echo "Deployment is ready and reports the expected immutable build."

