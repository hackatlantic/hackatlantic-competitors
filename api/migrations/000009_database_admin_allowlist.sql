CREATE TABLE ats.admin_email_allowlist (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    normalized_email text NOT NULL UNIQUE,
    created_by uuid REFERENCES ats.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT admin_email_allowlist_normalized_check CHECK (
        normalized_email = lower(btrim(normalized_email))
        AND normalized_email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'
    )
);

INSERT INTO ats.admin_email_allowlist (normalized_email)
VALUES
    ('adebowale.ca@gmail.com'),
    ('swe@daxmanuel.com')
ON CONFLICT (normalized_email) DO NOTHING;
