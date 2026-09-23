# No Grafana credentials or live changes: both provider configurations are mocked.
mock_provider "grafana" {}
mock_provider "grafana" {
  alias = "synthetics"
}

variables {
  grafana_url                   = "https://example.grafana.net"
  grafana_service_account_token = "test-only"
  prometheus_data_source_uid    = "test-prometheus"
  admin_alert_emails            = ["admin@example.test"]
}

run "minimal_setup" {
  command = plan

  assert {
    condition     = length(grafana_dashboard.operations) == 0
    error_message = "The minimal setup must not create the advanced operations dashboard."
  }
  assert {
    condition     = length(grafana_synthetic_monitoring_check.api) == 0 && length(grafana_synthetic_monitoring_check.frontend) == 0
    error_message = "Synthetic infrastructure must remain opt-in."
  }
  assert {
    condition     = length(local.alerts) == 3
    error_message = "Keep the initial alert set to errors, scanner latency, and missing telemetry."
  }
  assert {
    condition     = alltrue([for rule in grafana_rule_group.production_slos.rule : rule.is_paused])
    error_message = "Bootstrap alerts must remain paused until ingestion and delivery are verified."
  }
  assert {
    condition     = alltrue([for rule in grafana_rule_group.production_slos.rule : rule.notification_settings[0].contact_point == grafana_contact_point.administrators.name])
    error_message = "Each alert must route directly to HackAtlantic without replacing the global policy."
  }
  assert {
    condition     = !strcontains(grafana_dashboard.api.config_json, "$${DS_PROMETHEUS}")
    error_message = "Provisioned dashboard must use the selected real data-source UID."
  }
}

run "activate_verified_alerts" {
  command = plan
  variables { enable_alerts = true }
  assert {
    condition     = alltrue([for rule in grafana_rule_group.production_slos.rule : !rule.is_paused])
    error_message = "Enabling verified alerts must activate all three rules."
  }
}
