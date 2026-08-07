#!/usr/bin/env bash
set -euo pipefail

: "${ALERT_MESSAGE:?ALERT_MESSAGE is required}"

if [[ -n "${DISCORD_WEBHOOK_URL:-}" ]]; then
  jq -n --arg content "$ALERT_MESSAGE" '{content:$content}' |
    curl --fail-with-body --silent --show-error -H 'Content-Type: application/json' --data-binary @- "$DISCORD_WEBHOOK_URL"
fi

if [[ -n "${RESEND_API_KEY:-}" && -n "${ALERT_EMAIL_FROM:-}" && -n "${ADMIN_ALERT_EMAILS:-}" ]]; then
  jq -n \
    --arg from "$ALERT_EMAIL_FROM" \
    --arg emails "$ADMIN_ALERT_EMAILS" \
    --arg subject "[HackAtlantic] Platform alert" \
    --arg text "$ALERT_MESSAGE" \
    '{from:$from,to:($emails|split(",")|map(gsub("^\\s+|\\s+$";""))),subject:$subject,text:$text}' |
    curl --fail-with-body --silent --show-error \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${RESEND_API_KEY}" \
      --data-binary @- https://api.resend.com/emails
fi

if [[ -z "${DISCORD_WEBHOOK_URL:-}" && -z "${RESEND_API_KEY:-}" ]]; then
  echo "No incident notification transport is configured" >&2
  exit 1
fi
