UPDATE ats.reviews
SET submitted_at = CASE
    WHEN status = 'submitted' THEN COALESCE(submitted_at, updated_at)
    ELSE NULL
END
WHERE (status = 'submitted' AND submitted_at IS NULL)
   OR (status = 'draft' AND submitted_at IS NOT NULL);

ALTER TABLE ats.reviews
    ADD COLUMN lock_version integer NOT NULL DEFAULT 0
        CHECK (lock_version >= 0),
    ADD CONSTRAINT reviews_submission_state_check CHECK (
        (status = 'draft' AND submitted_at IS NULL)
        OR (status = 'submitted' AND submitted_at IS NOT NULL)
    );

CREATE FUNCTION ats.prevent_submitted_review_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status = 'submitted' THEN
        RAISE EXCEPTION 'submitted reviews are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER reviews_submitted_immutable
BEFORE UPDATE OR DELETE ON ats.reviews
FOR EACH ROW EXECUTE FUNCTION ats.prevent_submitted_review_mutation();

REVOKE ALL ON FUNCTION ats.prevent_submitted_review_mutation() FROM PUBLIC;
DO $$
DECLARE
    role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON FUNCTION ats.prevent_submitted_review_mutation() FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
