package users

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeScannerLookupEmail(t *testing.T) {
	for _, input := range []string{"", "volunteer", "Name <a@example.test>", "a@example.test,b@example.test", "%", "a@example.test\r\nBcc: b@example.test"} {
		if _, err := normalizeLookupEmail(input); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("accepted invalid input %q", input)
		}
	}
	if email, err := normalizeLookupEmail("  Person+check@EXAMPLE.test  "); err != nil || email != "person+check@example.test" {
		t.Fatal("email normalization failed")
	}
}

func TestScannerLookupRejectsNonAdminBeforeUsingDatabase(t *testing.T) {
	for _, role := range []Role{RoleApplicant, RoleScanner, RoleReviewer, RoleOrganizer} {
		_, err := (&Service{}).LookupScannerUser(context.Background(), User{Roles: map[Role]struct{}{role: {}}}, "person@example.test")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("role %s was not denied", role)
		}
	}
}
