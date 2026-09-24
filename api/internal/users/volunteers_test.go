package users

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestVolunteerNameValidation(t *testing.T) {
	for _, name := range []string{"", " ", "A", "Ada\nAdmin", strings.Repeat("a", 101), string([]byte{0xff, 0xff})} {
		if _, err := normalizeVolunteerName(name); !errors.Is(err, ErrVolunteerInvalid) {
			t.Errorf("accepted invalid name %q", name)
		}
	}
	for _, name := range []string{"  Alex Morgan  ", "O’Connor", "李明"} {
		if _, err := normalizeVolunteerName(name); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVolunteerAuthorizationBeforeDatabase(t *testing.T) {
	s := &Service{}
	for _, role := range []Role{RoleApplicant, RoleScanner, RoleOrganizer} {
		actor := User{ID: "someone", Roles: map[Role]struct{}{role: {}}}
		if _, err := s.ListVolunteerRequests(context.Background(), actor, 0); !errors.Is(err, ErrForbidden) {
			t.Fatal("non-admin list permitted")
		}
		if err := s.ReviewVolunteerRequest(context.Background(), actor, "other", "approved", "pending"); !errors.Is(err, ErrForbidden) {
			t.Fatal("non-admin approval permitted")
		}
	}
	admin := User{ID: "self", Roles: map[Role]struct{}{RoleAdmin: {}}}
	if err := s.ReviewVolunteerRequest(context.Background(), admin, "self", "approved", "pending"); !errors.Is(err, ErrForbidden) {
		t.Fatal("self approval permitted")
	}
	if _, err := s.RequestVolunteerAccess(context.Background(), User{}, "Alex Morgan"); !errors.Is(err, ErrForbidden) {
		t.Fatal("anonymous request permitted")
	}
	for _, status := range []string{"admin", "scanner", "pending"} {
		if err := s.ReviewVolunteerRequest(context.Background(), admin, "other", status, "pending"); !errors.Is(err, ErrVolunteerInvalid) {
			t.Fatal("invalid decision permitted")
		}
	}
}
