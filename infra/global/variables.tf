variable "manage_cloudflare" {
  description = "Enable only after the complete authoritative DNS inventory has passed parity review."
  type        = bool
  default     = false
}
variable "cloudflare_account_id" {
  type     = string
  default  = null
  nullable = true

  validation {
    condition     = !var.manage_cloudflare || var.cloudflare_account_id != null
    error_message = "cloudflare_account_id is required when manage_cloudflare is true."
  }
}
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
  default = {}
}
variable "vercel_production_env" {
  type      = map(string)
  sensitive = true
  default   = {}
}
variable "vercel_staging_env" {
  type      = map(string)
  sensitive = true
  default   = {}
}

output "cloudflare_nameservers" { value = try(cloudflare_zone.hackatlantic[0].name_servers, []) }
output "cloudflare_dnssec_ds" { value = try(cloudflare_zone_dnssec.hackatlantic[0].ds, null) }
output "vercel_project_id" { value = vercel_project.ats.id }
