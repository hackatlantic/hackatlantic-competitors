# Minimal default: one dashboard, email alerts, direct OTLP metrics.
# Advanced dashboards and synthetic probes are opt-in, not release prerequisites.
resource "grafana_folder" "platform" {
  title = "HackAtlantic Platform"
  uid   = "hackatlantic-platform"
}

resource "grafana_dashboard" "api" {
  folder      = grafana_folder.platform.uid
  config_json = replace(file("${path.module}/../../observability/grafana/dashboards/api-overview.json"), "$${DS_PROMETHEUS}", var.prometheus_data_source_uid)
  overwrite   = false
}

resource "grafana_dashboard" "operations" {
  count       = var.enable_operations_dashboard ? 1 : 0
  folder      = grafana_folder.platform.uid
  config_json = replace(file("${path.module}/../../observability/grafana/dashboards/ats-operations.json"), "$${DS_PROMETHEUS}", var.prometheus_data_source_uid)
  overwrite   = false
}

resource "grafana_contact_point" "administrators" {
  name = "HackAtlantic administrators"

  dynamic "discord" {
    for_each = nonsensitive(var.discord_webhook_url != "") ? [1] : []
    content {
      url                     = var.discord_webhook_url
      title                   = "{{ template \"default.title\" . }}"
      message                 = "{{ template \"default.message\" . }}"
      disable_resolve_message = false
    }
  }

  email {
    addresses               = var.admin_alert_emails
    single_email            = true
    subject                 = "[HackAtlantic] {{ template \"default.title\" . }}"
    message                 = "{{ template \"default.message\" . }}"
    disable_resolve_message = false
  }
}

# Do not take ownership of the Grafana stack's global notification tree.
# If an earlier version was applied, leave that policy intact when releasing ownership.
removed {
  from = grafana_notification_policy.root
  lifecycle { destroy = false }
}

resource "grafana_synthetic_monitoring_check" "api" {
  count     = var.enable_synthetics ? 1 : 0
  provider  = grafana.synthetics
  job       = "HackAtlantic production API readiness"
  target    = "https://api.hackatlantic.ca/readyz"
  enabled   = true
  probes    = var.synthetic_probe_ids
  frequency = 60000
  timeout   = 5000
  labels = {
    environment = "production"
    service     = "ats-api"
  }

  settings {
    http {
      method                          = "GET"
      ip_version                      = "V4"
      fail_if_not_ssl                 = true
      valid_status_codes              = [200]
      fail_if_body_not_matches_regexp = ["ready"]
    }
  }
}

resource "grafana_synthetic_monitoring_check_alerts" "api" {
  count    = var.enable_synthetics ? 1 : 0
  provider = grafana.synthetics
  check_id = grafana_synthetic_monitoring_check.api[0].id
  alerts = [{
    name        = "ProbeFailedExecutionsTooHigh"
    threshold   = 3
    period      = "5m"
    runbook_url = "https://github.com/hackatlantic/hackatlantic-competitors/blob/main/docs/runbooks/incident-response.md"
  }]
}

resource "grafana_synthetic_monitoring_check" "frontend" {
  count     = var.enable_synthetics ? 1 : 0
  provider  = grafana.synthetics
  job       = "HackAtlantic applicant portal"
  target    = "https://apply.hackatlantic.ca/"
  enabled   = true
  probes    = var.synthetic_probe_ids
  frequency = 60000
  timeout   = 5000
  labels = {
    environment = "production"
    service     = "ats-web"
  }

  settings {
    http {
      method             = "GET"
      ip_version         = "V4"
      fail_if_not_ssl    = true
      valid_status_codes = [200]
    }
  }
}

resource "grafana_synthetic_monitoring_check_alerts" "frontend" {
  count    = var.enable_synthetics ? 1 : 0
  provider = grafana.synthetics
  check_id = grafana_synthetic_monitoring_check.frontend[0].id
  alerts = [{
    name        = "ProbeFailedExecutionsTooHigh"
    threshold   = 3
    period      = "5m"
    runbook_url = "https://github.com/hackatlantic/hackatlantic-competitors/blob/main/docs/runbooks/incident-response.md"
  }]
}


