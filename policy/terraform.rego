package terraform.security

import rego.v1

protected_types := {
	"digitalocean_app",
	"digitalocean_spaces_bucket",
	"supabase_project",
	"vercel_project",
}

deny contains message if {
	change := input.resource_changes[_]
	change.type in protected_types
	"delete" in change.change.actions
	message := sprintf("protected resource %s may not be deleted", [change.address])
}

deny contains message if {
	change := input.resource_changes[_]
	change.type == "digitalocean_spaces_bucket"
	change.change.after.acl != "private"
	message := sprintf("Spaces bucket %s must remain private", [change.address])
}

deny contains message if {
	change := input.resource_changes[_]
	change.type == "digitalocean_app"
	spec := change.change.after.spec[_]
	component := array.concat(object.get(spec, "service", []), object.get(spec, "job", []))[_]
	image := component.image[_]
	object.get(image, "tag", null) != null
	message := sprintf("App component %s must use an image digest, not a tag", [component.name])
}

deny contains message if {
	change := input.resource_changes[_]
	change.type == "digitalocean_app"
	spec := change.change.after.spec[_]
	service := spec.service[_]
	count(object.get(service, "health_check", [])) == 0
	message := sprintf("App service %s is missing a readiness health check", [service.name])
}

deny contains message if {
	change := input.resource_changes[_]
	change.type == "digitalocean_app"
	spec := change.change.after.spec[_]
	component := array.concat(object.get(spec, "service", []), object.get(spec, "job", []))[_]
	env := component.env[_]
	contains(upper(env.key), "SECRET")
	env.type != "SECRET"
	message := sprintf("environment variable %s must use encrypted App Platform storage", [env.key])
}

deny contains message if {
	change := input.resource_changes[_]
	change.type == "digitalocean_app"
	spec := change.change.after.spec[_]
	component := array.concat(object.get(spec, "service", []), object.get(spec, "job", []))[_]
	env := component.env[_]
	upper(env.key) == "CORS_ALLOWED_ORIGINS"
	contains(env.value, "*")
	message := sprintf("environment variable %s must not allow wildcard origins", [env.key])
}
