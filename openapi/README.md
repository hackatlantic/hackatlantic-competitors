# OpenAPI

`openapi.yaml` describes the implemented Go API, not aspirational endpoints.
Update the contract in the same change that adds or changes an HTTP endpoint.

The frontend's higher-level client belongs in `lib/api.ts`. Generated types may
be introduced in a later milestone, but generated files must be reproducible.
