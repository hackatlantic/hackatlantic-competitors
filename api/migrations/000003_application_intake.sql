CREATE UNIQUE INDEX application_forms_one_published_per_cycle_idx
    ON ats.application_forms (cycle_id)
    WHERE published_at IS NOT NULL;

CREATE INDEX applications_applicant_created_at_idx
    ON ats.applications (applicant_user_id, created_at DESC);
