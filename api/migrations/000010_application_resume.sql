ALTER TABLE ats.applications
    ADD COLUMN applicant_email_snapshot text;

UPDATE ats.applications AS applications
SET applicant_email_snapshot = users.primary_email
FROM ats.users AS users
WHERE users.id = applications.applicant_user_id
  AND applications.status <> 'draft';

CREATE TABLE ats.application_resumes (
    application_id uuid PRIMARY KEY REFERENCES ats.applications(id),
    object_key text NOT NULL UNIQUE CHECK (object_key <> ''),
    original_filename text NOT NULL CHECK (original_filename <> ''),
    media_type text NOT NULL CHECK (media_type = 'application/pdf'),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 5242880),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
