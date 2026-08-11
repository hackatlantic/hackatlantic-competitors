DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_roles
        WHERE rolname = 'hackatlantic_app'
    ) THEN
        CREATE ROLE hackatlantic_app NOLOGIN;
    END IF;
END;
$$;

-- The API authenticates with the migration-owner connection through the
-- Supabase IPv4 session pooler, then assumes this restricted role on every
-- pooled connection. Explicit membership is required because Supabase's
-- project postgres role is intentionally not a PostgreSQL superuser.
DO $$
BEGIN
    EXECUTE format('GRANT hackatlantic_app TO %I', current_user);
END;
$$;

GRANT USAGE ON SCHEMA ats TO hackatlantic_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ats TO hackatlantic_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ats TO hackatlantic_app;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA ats TO hackatlantic_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA ats
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO hackatlantic_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA ats
    GRANT USAGE, SELECT ON SEQUENCES TO hackatlantic_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA ats
    GRANT EXECUTE ON FUNCTIONS TO hackatlantic_app;
