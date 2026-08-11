import {
  to = supabase_project.database
  id = "oizbfvfcownivwsrzlml"
}

import {
  to = module.platform.digitalocean_app.api
  id = "852abc0e-75a9-45a8-ba13-57a5bbe50017"
}

import {
  to = module.platform.digitalocean_spaces_bucket.resumes
  id = "tor1,hackatlantic-resumes-2026"
}

resource "supabase_project" "database" {
  organization_id   = var.supabase_organization_id
  name              = var.supabase_project_name
  database_password = var.supabase_database_password
  region            = var.supabase_region
  instance_size     = var.supabase_instance_size
  lifecycle {
    prevent_destroy = true
    ignore_changes  = [database_password]
  }
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

  environment                     = "production"
  app_name                        = "hackatlantic-api"
  region                          = var.digitalocean_region
  api_domain                      = "api.hackatlantic.ca"
  enhanced_threat_control_enabled = true
  image_digest                    = var.api_image_digest
  instance_size_slug              = var.api_instance_size_slug
  resume_bucket_name              = "hackatlantic-resumes-2026"
  backup_bucket_name              = "hackatlantic-ats-backups"
  spaces_region                   = var.spaces_region
  api_env                         = var.api_env
  migration_database_url          = var.migration_database_url
  alert_emails                    = var.alert_emails
}
