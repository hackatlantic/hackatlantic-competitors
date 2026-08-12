package users

import (
	"context"
	"testing"
)

type recordingProfileSource struct {
	called bool
}

func (s *recordingProfileSource) Profile(_ context.Context, clerkUserID string) (Profile, error) {
	s.called = true
	return Profile{ClerkUserID: clerkUserID, Email: "clerk@example.com"}, nil
}

func TestLoadTestProfileSourceResolvesSyntheticSubjectLocally(t *testing.T) {
	delegate := &recordingProfileSource{}
	source, err := NewLoadTestProfileSource(delegate)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := source.Profile(t.Context(), "hat_load_run_42_applicant_7")
	if err != nil {
		t.Fatal(err)
	}
	if delegate.called {
		t.Fatal("synthetic subject reached Clerk delegate")
	}
	if profile.Email != "hat_load_run_42_applicant_7@loadtest.invalid" {
		t.Fatalf("unexpected synthetic email %q", profile.Email)
	}
}

func TestLoadTestProfileSourceDelegatesNormalClerkSubject(t *testing.T) {
	delegate := &recordingProfileSource{}
	source, err := NewLoadTestProfileSource(delegate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Profile(t.Context(), "user_clerk_subject"); err != nil {
		t.Fatal(err)
	}
	if !delegate.called {
		t.Fatal("normal subject did not reach Clerk delegate")
	}
}

func TestLoadTestProfileSourceRejectsUnsafeSubject(t *testing.T) {
	source, err := NewLoadTestProfileSource(&recordingProfileSource{})
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"hat_load_", "hat_load_bad@example.com", "hat_load_UPPER"} {
		if _, err := source.Profile(t.Context(), subject); err == nil {
			t.Fatalf("expected %q to be rejected", subject)
		}
	}
}
