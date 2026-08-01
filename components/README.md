# Components

Shared React components belong here. Route-specific components should remain
near their route until they are reused.

Components must call the Go API through `lib/api.ts`; they must not import a
PostgreSQL or managed-database client.
