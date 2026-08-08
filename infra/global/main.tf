locals {
  repository = "hackatlantic-competitors"
  required_checks = [
    "Frontend",
    "Go",
    "Integration",
    "Containers",
    "Terraform",
    "Security",
    "Dependency Review",
    "CodeQL (go)",
    "CodeQL (javascript-typescript)",
  ]
}

resource "cloudflare_zone" "hackatlantic" {
  account = { id = var.cloudflare_account_id }
  name    = "hackatlantic.ca"
  type    = "full"
}

resource "cloudflare_dns_record" "managed" {
  for_each = var.dns_records

  zone_id  = cloudflare_zone.hackatlantic.id
  name     = each.value.name
  type     = each.value.type
  content  = each.value.content
  ttl      = each.value.proxied ? 1 : each.value.ttl
  proxied  = each.value.proxied
  priority = each.value.priority
  comment  = "Managed by Terraform (${each.key})"
}

resource "cloudflare_zone_dnssec" "hackatlantic" {
  zone_id = cloudflare_zone.hackatlantic.id
  status  = var.enable_dnssec ? "active" : "disabled"
}

resource "vercel_project" "ats" {
  name                                              = "hackatlantic-ats"
  framework                                         = "nextjs"
  auto_assign_custom_domains                        = false
  automatically_expose_system_environment_variables = true
  git_fork_protection                               = true

  git_repository = {
    type              = "github"
    repo              = "hackatlantic/hackatlantic-competitors"
    production_branch = "main"
  }
}

resource "vercel_project_environment_variable" "production" {
  for_each = nonsensitive(toset(keys(var.vercel_production_env)))

  project_id = vercel_project.ats.id
  key        = each.value
  value_wo   = var.vercel_production_env[each.value]
  target     = ["production"]
  sensitive  = true
  comment    = "Managed by Terraform"
}

resource "vercel_project_environment_variable" "staging" {
  for_each = nonsensitive(toset(keys(var.vercel_staging_env)))

  project_id = vercel_project.ats.id
  key        = each.value
  value_wo   = var.vercel_staging_env[each.value]
  target     = ["preview"]
  git_branch = "staging"
  sensitive  = true
  comment    = "Managed by Terraform"
}

resource "vercel_project_domain" "apply" {
  project_id = vercel_project.ats.id
  domain     = "apply.hackatlantic.ca"
}

data "github_user" "reviewer" {
  for_each = toset(var.production_reviewer_usernames)
  username = each.value
}

resource "github_branch_protection" "main" {
  repository_id                   = local.repository
  pattern                         = "main"
  enforce_admins                  = true
  allows_deletions                = false
  allows_force_pushes             = false
  require_signed_commits          = true
  required_linear_history         = true
  require_conversation_resolution = true

  required_status_checks {
    strict   = true
    contexts = local.required_checks
  }

  required_pull_request_reviews {
    dismiss_stale_reviews           = true
    require_code_owner_reviews      = true
    required_approving_review_count = 1
    require_last_push_approval      = true
  }
}

resource "github_repository_environment" "staging" {
  repository  = local.repository
  environment = "staging"
  deployment_branch_policy {
    protected_branches     = true
    custom_branch_policies = false
  }
}

resource "github_repository_environment" "backup" {
  repository  = local.repository
  environment = "backup"
  deployment_branch_policy {
    protected_branches     = true
    custom_branch_policies = false
  }
}

resource "github_repository_environment" "production" {
  repository          = local.repository
  environment         = "production"
  prevent_self_review = true
  can_admins_bypass   = false
  reviewers { users = [for reviewer in data.github_user.reviewer : reviewer.id] }
  deployment_branch_policy {
    protected_branches     = true
    custom_branch_policies = false
  }
}

resource "github_repository_environment" "disaster_recovery" {
  repository          = local.repository
  environment         = "disaster-recovery"
  prevent_self_review = true
  can_admins_bypass   = false
  reviewers { users = [for reviewer in data.github_user.reviewer : reviewer.id] }
  deployment_branch_policy {
    protected_branches     = true
    custom_branch_policies = false
  }
}
