-- name: GetDraftApplicationResumeTarget :one
SELECT applications.id
FROM ats.applications
WHERE applications.id = $1
  AND applications.applicant_user_id = $2
  AND applications.status = 'draft';

-- name: GetApplicationResumeForApplicant :one
SELECT resumes.application_id, resumes.object_key, resumes.original_filename,
       resumes.media_type, resumes.byte_size, resumes.sha256,
       resumes.uploaded_at, resumes.updated_at
FROM ats.application_resumes AS resumes
JOIN ats.applications AS applications ON applications.id = resumes.application_id
WHERE resumes.application_id = $1
  AND applications.applicant_user_id = $2;

-- name: GetApplicationResumeForAdmin :one
SELECT resumes.application_id, resumes.object_key, resumes.original_filename,
       resumes.media_type, resumes.byte_size, resumes.sha256,
       resumes.uploaded_at, resumes.updated_at
FROM ats.application_resumes AS resumes
JOIN ats.applications AS applications ON applications.id = resumes.application_id
WHERE resumes.application_id = $1
  AND applications.status IN ('submitted', 'accepted', 'waitlisted', 'rejected');

-- name: UpsertApplicationResume :one
INSERT INTO ats.application_resumes (
    application_id, object_key, original_filename, media_type, byte_size, sha256
)
VALUES ($1, $2, $3, 'application/pdf', $4, $5)
ON CONFLICT (application_id) DO UPDATE
SET object_key = EXCLUDED.object_key,
    original_filename = EXCLUDED.original_filename,
    media_type = EXCLUDED.media_type,
    byte_size = EXCLUDED.byte_size,
    sha256 = EXCLUDED.sha256,
    uploaded_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
RETURNING application_id, object_key, original_filename, media_type,
          byte_size, sha256, uploaded_at, updated_at;

-- name: ApplicationHasResume :one
SELECT EXISTS (
    SELECT 1 FROM ats.application_resumes WHERE application_id = $1
) AS has_resume;
