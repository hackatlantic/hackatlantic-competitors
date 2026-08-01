CREATE TABLE ats.passes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attendee_id uuid NOT NULL REFERENCES ats.attendees(id),
    qr_token_hash bytea NOT NULL UNIQUE CHECK (octet_length(qr_token_hash) = 32),
    claim_token_hash bytea NOT NULL UNIQUE CHECK (octet_length(claim_token_hash) = 32),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked', 'replaced')),
    issued_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    replaced_by_pass_id uuid,
    CONSTRAINT passes_id_attendee_unique UNIQUE (id, attendee_id),
    CONSTRAINT passes_replacement_same_attendee_fkey
        FOREIGN KEY (replaced_by_pass_id, attendee_id)
        REFERENCES ats.passes(id, attendee_id)
        DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT passes_replacement_not_self_check CHECK (replaced_by_pass_id IS NULL OR replaced_by_pass_id <> id),
    CONSTRAINT passes_lifecycle_check CHECK (
        (status = 'active' AND revoked_at IS NULL AND replaced_by_pass_id IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL AND replaced_by_pass_id IS NULL)
        OR (status = 'replaced' AND revoked_at IS NOT NULL AND replaced_by_pass_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX passes_one_active_per_attendee_idx
    ON ats.passes (attendee_id)
    WHERE status = 'active';
CREATE INDEX passes_attendee_issued_at_idx
    ON ats.passes (attendee_id, issued_at DESC);

REVOKE ALL ON TABLE ats.passes FROM PUBLIC;
DO $$
DECLARE
    role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON TABLE ats.passes FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
