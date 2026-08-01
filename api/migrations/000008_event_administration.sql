CREATE TABLE ats.activities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    slug text NOT NULL CHECK (slug <> ''),
    name text NOT NULL CHECK (name <> ''),
    starts_at timestamptz,
    ends_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT activities_cycle_slug_unique UNIQUE (cycle_id, slug),
    CONSTRAINT activities_id_cycle_unique UNIQUE (id, cycle_id),
    CONSTRAINT activities_window_check CHECK (starts_at IS NULL OR ends_at IS NULL OR starts_at < ends_at)
);

ALTER TABLE ats.checkpoints
    ADD COLUMN activity_id uuid,
    ADD CONSTRAINT checkpoints_activity_cycle_fkey
    FOREIGN KEY (activity_id, cycle_id)
    REFERENCES ats.activities(id, cycle_id)
    ON DELETE RESTRICT;

CREATE INDEX redemptions_cycle_redeemed_at_idx
    ON ats.redemptions (cycle_id, redeemed_at DESC, id DESC);
CREATE INDEX redemptions_checkpoint_redeemed_at_idx
    ON ats.redemptions (checkpoint_id, redeemed_at DESC, id DESC);

REVOKE ALL ON TABLE ats.activities FROM PUBLIC;
DO $$
DECLARE
    role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('REVOKE ALL ON TABLE ats.activities FROM %I', role_name);
        END IF;
    END LOOP;
END;
$$;
