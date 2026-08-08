variable "cloudflare_account_id" { type = string }
variable "enable_dnssec" {
  type    = bool
  default = false
}
variable "vercel_team_slug" {
  type    = string
  default = "10xdevvs-projects"
}
variable "production_reviewer_usernames" {
  type    = list(string)
  default = ["10xDeVv", "DaxManuel27"]
}
variable "dns_records" {
  description = "Complete authoritative zone inventory. Do not apply until it contains every GoDaddy record."
  type = map(object({
    name     = string
    type     = string
    content  = string
    ttl      = number
    proxied  = bool
    priority = optional(number)
  }))
}
variable "vercel_production_env" {
  type      = map(string)
  sensitive = true
}
variable "vercel_staging_env" {
  type      = map(string)
  sensitive = true
}

output "cloudflare_nameservers" { value = cloudflare_zone.hackatlantic.name_servers }
output "cloudflare_dnssec_ds" { value = try(cloudflare_zone_dnssec.hackatlantic.ds, null) }
output "vercel_project_id" { value = vercel_project.ats.id }
