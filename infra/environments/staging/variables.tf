variable "digitalocean_region" {
  type    = string
  default = "tor"
}
variable "spaces_region" {
  type    = string
  default = "tor1"
}
variable "api_image_digest" { type = string }
variable "supabase_organization_id" { type = string }
variable "supabase_region" {
  type    = string
  default = "ca-central-1"
}
variable "supabase_database_password" {
  type      = string
  sensitive = true
}
variable "api_env" {
  type      = map(string)
  sensitive = true
}
variable "alert_emails" {
  description = "Deprecated compatibility input retained while protected TFVARS are rotated. DigitalOcean uptime alerts use digitalocean_alert_emails."
  type        = list(string)
  default     = []
}
variable "digitalocean_alert_emails" {
  description = "Email addresses verified on the DigitalOcean account and eligible for uptime notifications."
  type        = list(string)
  default     = ["dacodegen@gmail.com"]
}

output "api_app_id" { value = module.platform.app_id }
output "api_url" { value = "https://${module.platform.api_domain}" }
output "api_live_url" { value = module.platform.api_live_url }
output "api_default_ingress" { value = module.platform.api_default_ingress }
output "supabase_project_ref" { value = supabase_project.database.id }
output "load_test_auth_secret" {
  value     = random_id.load_test_auth_secret.b64_std
  sensitive = true
}
