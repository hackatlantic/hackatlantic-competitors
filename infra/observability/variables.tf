variable "grafana_url" { type = string }
variable "grafana_service_account_token" {
  type      = string
  sensitive = true
}
variable "prometheus_data_source_uid" { type = string }
variable "enable_alerts" {
  description = "Activate only after API ingestion and the contact-point delivery test are verified."
  type        = bool
  default     = false
}
variable "discord_webhook_url" {
  type      = string
  sensitive = true
  default   = ""
}
variable "admin_alert_emails" {
  type = list(string)
  validation {
    condition     = length(var.admin_alert_emails) > 0 && alltrue([for address in var.admin_alert_emails : can(regex("^[^ @]+@[^ @]+[.][^ @]+$", address))])
    error_message = "Provide at least one real administrator alert email."
  }
}
variable "grafana_sm_access_token" {
  type      = string
  sensitive = true
  default   = null
}
variable "grafana_sm_url" {
  type    = string
  default = null
}
variable "synthetic_probe_ids" {
  type    = list(number)
  default = []
  validation {
    condition     = !var.enable_synthetics || (length(var.synthetic_probe_ids) > 0 && var.grafana_sm_url != null && var.grafana_sm_access_token != null)
    error_message = "Enabling synthetic checks requires actual probe IDs, SM URL, and access token."
  }
}
variable "enable_synthetics" {
  description = "Opt in after configuring Grafana synthetic monitoring credentials. Preserve true for an already-managed installation."
  type        = bool
  default     = false
}
variable "enable_operations_dashboard" {
  description = "Opt in only once lifecycle and backup metrics are actually being sent."
  type        = bool
  default     = false
}
