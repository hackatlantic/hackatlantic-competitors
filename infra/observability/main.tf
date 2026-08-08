resource "grafana_folder" "platform" {
  title = "HackAtlantic Platform"
  uid   = "hackatlantic-platform"
}

resource "grafana_dashboard" "api" {
  folder      = grafana_folder.platform.uid
  config_json = file("${path.module}/../../observability/grafana/dashboards/api-overview.json")
  overwrite   = true
}

resource "grafana_dashboard" "operations" {
  folder      = grafana_folder.platform.uid
  config_json = file("${path.module}/../../observability/grafana/dashboards/ats-operations.json")
  overwrite   = true
}

resource "grafana_contact_point" "administrators" {
  name = "HackAtlantic administrators"

  discord {
    url                     = var.discord_webhook_url
    title                   = "{{ template \"default.title\" . }}"
    message                 = "{{ template \"default.message\" . }}"
    disable_resolve_message = false
  }

  email {
    addresses               = var.admin_alert_emails
    single_email            = true
    subject                 = "[HackAtlantic] {{ template \"default.title\" . }}"
    message                 = "{{ template \"default.message\" . }}"
    disable_resolve_message = false
  }
}

resource "grafana_notification_policy" "root" {
  contact_point   = grafana_contact_point.administrators.name
  group_by        = ["alertname", "deployment_environment_name"]
  group_wait      = "30s"
  group_interval  = "5m"
  repeat_interval = "4h"
}

resource "grafana_synthetic_monitoring_check" "api" {
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
  provider = grafana.synthetics
  check_id = grafana_synthetic_monitoring_check.api.id
  alerts = [{
    name        = "ProbeFailedExecutionsTooHigh"
    threshold   = 3
    period      = "5m"
    runbook_url = "https://github.com/hackatlantic/hackatlantic-competitors/blob/main/docs/runbooks/incident-response.md"
  }]
}

resource "grafana_synthetic_monitoring_check" "frontend" {
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
  provider = grafana.synthetics
  check_id = grafana_synthetic_monitoring_check.frontend.id
  alerts = [{
    name        = "ProbeFailedExecutionsTooHigh"
    threshold   = 3
    period      = "5m"
    runbook_url = "https://github.com/hackatlantic/hackatlantic-competitors/blob/main/docs/runbooks/incident-response.md"
  }]
}

resource "grafana_rule_group" "production_slos" {
  name             = "HackAtlantic production SLOs"
  folder_uid       = grafana_folder.platform.uid
  interval_seconds = 60

  rule {
    name           = "Production API 5xx rate exceeds 5%"
    for            = "5m"
    condition      = "C"
    no_data_state  = "NoData"
    exec_err_state = "Error"
    annotations = {
      summary = "Production API error budget is burning"
      runbook = "docs/runbooks/incident-response.md"
    }
    labels = { severity = "critical", service = "ats-api" }

    data {
      ref_id         = "A"
      datasource_uid = var.prometheus_data_source_uid
      relative_time_range {
        from = 600
        to   = 0
      }
      model = jsonencode({
        refId         = "A"
        expr          = "sum(rate(http_server_requests_total{deployment_environment_name=\"production\",http_response_status_class=\"5xx\"}[5m])) / clamp_min(sum(rate(http_server_requests_total{deployment_environment_name=\"production\"}[5m])), 0.001)"
        instant       = true
        intervalMs    = 15000
        maxDataPoints = 43200
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
        conditions = [{ evaluator = { params = [0.05], type = "gt" }, operator = { type = "and" }, query = { params = ["C"] }, reducer = { params = [], type = "last" }, type = "query" }]
      })
    }
  }

  rule {
    name           = "Scanner redemption p95 exceeds 750ms"
    for            = "5m"
    condition      = "C"
    no_data_state  = "OK"
    exec_err_state = "Error"
    annotations = {
      summary = "Scanner redemption latency is outside its SLO"
      runbook = "docs/runbooks/incident-response.md"
    }
    labels = { severity = "warning", service = "ats-api" }

    data {
      ref_id         = "A"
      datasource_uid = var.prometheus_data_source_uid
      relative_time_range {
        from = 600
        to   = 0
      }
      model = jsonencode({
        refId         = "A"
        expr          = "histogram_quantile(0.95, sum by (le) (rate(http_server_duration_milliseconds_bucket{deployment_environment_name=\"production\",http_route=\"POST /v1/redemptions\"}[5m])))"
        instant       = true
        intervalMs    = 15000
        maxDataPoints = 43200
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
        conditions = [{ evaluator = { params = [750], type = "gt" }, operator = { type = "and" }, query = { params = ["C"] }, reducer = { params = [], type = "last" }, type = "query" }]
      })
    }
  }
}

output "api_dashboard_url" { value = grafana_dashboard.api.url }
output "operations_dashboard_url" { value = grafana_dashboard.operations.url }
