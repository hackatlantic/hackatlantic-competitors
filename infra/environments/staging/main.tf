locals {
  database_url = "postgresql://postgres.${supabase_project.database.id}:${var.supabase_database_password}@aws-0-${var.supabase_region}.pooler.supabase.com:5432/postgres?sslmode=require"
  api_env = merge(var.api_env, {
    DATABASE_URL       = local.database_url
    DATABASE_ROLE      = "hackatlantic_app"
    QR_TOKEN_PEPPER    = random_id.qr_token_pepper.b64_std
    CLAIM_TOKEN_PEPPER = random_id.claim_token_pepper.b64_std
  })
}

// Pass credentials must be independent standard-base64 values containing at
// least 32 random bytes. Terraform owns the staging values so placeholder
// tfvars cannot reach the API, while HCP Terraform keeps the values encrypted
// and stable across deployments.
resource "random_id" "qr_token_pepper" {
  byte_length = 32
}

resource "random_id" "claim_token_pepper" {
  byte_length = 32
}

// The provider created this project during the initial apply but returned an
// inconsistent instance_size value before Terraform could persist it to state.
// A declarative import adopts that existing staging project idempotently.
import {
  to = supabase_project.database
  id = "ovzrhurmiwqthfgycamx"
}

resource "supabase_project" "database" {
  organization_id   = var.supabase_organization_id
  name              = "hackatlantic-ats-staging"
  database_password = var.supabase_database_password
  region            = var.supabase_region
  lifecycle { prevent_destroy = true }
}

resource "supabase_settings" "private_api" {
  project_ref = supabase_project.database.id
  api = jsonencode({
    db_schema            = "public,storage,graphql_public"
    db_extra_search_path = "public,extensions"
    max_rows             = 1000
  })
}

module "platform" {
  source = "../../modules/app_environment"

  environment                     = "staging"
  app_name                        = "hackatlantic-api-staging"
  region                          = var.digitalocean_region
  api_domain                      = "staging-api.hackatlantic.ca"
  enhanced_threat_control_enabled = false
  image_digest                    = var.api_image_digest
  instance_size_slug              = "apps-s-1vcpu-0.5gb"
  resume_bucket_name              = "hackatlantic-resumes-staging"
  spaces_region                   = var.spaces_region
  api_env                         = local.api_env
  migration_database_url          = local.database_url
  alert_emails                    = length(var.digitalocean_alert_emails) > 0 ? var.digitalocean_alert_emails : var.alert_emails
}
