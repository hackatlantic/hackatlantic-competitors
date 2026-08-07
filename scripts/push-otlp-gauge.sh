#!/usr/bin/env bash
set -euo pipefail

metric_name="${1:?metric name is required}"
metric_value="${2:?metric value is required}"
metric_unit="${3:-1}"

: "${GRAFANA_OTLP_ENDPOINT:?GRAFANA_OTLP_ENDPOINT is required}"
: "${GRAFANA_OTLP_AUTHORIZATION:?GRAFANA_OTLP_AUTHORIZATION is required}"

timestamp_ns="$(($(date +%s) * 1000000000))"
payload="$(jq -n \
  --arg metric_name "$metric_name" \
  --arg metric_unit "$metric_unit" \
  --arg timestamp_ns "$timestamp_ns" \
  --arg environment "${DEPLOYMENT_ENVIRONMENT:-production}" \
  --arg workflow "${GITHUB_WORKFLOW:-manual}" \
  --argjson metric_value "$metric_value" \
  '{
    resourceMetrics: [{
      resource: {attributes: [
        {key: "service.name", value: {stringValue: "hackatlantic-platform-jobs"}},
        {key: "deployment.environment.name", value: {stringValue: $environment}}
      ]},
      scopeMetrics: [{
        scope: {name: "hackatlantic.github-actions"},
        metrics: [{
          name: $metric_name,
          unit: $metric_unit,
          gauge: {dataPoints: [{
            timeUnixNano: $timestamp_ns,
            asDouble: $metric_value,
            attributes: [{key: "workflow", value: {stringValue: $workflow}}]
          }]}
        }]
      }]
    }]
  }')"

curl --fail-with-body --silent --show-error \
  --request POST \
  --header "Authorization: ${GRAFANA_OTLP_AUTHORIZATION}" \
  --header "Content-Type: application/json" \
  --data "$payload" \
  "${GRAFANA_OTLP_ENDPOINT%/}/v1/metrics" >/dev/null

echo "Published metric ${metric_name}; no backup or applicant data was included."
