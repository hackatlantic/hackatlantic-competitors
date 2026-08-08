package terraform.security

import rego.v1

test_private_digest_pinned_plan_has_no_denials if {
	denials := deny with input as {
		"resource_changes": [{
			"address": "digitalocean_app.api",
			"type": "digitalocean_app",
			"change": {
				"actions": ["update"],
				"after": {"spec": [{"service": [{
					"name": "api",
					"health_check": [{"http_path": "/readyz"}],
					"image": [{"digest": "sha256:abc"}],
					"env": [],
				}]}]},
			},
		}],
	}
	count(denials) == 0
}

test_public_bucket_is_denied if {
	some message in deny with input as {
		"resource_changes": [{
			"address": "digitalocean_spaces_bucket.resumes",
			"type": "digitalocean_spaces_bucket",
			"change": {"actions": ["create"], "after": {"acl": "public-read"}},
		}],
	}
	contains(message, "must remain private")
}

test_wildcard_production_cors_is_denied if {
	some message in deny with input as {
		"resource_changes": [{
			"address": "digitalocean_app.api",
			"type": "digitalocean_app",
			"change": {
				"actions": ["update"],
				"after": {"spec": [{"service": [{
					"name": "api",
					"health_check": [{"http_path": "/readyz"}],
					"image": [{"digest": "sha256:abc"}],
					"env": [{"key": "CORS_ALLOWED_ORIGINS", "value": "*", "type": "GENERAL"}],
				}]}]},
			},
		}],
	}
	contains(message, "must not allow wildcard origins")
}
