variable "grafana_url" { type = string }
variable "grafana_service_account_token" {
  type      = string
  sensitive = true
}
variable "prometheus_data_source_uid" { type = string }
variable "discord_webhook_url" {
  type      = string
  sensitive = true
}
variable "admin_alert_emails" { type = list(string) }
variable "grafana_sm_access_token" {
  type      = string
  sensitive = true
}
variable "grafana_sm_url" { type = string }
variable "synthetic_probe_ids" { type = list(number) }
