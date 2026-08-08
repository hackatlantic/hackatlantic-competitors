resource "supabase_project" "database" {
  organization_id   = var.supabase_organization_id
  name              = "hackatlantic-ats-staging"
  database_password = var.supabase_database_password
  region            = var.supabase_region
  instance_size     = "micro"
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

  environment            = "staging"
  app_name               = "hackatlantic-api-staging"
  region                 = var.digitalocean_region
  api_domain             = "staging-api.hackatlantic.ca"
  image_digest           = var.api_image_digest
  instance_size_slug     = "apps-s-1vcpu-0.5gb"
  resume_bucket_name     = "hackatlantic-resumes-staging"
  spaces_region          = var.spaces_region
  api_env                = var.api_env
  migration_database_url = var.migration_database_url
  alert_emails           = var.alert_emails
}
