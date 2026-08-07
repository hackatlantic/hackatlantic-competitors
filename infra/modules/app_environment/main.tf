resource "digitalocean_spaces_bucket" "resumes" {
  name   = var.resume_bucket_name
  region = var.spaces_region
  acl    = "private"

  versioning { enabled = true }

  lifecycle {
    prevent_destroy = true
  }
}

resource "digitalocean_spaces_bucket" "backups" {
  count  = var.backup_bucket_name == null ? 0 : 1
  name   = var.backup_bucket_name
  region = var.spaces_region
  acl    = "private"

  versioning { enabled = true }

  lifecycle_rule {
    id      = "expire-old-backup-versions"
    enabled = true
    noncurrent_version_expiration { days = 90 }
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "digitalocean_app" "api" {
  spec {
    name                            = var.app_name
    region                          = var.region
    disable_edge_cache              = true
    enhanced_threat_control_enabled = true

    domain {
      name = var.api_domain
      type = "PRIMARY"
    }

    service {
      name               = "api"
      instance_count     = 1
      instance_size_slug = var.instance_size_slug
      http_port          = 8080

      image {
        registry_type = "GHCR"
        registry      = "hackatlantic"
        repository    = "hackatlantic-api"
        digest        = var.image_digest
      }

      run_command = "/api"

      health_check {
        http_path             = "/readyz"
        initial_delay_seconds = 10
        period_seconds        = 10
        timeout_seconds       = 5
        success_threshold     = 1
        failure_threshold     = 3
      }

      liveness_health_check {
        http_path             = "/healthz"
        initial_delay_seconds = 10
        period_seconds        = 30
        timeout_seconds       = 5
        success_threshold     = 1
        failure_threshold     = 3
      }

      dynamic "env" {
        for_each = nonsensitive(toset(keys(var.api_env)))
        content {
          key   = env.value
          value = var.api_env[env.value]
          scope = "RUN_TIME"
          type  = "SECRET"
        }
      }

      alert {
        rule     = "RESTART_COUNT"
        operator = "GREATER_THAN"
        value    = 2
        window   = "FIVE_MINUTES"
      }
    }

    job {
      name               = "migrate"
      kind               = "PRE_DEPLOY"
      instance_count     = 1
      instance_size_slug = "apps-s-1vcpu-0.5gb"
      run_command        = "/migrate"

      image {
        registry_type = "GHCR"
        registry      = "hackatlantic"
        repository    = "hackatlantic-api"
        digest        = var.image_digest
      }

      env {
        key   = "DATABASE_URL"
        value = var.migration_database_url
        scope = "RUN_TIME"
        type  = "SECRET"
      }
    }

    ingress {
      rule {
        match {
          path {
            prefix = "/"
          }
        }
        component { name = "api" }
      }
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "digitalocean_uptime_check" "api" {
  name    = "${var.app_name}-ready"
  target  = "https://${var.api_domain}/readyz"
  type    = "https"
  regions = ["us_east", "eu_west"]
  enabled = true
}

resource "digitalocean_uptime_alert" "api_down" {
  count      = length(var.alert_emails) == 0 ? 0 : 1
  name       = "${var.app_name}-globally-down"
  check_id   = digitalocean_uptime_check.api.id
  type       = "down_global"
  period     = "3m"
  comparison = "greater_than"
  threshold  = 0
  notifications { email = var.alert_emails }
}
