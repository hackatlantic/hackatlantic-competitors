-- Requests never confer authority. Only an audited admin decision grants a role.
CREATE TABLE ats.volunteer_requests (
    user_id uuid PRIMARY KEY REFERENCES ats.users(id),
    real_name text NOT NULL CHECK (char_length(real_name) BETWEEN 2 AND 100),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_at timestamptz,
    reviewed_by uuid REFERENCES ats.users(id),
    CHECK ((status = 'pending' AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR (status <> 'pending' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL))
);
REVOKE ALL ON ats.volunteer_requests FROM PUBLIC;
DO $$
DECLARE role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON ats.volunteer_requests FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
GRANT SELECT, INSERT, UPDATE ON ats.volunteer_requests TO hackatlantic_app;
REVOKE DELETE ON ats.volunteer_requests FROM hackatlantic_app;
