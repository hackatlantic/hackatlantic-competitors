terraform {
  required_version = ">= 1.11.0, < 2.0.0"

  required_providers {
    grafana = {
      source  = "grafana/grafana"
      version = "~> 4.40"
    }
  }

  backend "remote" {}
}

provider "grafana" {
  url  = var.grafana_url
  auth = var.grafana_service_account_token
}

provider "grafana" {
  alias           = "synthetics"
  sm_access_token = var.grafana_sm_access_token
  sm_url          = var.grafana_sm_url
}
