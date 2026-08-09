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

# Existing control-plane resources are adopted declaratively. These imports run
# only until the objects are recorded in the HCP-backed state.
import {
  to = vercel_project.ats
  id = "prj_QrGSA0JeVhyCnar90c5MkoHIWnWC"
}

import {
  to = github_branch_protection.main
  id = "hackatlantic-competitors:main"
}

import {
  to = github_repository_environment.staging
  id = "hackatlantic-competitors:staging"
}

import {
  to = github_repository_environment.terraform_plan
  id = "hackatlantic-competitors:terraform-plan"
}

import {
  to = github_repository_environment.production
  id = "hackatlantic-competitors:Production"
}

import {
  to = github_repository_environment.disaster_recovery
  id = "hackatlantic-competitors:disaster-recovery"
}

resource "cloudflare_zone" "hackatlantic" {
  count   = var.manage_cloudflare ? 1 : 0
  account = { id = var.cloudflare_account_id }
  name    = "hackatlantic.ca"
  type    = "full"
}

resource "cloudflare_dns_record" "managed" {
  for_each = var.manage_cloudflare ? var.dns_records : {}

  zone_id  = cloudflare_zone.hackatlantic[0].id
  name     = each.value.name
  type     = each.value.type
  content  = each.value.content
  ttl      = each.value.proxied ? 1 : each.value.ttl
  proxied  = each.value.proxied
  priority = each.value.priority
  comment  = "Managed by Terraform (${each.key})"
}

resource "cloudflare_zone_dnssec" "hackatlantic" {
  count   = var.manage_cloudflare ? 1 : 0
  zone_id = cloudflare_zone.hackatlantic[0].id
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

data "github_repository" "ats" {
  full_name = "hackatlantic/hackatlantic-competitors"
}

resource "github_branch_protection" "main" {
  # The provider stores the repository GraphQL node ID after import. Using the
  # immutable node ID here adopts the existing rule without a forced replacement.
  repository_id = data.github_repository.ats.node_id
  pattern       = "main"
  # Repository administrators retain emergency ownership of the release path
  # and may merge without a second review. All non-admin contributors remain
  # subject to the required checks and CODEOWNER approval below.
  enforce_admins                  = false
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

resource "github_repository_environment" "terraform_plan" {
  repository          = local.repository
  environment         = "terraform-plan"
  prevent_self_review = false
  can_admins_bypass   = false

  reviewers {
    users = [for reviewer in data.github_user.reviewer : reviewer.id]
  }
}

resource "github_repository_environment" "terraform_drift" {
  repository  = local.repository
  environment = "terraform-drift"

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
  repository = local.repository
  # GitHub environment names retain their original case in provider state.
  # Preserve the existing environment so its secrets and approvals are adopted.
  environment         = "Production"
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
