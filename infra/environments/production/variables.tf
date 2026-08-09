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
variable "alert_emails" {
  type    = list(string)
  default = []
}

output "api_app_id" { value = module.platform.app_id }
output "api_url" { value = "https://${module.platform.api_domain}" }
output "backup_bucket" { value = module.platform.backup_bucket }
output "supabase_project_ref" { value = supabase_project.database.id }
