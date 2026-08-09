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
  type    = list(string)
  default = []
}

output "api_app_id" { value = module.platform.app_id }
output "api_url" { value = "https://${module.platform.api_domain}" }
output "supabase_project_ref" { value = supabase_project.database.id }
