-- RSVP belongs to an acceptance, not the application forever: a replacement
-- decision starts unanswered while earlier responses remain available for audit.
CREATE TABLE ats.attendance_responses (
    decision_id uuid PRIMARY KEY REFERENCES ats.decisions(id),
    status text NOT NULL CHECK (status IN ('confirmed', 'declined')),
    lock_version integer NOT NULL DEFAULT 1 CHECK (lock_version > 0),
    responded_by uuid NOT NULL REFERENCES ats.users(id),
    responded_at timestamptz NOT NULL DEFAULT now()
);

REVOKE ALL ON TABLE ats.attendance_responses FROM PUBLIC;
DO $$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON TABLE ats.attendance_responses FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
GRANT SELECT, INSERT, UPDATE ON ats.attendance_responses TO hackatlantic_app;
REVOKE DELETE ON ats.attendance_responses FROM hackatlantic_app;