locals {
  production = "deployment_environment_name=\"production\",service_name=\"hackatlantic-ats-api\""
  traffic    = "http_server_requests_total{${local.production}}"
  failures   = "http_server_requests_total{${local.production},http_response_status_class=\"5xx\"}"
  scanner    = "http_server_duration_milliseconds_bucket{${local.production},http_route=\"POST /v1/redemptions\"}"

  alerts = {
    errors = {
      name      = "Production API sustained server errors"
      severity  = "critical"
      summary   = "Over 5% server errors, at least 5 errors and 20 requests in 5 minutes. Inspect API logs and the last deployment."
      expr      = "(sum(increase(${local.failures}[5m])) / clamp_min(sum(increase(${local.traffic}[5m])), 1) > bool 0.05) * (sum(increase(${local.failures}[5m])) >= bool 5) * (sum(increase(${local.traffic}[5m])) >= bool 20) or vector(0)"
      hold      = "5m"
      threshold = 0
    }
    scanner_latency = {
      name      = "Scanner redemption p95 exceeds 1000ms"
      severity  = "warning"
      summary   = "Scanner p95 exceeds the realistic release-gate ceiling with at least 20 redemptions in 5 minutes. Check database pool usage and recent releases."
      expr      = "(histogram_quantile(0.95, sum by (le) (rate(${local.scanner}[5m]))) > bool 1000) * (sum(increase(http_server_duration_milliseconds_count{${local.production},http_route=\"POST /v1/redemptions\"}[5m])) >= bool 20) or vector(0)"
      hold      = "5m"
      threshold = 0
    }
    telemetry_missing = {
      name      = "Production API telemetry missing"
      severity  = "critical"
      summary   = "No API heartbeat for 10 minutes. Check /readyz, DigitalOcean deployment status, and OTLP credentials. Missing telemetry does not by itself prove an outage."
      expr      = "sum(absent_over_time(hackatlantic_telemetry_up{${local.production}}[10m])) or vector(0)"
      hold      = "0s"
      threshold = 0
    }
  }
}

resource "grafana_rule_group" "production_slos" {
  name             = "HackAtlantic production SLOs"
  folder_uid       = grafana_folder.platform.uid
  interval_seconds = 60

  dynamic "rule" {
    for_each = local.alerts
    content {
      name           = rule.value.name
      is_paused      = !var.enable_alerts
      condition      = "C"
      for            = rule.value.hold
      no_data_state  = "OK"
      exec_err_state = "Error"
      annotations = {
        summary          = rule.value.summary
        runbook_url      = "https://github.com/hackatlantic/hackatlantic-competitors/blob/main/docs/runbooks/grafana-essentials.md"
        __dashboardUid__ = "hackatlantic-api"
        __panelId__      = "1"
      }
      labels = { severity = rule.value.severity, service = "ats-api", deployment_environment_name = "production" }

      notification_settings {
        contact_point   = grafana_contact_point.administrators.name
        group_by        = ["alertname", "grafana_folder"]
        group_wait      = "30s"
        group_interval  = "5m"
        repeat_interval = "4h"
      }

      data {
        ref_id         = "A"
        datasource_uid = var.prometheus_data_source_uid
        relative_time_range {
          from = 600
          to   = 0
        }
        model = jsonencode({
          refId      = "A", expr = rule.value.expr, instant = true,
          intervalMs = 15000, maxDataPoints = 43200
        })
      }
      data {
        ref_id         = "B"
        datasource_uid = "__expr__"
        relative_time_range {
          from = 0
          to   = 0
        }
        model = jsonencode({ refId = "B", type = "reduce", expression = "A", reducer = "last" })
      }
      data {
        ref_id         = "C"
        datasource_uid = "__expr__"
        relative_time_range {
          from = 0
          to   = 0
        }
        model = jsonencode({
          refId      = "C", type = "threshold", expression = "B",
          conditions = [{ evaluator = { params = [rule.value.threshold], type = "gt" }, operator = { type = "and" }, query = { params = ["C"] }, reducer = { params = [], type = "last" }, type = "query" }]
        })
      }
    }
  }
}

# Preserve addresses when opting existing resources into the smaller setup.
moved {
  from = grafana_dashboard.operations
  to   = grafana_dashboard.operations[0]
}
moved {
  from = grafana_synthetic_monitoring_check.api
  to   = grafana_synthetic_monitoring_check.api[0]
}
moved {
  from = grafana_synthetic_monitoring_check.frontend
  to   = grafana_synthetic_monitoring_check.frontend[0]
}
moved {
  from = grafana_synthetic_monitoring_check_alerts.api
  to   = grafana_synthetic_monitoring_check_alerts.api[0]
}
moved {
  from = grafana_synthetic_monitoring_check_alerts.frontend
  to   = grafana_synthetic_monitoring_check_alerts.frontend[0]
}

output "api_dashboard_url" { value = grafana_dashboard.api.url }
output "operations_dashboard_url" { value = try(grafana_dashboard.operations[0].url, null) }
