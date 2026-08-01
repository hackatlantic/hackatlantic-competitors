CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_user_id text NOT NULL UNIQUE,
    primary_email text NOT NULL CHECK (primary_email <> ''),
    display_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id),
    role text NOT NULL CHECK (role IN ('applicant', 'reviewer', 'organizer', 'scanner')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid REFERENCES users(id),
    PRIMARY KEY (user_id, role)
);
CREATE INDEX user_roles_role_user_id_idx ON user_roles (role, user_id);

CREATE TABLE application_cycles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    applications_open_at timestamptz NOT NULL,
    applications_close_at timestamptz NOT NULL,
    active boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT application_cycles_window_check CHECK (applications_open_at < applications_close_at)
);
CREATE UNIQUE INDEX application_cycles_one_active_idx ON application_cycles (active) WHERE active;

CREATE TABLE application_forms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cycle_id uuid NOT NULL REFERENCES application_cycles(id),
    version integer NOT NULL CHECK (version > 0),
    schema_json jsonb NOT NULL CHECK (jsonb_typeof(schema_json) = 'object'),
    published_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cycle_id, version),
    UNIQUE (id, cycle_id)
);

CREATE TABLE applications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cycle_id uuid NOT NULL REFERENCES application_cycles(id),
    form_id uuid NOT NULL,
    applicant_user_id uuid NOT NULL REFERENCES users(id),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'withdrawn', 'accepted', 'waitlisted', 'rejected')),
    submitted_at timestamptz,
    withdrawn_at timestamptz,
    current_decision text CHECK (current_decision IS NULL OR current_decision IN ('accepted', 'waitlisted', 'rejected')),
    decision_released_at timestamptz,
    lock_version integer NOT NULL DEFAULT 0 CHECK (lock_version >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cycle_id, applicant_user_id),
    FOREIGN KEY (form_id, cycle_id) REFERENCES application_forms(id, cycle_id)
);

CREATE TABLE application_answers (
    application_id uuid NOT NULL REFERENCES applications(id),
    question_key text NOT NULL CHECK (question_key <> ''),
    value_json jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (application_id, question_key)
);

CREATE TABLE review_assignments (
    application_id uuid NOT NULL REFERENCES applications(id),
    reviewer_user_id uuid NOT NULL REFERENCES users(id),
    assigned_by uuid NOT NULL REFERENCES users(id),
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (application_id, reviewer_user_id)
);
CREATE INDEX review_assignments_reviewer_application_idx ON review_assignments (reviewer_user_id, application_id);

CREATE TABLE reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES applications(id),
    reviewer_user_id uuid NOT NULL REFERENCES users(id),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted')),
    score_json jsonb NOT NULL CHECK (jsonb_typeof(score_json) = 'object'),
    recommendation text,
    internal_notes text,
    submitted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, reviewer_user_id)
);
CREATE INDEX reviews_reviewer_status_idx ON reviews (reviewer_user_id, status);

CREATE TABLE decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES applications(id),
    outcome text NOT NULL CHECK (outcome IN ('accepted', 'waitlisted', 'rejected')),
    internal_reason text,
    decided_by uuid NOT NULL REFERENCES users(id),
    decided_at timestamptz NOT NULL DEFAULT now(),
    released_by uuid REFERENCES users(id),
    released_at timestamptz,
    supersedes_id uuid REFERENCES decisions(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT decisions_release_actor_check CHECK ((released_at IS NULL) = (released_by IS NULL))
);
CREATE INDEX decisions_application_decided_at_idx ON decisions (application_id, decided_at DESC);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid REFERENCES users(id),
    event_type text NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata_json) = 'object'),
    request_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_subject_created_at_idx ON audit_events (subject_type, subject_id, created_at DESC);
CREATE INDEX audit_events_actor_created_at_idx ON audit_events (actor_user_id, created_at DESC);

CREATE TABLE email_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type text NOT NULL,
    recipient_user_id uuid REFERENCES users(id),
    recipient_email text NOT NULL CHECK (recipient_email <> ''),
    template_key text NOT NULL,
    template_data_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(template_data_json) = 'object'),
    dedupe_key text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'sent', 'failed')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    provider_message_id text,
    last_error_code text,
    sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_outbox_available_idx ON email_outbox (available_at, id) WHERE status IN ('pending', 'failed');

CREATE FUNCTION prevent_non_draft_application_answers() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    application_status text;
BEGIN
    SELECT status INTO application_status
    FROM applications
    WHERE id = COALESCE(NEW.application_id, OLD.application_id);

    IF application_status IS DISTINCT FROM 'draft' THEN
        RAISE EXCEPTION 'application answers are editable only while the application is a draft';
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE TRIGGER application_answers_editable_only
BEFORE INSERT OR UPDATE OR DELETE ON application_answers
FOR EACH ROW EXECUTE FUNCTION prevent_non_draft_application_answers();

CREATE FUNCTION prevent_immutable_form_schema_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.schema_json IS DISTINCT FROM OLD.schema_json
       AND (OLD.published_at IS NOT NULL OR EXISTS (SELECT 1 FROM applications WHERE form_id = OLD.id)) THEN
        RAISE EXCEPTION 'a published or referenced application form schema is immutable';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER application_forms_schema_immutable
BEFORE UPDATE ON application_forms
FOR EACH ROW EXECUTE FUNCTION prevent_immutable_form_schema_change();
