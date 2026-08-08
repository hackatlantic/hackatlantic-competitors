terraform {
  required_version = ">= 1.11.0"
  backend "remote" {}
  required_providers {
    cloudflare = { source = "cloudflare/cloudflare", version = "~> 5.22" }
    github     = { source = "integrations/github", version = "~> 6.13" }
    vercel     = { source = "vercel/vercel", version = "~> 5.4" }
  }
}

provider "cloudflare" {}
provider "github" { owner = "hackatlantic" }
provider "vercel" { team = var.vercel_team_slug }

