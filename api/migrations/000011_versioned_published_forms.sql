DROP INDEX ats.application_forms_one_published_per_cycle_idx;

CREATE INDEX application_forms_published_versions_idx
    ON ats.application_forms (cycle_id, version DESC)
    WHERE published_at IS NOT NULL;
