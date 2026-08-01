CREATE SCHEMA IF NOT EXISTS ats;

CREATE TABLE ats.users (
    id uuid PRIMARY KEY,
    clerk_user_id text NOT NULL UNIQUE,
    primary_email text NOT NULL,
    display_name text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE ats.user_roles (
    user_id uuid NOT NULL REFERENCES ats.users(id),
    role text NOT NULL,
    created_at timestamptz NOT NULL,
    created_by uuid REFERENCES ats.users(id),
    PRIMARY KEY (user_id, role)
);

CREATE TABLE ats.admin_email_allowlist (
    id uuid PRIMARY KEY,
    normalized_email text NOT NULL UNIQUE,
    created_by uuid REFERENCES ats.users(id),
    created_at timestamptz NOT NULL
);

CREATE TABLE ats.application_cycles (
    id uuid PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    applications_open_at timestamptz NOT NULL,
    applications_close_at timestamptz NOT NULL,
    active boolean NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE ats.application_forms (
    id uuid PRIMARY KEY,
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    version integer NOT NULL,
    schema_json jsonb NOT NULL,
    published_at timestamptz,
    created_by uuid NOT NULL REFERENCES ats.users(id),
    created_at timestamptz NOT NULL,
    UNIQUE (cycle_id, version),
    UNIQUE (id, cycle_id)
);

CREATE TABLE ats.applications (
    id uuid PRIMARY KEY,
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    form_id uuid NOT NULL,
    applicant_user_id uuid NOT NULL REFERENCES ats.users(id),
    status text NOT NULL,
    submitted_at timestamptz,
    withdrawn_at timestamptz,
    current_decision text,
    current_decision_id uuid,
    decision_released_at timestamptz,
    applicant_email_snapshot text,
    lock_version integer NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (cycle_id, applicant_user_id),
    FOREIGN KEY (form_id, cycle_id) REFERENCES ats.application_forms(id, cycle_id)
);

CREATE TABLE ats.application_resumes (
    application_id uuid PRIMARY KEY REFERENCES ats.applications(id),
    object_key text NOT NULL UNIQUE,
    original_filename text NOT NULL,
    media_type text NOT NULL,
    byte_size bigint NOT NULL,
    sha256 bytea NOT NULL,
    uploaded_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE ats.application_answers (
    application_id uuid NOT NULL REFERENCES ats.applications(id),
    question_key text NOT NULL,
    value_json jsonb NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (application_id, question_key)
);

CREATE TABLE ats.review_assignments (
    application_id uuid NOT NULL REFERENCES ats.applications(id),
    reviewer_user_id uuid NOT NULL REFERENCES ats.users(id),
    assigned_by uuid NOT NULL REFERENCES ats.users(id),
    assigned_at timestamptz NOT NULL,
    PRIMARY KEY (application_id, reviewer_user_id)
);

CREATE TABLE ats.reviews (
    id uuid PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES ats.applications(id),
    reviewer_user_id uuid NOT NULL REFERENCES ats.users(id),
    status text NOT NULL,
    score_json jsonb NOT NULL,
    recommendation text,
    internal_notes text,
    submitted_at timestamptz,
    lock_version integer NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (application_id, reviewer_user_id)
);

CREATE TABLE ats.decisions (
    id uuid PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES ats.applications(id),
    outcome text NOT NULL,
    internal_reason text,
    decided_by uuid NOT NULL REFERENCES ats.users(id),
    decided_at timestamptz NOT NULL,
    released_by uuid REFERENCES ats.users(id),
    released_at timestamptz,
    supersedes_id uuid REFERENCES ats.decisions(id),
    created_at timestamptz NOT NULL
);

CREATE TABLE ats.attendees (
    id uuid PRIMARY KEY,
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    application_id uuid NOT NULL UNIQUE REFERENCES ats.applications(id),
    user_id uuid NOT NULL REFERENCES ats.users(id),
    display_name text NOT NULL,
    email text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (cycle_id, user_id)
);

CREATE TABLE ats.attendee_roles (
    attendee_id uuid NOT NULL REFERENCES ats.attendees(id),
    role text NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (attendee_id, role)
);

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

CREATE TABLE ats.audit_events (
    id uuid PRIMARY KEY,
    actor_user_id uuid REFERENCES ats.users(id),
    event_type text NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    metadata_json jsonb NOT NULL,
    request_id text,
    created_at timestamptz NOT NULL
);

CREATE TABLE ats.email_outbox (
    id uuid PRIMARY KEY,
    event_type text NOT NULL,
    recipient_user_id uuid REFERENCES ats.users(id),
    recipient_email text NOT NULL,
    template_key text NOT NULL,
    template_data_json jsonb NOT NULL,
    dedupe_key text NOT NULL UNIQUE,
    status text NOT NULL,
    attempt_count integer NOT NULL,
    available_at timestamptz NOT NULL,
    provider_message_id text,
    last_error_code text,
    sent_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX application_forms_published_versions_idx
    ON ats.application_forms (cycle_id, version DESC)
    WHERE published_at IS NOT NULL;

CREATE INDEX applications_applicant_created_at_idx
    ON ats.applications (applicant_user_id, created_at DESC);
CREATE TABLE ats.activities (
    id uuid PRIMARY KEY,
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    slug text NOT NULL,
    name text NOT NULL,
    starts_at timestamptz,
    ends_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (cycle_id, slug),
    UNIQUE (id, cycle_id),
    CHECK (starts_at IS NULL OR ends_at IS NULL OR starts_at < ends_at)
);


CREATE TABLE ats.checkpoints (
    id uuid PRIMARY KEY,
    cycle_id uuid NOT NULL REFERENCES ats.application_cycles(id),
    activity_id uuid,
    slug text NOT NULL,
    name text NOT NULL,
    opens_at timestamptz,
    closes_at timestamptz,
    default_allowed boolean NOT NULL,
    default_max_redemptions integer NOT NULL,
    active boolean NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (cycle_id, slug),
    UNIQUE (id, cycle_id),
    FOREIGN KEY (activity_id, cycle_id) REFERENCES ats.activities(id, cycle_id) ON DELETE RESTRICT
);

ALTER TABLE ats.attendees
    ADD CONSTRAINT attendees_id_cycle_unique UNIQUE (id, cycle_id);

CREATE TABLE ats.attendee_entitlements (
    attendee_id uuid NOT NULL,
    checkpoint_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    allowed boolean NOT NULL,
    max_redemptions integer NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (attendee_id, checkpoint_id),
    FOREIGN KEY (attendee_id, cycle_id) REFERENCES ats.attendees(id, cycle_id),
    FOREIGN KEY (checkpoint_id, cycle_id) REFERENCES ats.checkpoints(id, cycle_id)
);

CREATE TABLE ats.redemption_requests (
    id uuid PRIMARY KEY,
    idempotency_key uuid NOT NULL UNIQUE,
    scanner_user_id uuid NOT NULL REFERENCES ats.users(id),
    checkpoint_id uuid NOT NULL REFERENCES ats.checkpoints(id),
    qr_token_hash bytea NOT NULL,
    pass_id uuid REFERENCES ats.passes(id),
    attendee_id uuid REFERENCES ats.attendees(id),
    outcome text NOT NULL,
    attendee_display_name text,
    pass_status text,
    checkpoint_name text NOT NULL,
    redemption_id uuid UNIQUE,
    created_at timestamptz NOT NULL,
    FOREIGN KEY (pass_id, attendee_id) REFERENCES ats.passes(id, attendee_id)
);

CREATE TABLE ats.redemptions (
    id uuid PRIMARY KEY,
    pass_id uuid NOT NULL,
    attendee_id uuid NOT NULL,
    checkpoint_id uuid NOT NULL,
    cycle_id uuid NOT NULL,
    ordinal integer NOT NULL,
    scanner_user_id uuid NOT NULL REFERENCES ats.users(id),
    idempotency_key uuid NOT NULL UNIQUE,
    redeemed_at timestamptz NOT NULL,
    FOREIGN KEY (pass_id, attendee_id) REFERENCES ats.passes(id, attendee_id),
    FOREIGN KEY (attendee_id, cycle_id) REFERENCES ats.attendees(id, cycle_id),
    FOREIGN KEY (checkpoint_id, cycle_id) REFERENCES ats.checkpoints(id, cycle_id),
    UNIQUE (attendee_id, checkpoint_id, ordinal)
);

ALTER TABLE ats.redemption_requests
    ADD FOREIGN KEY (redemption_id) REFERENCES ats.redemptions(id);

CREATE INDEX checkpoints_active_cycle_idx ON ats.checkpoints (cycle_id, name) WHERE active;
CREATE INDEX attendee_entitlements_checkpoint_idx ON ats.attendee_entitlements (checkpoint_id, attendee_id);
CREATE INDEX redemptions_attendee_checkpoint_idx ON ats.redemptions (attendee_id, checkpoint_id, ordinal);
CREATE INDEX redemption_requests_scanner_created_idx ON ats.redemption_requests (scanner_user_id, created_at DESC);
CREATE INDEX redemptions_cycle_redeemed_at_idx ON ats.redemptions (cycle_id, redeemed_at DESC, id DESC);
CREATE INDEX redemptions_checkpoint_redeemed_at_idx ON ats.redemptions (checkpoint_id, redeemed_at DESC, id DESC);
