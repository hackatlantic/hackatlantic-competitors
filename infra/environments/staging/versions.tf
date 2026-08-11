terraform {
  required_version = ">= 1.11.0"
  backend "remote" {}
  required_providers {
    digitalocean = { source = "digitalocean/digitalocean", version = "~> 2.96" }
    random       = { source = "hashicorp/random", version = "~> 3.7" }
    supabase     = { source = "supabase/supabase", version = "~> 1.9" }
  }
}

provider "digitalocean" {}
provider "supabase" {}
