CREATE SCHEMA IF NOT EXISTS ats;

DO $$
DECLARE
    object_name text;
BEGIN
    FOREACH object_name IN ARRAY ARRAY[
        'users',
        'user_roles',
        'application_cycles',
        'application_forms',
        'applications',
        'application_answers',
        'review_assignments',
        'reviews',
        'decisions',
        'audit_events',
        'email_outbox',
        'schema_migrations'
    ] LOOP
        IF to_regclass(format('public.%I', object_name)) IS NOT NULL THEN
            EXECUTE format('ALTER TABLE public.%I SET SCHEMA ats', object_name);
        END IF;
    END LOOP;
END;
$$;

DO $$
BEGIN
    IF to_regprocedure('public.prevent_non_draft_application_answers()') IS NOT NULL THEN
        ALTER FUNCTION public.prevent_non_draft_application_answers() SET SCHEMA ats;
    END IF;
    IF to_regprocedure('public.prevent_immutable_form_schema_change()') IS NOT NULL THEN
        ALTER FUNCTION public.prevent_immutable_form_schema_change() SET SCHEMA ats;
    END IF;
END;
$$;

ALTER TABLE ats.schema_migrations ADD COLUMN IF NOT EXISTS checksum text;

CREATE OR REPLACE FUNCTION ats.prevent_non_draft_application_answers()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, ats
AS $$
DECLARE
    application_status text;
BEGIN
    SELECT status INTO application_status
    FROM ats.applications
    WHERE id = COALESCE(NEW.application_id, OLD.application_id);

    IF application_status IS DISTINCT FROM 'draft' THEN
        RAISE EXCEPTION 'application answers are editable only while the application is a draft';
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE OR REPLACE FUNCTION ats.prevent_immutable_form_schema_change()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, ats
AS $$
BEGIN
    IF (OLD.published_at IS NOT NULL AND NEW.published_at IS NULL)
       OR (
           NEW.schema_json IS DISTINCT FROM OLD.schema_json
           AND (OLD.published_at IS NOT NULL OR EXISTS (SELECT 1 FROM ats.applications WHERE form_id = OLD.id))
       ) THEN
        RAISE EXCEPTION 'a published or referenced application form schema is immutable';
    END IF;

    RETURN NEW;
END;
$$;

REVOKE ALL ON SCHEMA ats FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA ats FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA ats FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA ats FROM PUBLIC;
ALTER DEFAULT PRIVILEGES IN SCHEMA ats REVOKE ALL ON TABLES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES IN SCHEMA ats REVOKE ALL ON SEQUENCES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;

DO $$
DECLARE
    role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON SCHEMA ats FROM %I', role_name);
            EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA ats FROM %I', role_name);
            EXECUTE format('REVOKE ALL ON ALL SEQUENCES IN SCHEMA ats FROM %I', role_name);
            EXECUTE format('REVOKE ALL ON ALL FUNCTIONS IN SCHEMA ats FROM %I', role_name);
            EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA ats REVOKE ALL ON TABLES FROM %I', role_name);
            EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA ats REVOKE ALL ON SEQUENCES FROM %I', role_name);
            EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA ats REVOKE EXECUTE ON FUNCTIONS FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
