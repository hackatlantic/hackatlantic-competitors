ALTER TABLE ats.applications
    ADD COLUMN current_decision_id uuid REFERENCES ats.decisions(id);

CREATE INDEX applications_current_decision_id_idx
    ON ats.applications (current_decision_id)
    WHERE current_decision_id IS NOT NULL;

CREATE TABLE ats.attendees (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    application_id uuid NOT NULL UNIQUE REFERENCES ats.applications(id),
    user_id uuid NOT NULL REFERENCES ats.users(id),
    display_name text NOT NULL CHECK (display_name <> ''),
    email text NOT NULL CHECK (email <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cycle_id, user_id)
);
CREATE INDEX attendees_user_id_idx ON ats.attendees (user_id);

CREATE TABLE ats.attendee_roles (
    attendee_id uuid NOT NULL REFERENCES ats.attendees(id),
    role text NOT NULL CHECK (role IN ('hacker', 'mentor', 'judge', 'sponsor')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (attendee_id, role)
);

CREATE FUNCTION ats.prevent_decision_history_mutation()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, ats
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'decisions are append-only';
    END IF;

    IF OLD.released_at IS NULL
       AND OLD.released_by IS NULL
       AND NEW.released_at IS NOT NULL
       AND NEW.released_by IS NOT NULL
       AND NEW.id IS NOT DISTINCT FROM OLD.id
       AND NEW.application_id IS NOT DISTINCT FROM OLD.application_id
       AND NEW.outcome IS NOT DISTINCT FROM OLD.outcome
       AND NEW.internal_reason IS NOT DISTINCT FROM OLD.internal_reason
       AND NEW.decided_by IS NOT DISTINCT FROM OLD.decided_by
       AND NEW.decided_at IS NOT DISTINCT FROM OLD.decided_at
       AND NEW.supersedes_id IS NOT DISTINCT FROM OLD.supersedes_id
       AND NEW.created_at IS NOT DISTINCT FROM OLD.created_at THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'decisions are append-only except for a single release transition';
END;
$$;

CREATE TRIGGER decisions_append_only
BEFORE UPDATE OR DELETE ON ats.decisions
FOR EACH ROW EXECUTE FUNCTION ats.prevent_decision_history_mutation();

REVOKE ALL ON TABLE ats.attendees, ats.attendee_roles FROM PUBLIC;
REVOKE ALL ON FUNCTION ats.prevent_decision_history_mutation() FROM PUBLIC;
DO $$
DECLARE
    role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON TABLE ats.attendees, ats.attendee_roles FROM %I', role_name);
            EXECUTE format('REVOKE ALL ON FUNCTION ats.prevent_decision_history_mutation() FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
