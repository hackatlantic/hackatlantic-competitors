CREATE TABLE ats.checkpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    slug text NOT NULL CHECK (slug <> ''),
    name text NOT NULL CHECK (name <> ''),
    opens_at timestamptz,
    closes_at timestamptz,
    default_allowed boolean NOT NULL DEFAULT false,
    default_max_redemptions integer NOT NULL DEFAULT 0 CHECK (default_max_redemptions >= 0),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT checkpoints_cycle_slug_unique UNIQUE (cycle_id, slug),
    CONSTRAINT checkpoints_id_cycle_unique UNIQUE (id, cycle_id),
    CONSTRAINT checkpoints_window_check CHECK (opens_at IS NULL OR closes_at IS NULL OR opens_at < closes_at)
);

ALTER TABLE ats.attendees
    ADD CONSTRAINT attendees_id_cycle_unique UNIQUE (id, cycle_id);

CREATE TABLE ats.attendee_entitlements (
    attendee_id uuid NOT NULL,
    checkpoint_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    allowed boolean NOT NULL,
    max_redemptions integer NOT NULL CHECK (max_redemptions >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (attendee_id, checkpoint_id),
    CONSTRAINT attendee_entitlements_attendee_cycle_fkey
        FOREIGN KEY (attendee_id, cycle_id)
        REFERENCES ats.attendees(id, cycle_id),
    CONSTRAINT attendee_entitlements_checkpoint_cycle_fkey
        FOREIGN KEY (checkpoint_id, cycle_id)
        REFERENCES ats.checkpoints(id, cycle_id)
);

CREATE TABLE ats.redemption_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL UNIQUE,
    scanner_user_id uuid NOT NULL REFERENCES ats.users(id),
    checkpoint_id uuid NOT NULL REFERENCES ats.checkpoints(id),
    qr_token_hash bytea NOT NULL CHECK (octet_length(qr_token_hash) = 32),
    pass_id uuid REFERENCES ats.passes(id),
    attendee_id uuid REFERENCES ats.attendees(id),
    outcome text NOT NULL CHECK (outcome IN ('redeemed', 'already_exhausted', 'not_entitled', 'outside_window', 'invalid_pass', 'revoked_pass')),
    attendee_display_name text,
    pass_status text,
    checkpoint_name text NOT NULL,
    redemption_id uuid UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT redemption_requests_outcome_subject_check CHECK (
        (outcome = 'invalid_pass' AND pass_id IS NULL AND attendee_id IS NULL AND attendee_display_name IS NULL AND pass_status IS NULL AND redemption_id IS NULL)
        OR (outcome = 'redeemed' AND pass_id IS NOT NULL AND attendee_id IS NOT NULL AND attendee_display_name IS NOT NULL AND pass_status = 'active' AND redemption_id IS NOT NULL)
        OR (outcome = 'revoked_pass' AND pass_id IS NOT NULL AND attendee_id IS NOT NULL AND attendee_display_name IS NOT NULL AND pass_status = 'revoked' AND redemption_id IS NULL)
        OR (outcome IN ('already_exhausted', 'not_entitled', 'outside_window') AND pass_id IS NOT NULL AND attendee_id IS NOT NULL AND attendee_display_name IS NOT NULL AND pass_status = 'active' AND redemption_id IS NULL)
    ),
    CONSTRAINT redemption_requests_pass_attendee_fkey
        FOREIGN KEY (pass_id, attendee_id)
        REFERENCES ats.passes(id, attendee_id)
);

CREATE TABLE ats.redemptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pass_id uuid NOT NULL,
    attendee_id uuid NOT NULL,
    checkpoint_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    scanner_user_id uuid NOT NULL REFERENCES ats.users(id),
    idempotency_key uuid NOT NULL UNIQUE,
    redeemed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT redemptions_pass_attendee_fkey
        FOREIGN KEY (pass_id, attendee_id)
        REFERENCES ats.passes(id, attendee_id),
    CONSTRAINT redemptions_attendee_cycle_fkey
        FOREIGN KEY (attendee_id, cycle_id)
        REFERENCES ats.attendees(id, cycle_id),
    CONSTRAINT redemptions_checkpoint_cycle_fkey
        FOREIGN KEY (checkpoint_id, cycle_id)
        REFERENCES ats.checkpoints(id, cycle_id),
    CONSTRAINT redemptions_attendee_checkpoint_ordinal_unique UNIQUE (attendee_id, checkpoint_id, ordinal)
);

ALTER TABLE ats.redemption_requests
    ADD CONSTRAINT redemption_requests_redemption_fkey
    FOREIGN KEY (redemption_id)
    REFERENCES ats.redemptions(id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX checkpoints_active_cycle_idx ON ats.checkpoints (cycle_id, name) WHERE active;
CREATE INDEX attendee_entitlements_checkpoint_idx ON ats.attendee_entitlements (checkpoint_id, attendee_id);
CREATE INDEX redemptions_attendee_checkpoint_idx ON ats.redemptions (attendee_id, checkpoint_id, ordinal);
CREATE INDEX redemption_requests_scanner_created_idx ON ats.redemption_requests (scanner_user_id, created_at DESC);

CREATE FUNCTION ats.prevent_redemption_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'redemption history is append-only';
END;
$$;

CREATE TRIGGER redemptions_append_only
BEFORE UPDATE OR DELETE ON ats.redemptions
FOR EACH ROW EXECUTE FUNCTION ats.prevent_redemption_history_mutation();

CREATE TRIGGER redemption_requests_append_only
BEFORE UPDATE OR DELETE ON ats.redemption_requests
FOR EACH ROW EXECUTE FUNCTION ats.prevent_redemption_history_mutation();

REVOKE ALL ON TABLE ats.checkpoints, ats.attendee_entitlements, ats.redemption_requests, ats.redemptions FROM PUBLIC;
REVOKE ALL ON FUNCTION ats.prevent_redemption_history_mutation() FROM PUBLIC;
DO $$
DECLARE
    role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON TABLE ats.checkpoints, ats.attendee_entitlements, ats.redemption_requests, ats.redemptions FROM %I', role_name);
            EXECUTE format('REVOKE ALL ON FUNCTION ats.prevent_redemption_history_mutation() FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
