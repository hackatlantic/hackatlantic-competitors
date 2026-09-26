-- Staff credentials do not create applications, RSVP responses or attendee benefits.
CREATE TABLE ats.staff_passes (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES ats.users(id),
    event_key text NOT NULL CHECK (event_key = 'hackatlantic-2026'),
    qr_token_hash bytea NOT NULL UNIQUE CHECK (octet_length(qr_token_hash) = 32),
    issued_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    UNIQUE (user_id, event_key)
);
REVOKE ALL ON ats.staff_passes FROM PUBLIC;
DO $$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON ats.staff_passes FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
GRANT SELECT, INSERT ON ats.staff_passes TO hackatlantic_app;
REVOKE UPDATE, DELETE ON ats.staff_passes FROM hackatlantic_app;
