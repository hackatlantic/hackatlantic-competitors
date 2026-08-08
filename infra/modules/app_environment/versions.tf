terraform {
  required_version = ">= 1.11.0, < 2.0.0"

  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = ">= 2.96.0, < 3.0.0"
    }
  }
}
