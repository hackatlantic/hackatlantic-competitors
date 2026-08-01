//go:build integration

package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/applications"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
)

func TestRequiredResumeAndEmailSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	creatorID := createUser(t, ctx, pool, "resume-creator")
	applicantID := createUser(t, ctx, pool, "resume-applicant")
	cycleID := insertCycle(t, ctx, pool, "resume-cycle", true)
	formSchema := `{"resumeRequired":true,"questions":[{"key":"school","label":"School","type":"string","required":true}]}`
	if _, err := pool.Exec(ctx, `INSERT INTO ats.application_forms (cycle_id, version, schema_json, published_at, created_by) VALUES ($1, 1, $2::jsonb, CURRENT_TIMESTAMP, $3)`, cycleID, formSchema, creatorID); err != nil {
		t.Fatalf("create resume-required form: %v", err)
	}

	service := applications.NewService(pool, 5*time.Second, 10*time.Second)
	applicant := applications.Applicant{ID: applicantID, Email: "verified@example.test"}
	application, err := service.Create(ctx, applicant)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	application, err = service.SaveDraft(ctx, applicant, applications.SaveDraftInput{
		ApplicationID: application.ID,
		LockVersion:   application.LockVersion,
		Answers:       map[string]json.RawMessage{"school": json.RawMessage(`"Dalhousie University"`)},
	})
	if err != nil {
		t.Fatalf("save application: %v", err)
	}
	if _, err := service.Submit(ctx, applicant, applications.SubmitInput{ApplicationID: application.ID, LockVersion: application.LockVersion}); !errors.Is(err, applications.ErrIncomplete) {
		t.Fatalf("submit without required resume: got %v, want incomplete", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO ats.application_resumes (application_id, object_key, original_filename, media_type, byte_size, sha256) VALUES ($1, 'test/resume.pdf', 'resume.pdf', 'application/pdf', 64, decode(repeat('ab', 32), 'hex'))`, application.ID); err != nil {
		t.Fatalf("attach resume metadata: %v", err)
	}
	submitted, err := service.Submit(ctx, applicant, applications.SubmitInput{ApplicationID: application.ID, LockVersion: application.LockVersion})
	if err != nil {
		t.Fatalf("submit with required resume: %v", err)
	}
	if submitted.Status != "submitted" {
		t.Fatalf("submission status = %q", submitted.Status)
	}
	var emailSnapshot string
	if err := pool.QueryRow(ctx, `SELECT applicant_email_snapshot FROM ats.applications WHERE id = $1`, application.ID).Scan(&emailSnapshot); err != nil {
		t.Fatalf("load email snapshot: %v", err)
	}
	if emailSnapshot != applicant.Email {
		t.Fatalf("email snapshot = %q, want %q", emailSnapshot, applicant.Email)
	}
}
