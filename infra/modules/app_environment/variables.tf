variable "environment" {
  description = "Deployment environment name."
  type        = string
  validation {
    condition     = contains(["staging", "production"], var.environment)
    error_message = "environment must be staging or production."
  }
}

variable "app_name" { type = string }
variable "region" { type = string }
variable "api_domain" { type = string }
variable "enhanced_threat_control_enabled" {
  description = "Enable DigitalOcean's browser-oriented Layer 7 threat challenge. Disable only for synthetic staging traffic."
  type        = bool
  default     = true
}
variable "image_digest" {
  type = string
  validation {
    condition     = can(regex("^sha256:[0-9a-f]{64}$", var.image_digest))
    error_message = "image_digest must be an immutable sha256 OCI digest."
  }
}
variable "instance_size_slug" { type = string }
variable "resume_bucket_name" { type = string }
variable "backup_bucket_name" {
  type    = string
  default = null
}
variable "spaces_region" { type = string }
variable "api_env" {
  description = "Runtime configuration. Values are encrypted by App Platform and treated as sensitive in state."
  type        = map(string)
  sensitive   = true
}
variable "migration_database_url" {
  type      = string
  sensitive = true
}
variable "alert_emails" {
  type    = list(string)
  default = []
}
