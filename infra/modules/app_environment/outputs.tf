output "app_id" { value = digitalocean_app.api.id }
output "api_live_url" { value = digitalocean_app.api.live_url }
output "api_default_ingress" { value = digitalocean_app.api.default_ingress }
output "api_domain" { value = var.api_domain }
output "resume_bucket" { value = digitalocean_spaces_bucket.resumes.name }
output "backup_bucket" {
  value = try(digitalocean_spaces_bucket.backups[0].name, null)
}
