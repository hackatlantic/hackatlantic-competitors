variable "digitalocean_region" {
  type    = string
  default = "tor"
}
variable "spaces_region" {
  type    = string
  default = "tor1"
}
variable "api_instance_size_slug" {
  type    = string
  default = "apps-s-1vcpu-0.5gb"
}
variable "api_image_digest" { type = string }
variable "supabase_organization_id" { type = string }
variable "supabase_project_name" {
  type    = string
  default = "hackatlantic-competitors"
}
variable "supabase_region" {
  type    = string
  default = "ca-central-1"
}
variable "supabase_instance_size" {
  type    = string
  default = "micro"
}
variable "supabase_database_password" {
  type      = string
  sensitive = true
}
variable "migration_database_url" {
  type      = string
  sensitive = true
}
variable "api_env" {
  type      = map(string)
  sensitive = true
}

variable "api_observability_env" {
  description = "Dedicated, protected metrics-only exporter settings merged into the API environment without rewriting the main api_env secret payload."
  type        = map(string)
  sensitive   = true
  default     = {}

  validation {
    condition = alltrue([
      for key in keys(var.api_observability_env) : contains([
        "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
        "OTEL_EXPORTER_OTLP_METRICS_HEADERS",
        "OTEL_EXPORTER_OTLP_PROTOCOL",
        "OTEL_EXPORTER_OTLP_ENDPOINT",
        "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
        "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
      ], key)
    ])
    error_message = "api_observability_env may only set approved OTLP exporter variables."
  }
}
variable "alert_emails" {
  type    = list(string)
  default = []
}

output "api_app_id" { value = module.platform.app_id }
output "api_url" { value = "https://${module.platform.api_domain}" }
output "backup_bucket" { value = module.platform.backup_bucket }
output "supabase_project_ref" { value = supabase_project.database.id }
