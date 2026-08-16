#!/usr/bin/env bash
set -euo pipefail

: "${ALERT_MESSAGE:?ALERT_MESSAGE is required}"

configured=0
failures=0

if [[ -n "${DISCORD_WEBHOOK_URL:-}" ]]; then
  configured=$((configured + 1))
  if ! jq -n --arg content "$ALERT_MESSAGE" '{content:$content}' |
    curl --fail-with-body --silent --show-error -H 'Content-Type: application/json' --data-binary @- "$DISCORD_WEBHOOK_URL"; then
    echo "Discord incident notification failed" >&2
    failures=$((failures + 1))
  fi
fi

if [[ -n "${RESEND_API_KEY:-}" && -n "${ALERT_EMAIL_FROM:-}" && -n "${ADMIN_ALERT_EMAILS:-}" ]]; then
  configured=$((configured + 1))
  if ! jq -n \
    --arg from "$ALERT_EMAIL_FROM" \
    --arg emails "$ADMIN_ALERT_EMAILS" \
    --arg subject "[HackAtlantic] Platform alert" \
    --arg text "$ALERT_MESSAGE" \
    '{from:$from,to:($emails|split(",")|map(gsub("^\\s+|\\s+$";""))),subject:$subject,text:$text}' |
    curl --fail-with-body --silent --show-error \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${RESEND_API_KEY}" \
      --data-binary @- https://api.resend.com/emails; then
    echo "Email incident notification failed" >&2
    failures=$((failures + 1))
  fi
fi

if [[ "$configured" -eq 0 ]]; then
  echo "No incident notification transport is configured" >&2
  exit 1
fi

if [[ "$failures" -ne 0 ]]; then
  echo "${failures} configured incident notification transport(s) failed" >&2
  exit 1
fi

echo "Delivered incident notification through ${configured} configured transport(s)."
