package users

import "testing"

func TestUserRolesAreAdditive(t *testing.T) {
	user := User{Roles: map[Role]struct{}{RoleApplicant: {}, RoleReviewer: {}}}
	if !user.HasRole(RoleApplicant) || !user.HasRole(RoleReviewer) {
		t.Fatal("expected additive applicant and reviewer roles")
	}
	if user.HasRole(RoleOrganizer) {
		t.Fatal("unexpected organizer role")
	}
}

func TestAdminIncludesAllStaffCapabilities(t *testing.T) {
	user := User{Roles: map[Role]struct{}{RoleApplicant: {}, RoleAdmin: {}}}
	for _, role := range []Role{RoleOrganizer, RoleReviewer, RoleScanner} {
		if !user.HasRole(role) {
			t.Fatalf("expected admin to include %s capability", role)
		}
	}
}

func TestAssignPrivilegedRoleRejectsSelfService(t *testing.T) {
	service := &Service{}
	organizer := User{
		ID:    "organizer-id",
		Roles: map[Role]struct{}{RoleOrganizer: {}},
	}

	if err := service.AssignPrivilegedRole(t.Context(), organizer, organizer.ID, RoleReviewer); err != ErrForbidden {
		t.Fatalf("expected organizer self-assignment to be forbidden, got %v", err)
	}
	if err := service.AssignPrivilegedRole(t.Context(), User{}, "target-id", RoleScanner); err != ErrForbidden {
		t.Fatalf("expected non-organizer assignment to be forbidden, got %v", err)
	}
}
