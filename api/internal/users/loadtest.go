package users

import (
	"context"
	"fmt"
	"strings"
)

const loadTestSubjectPrefix = "hat_load_"

// LoadTestProfileSource resolves staging-only synthetic subjects locally and
// delegates every normal Clerk subject unchanged.
type LoadTestProfileSource struct {
	delegate ProfileSource
}

func NewLoadTestProfileSource(delegate ProfileSource) (*LoadTestProfileSource, error) {
	if delegate == nil {
		return nil, fmt.Errorf("load-test profile source requires a Clerk delegate")
	}
	return &LoadTestProfileSource{delegate: delegate}, nil
}

func (s *LoadTestProfileSource) Profile(ctx context.Context, clerkUserID string) (Profile, error) {
	if !strings.HasPrefix(clerkUserID, loadTestSubjectPrefix) {
		return s.delegate.Profile(ctx, clerkUserID)
	}
	if len(clerkUserID) > 180 || !validLoadTestSubject(clerkUserID) {
		return Profile{}, fmt.Errorf("invalid staging load-test subject")
	}
	displayName := "Synthetic load user"
	return Profile{
		ClerkUserID: clerkUserID,
		Email:       clerkUserID + "@loadtest.invalid",
		DisplayName: &displayName,
	}, nil
}

func validLoadTestSubject(subject string) bool {
	if len(subject) <= len(loadTestSubjectPrefix) {
		return false
	}
	for _, character := range subject {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}

var _ ProfileSource = (*LoadTestProfileSource)(nil)
