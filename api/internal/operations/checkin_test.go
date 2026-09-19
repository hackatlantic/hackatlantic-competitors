package operations

import (
	"context"
	"errors"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"testing"
)

func TestCheckInServiceRejectsUnauthorizedAndInvalidSetupBeforeDatabase(t *testing.T) {
	service := NewService(nil, 0, 0)
	if _, err := service.GetAttendanceSummary(context.Background(), users.User{}); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := service.EnableEntrance(context.Background(), users.User{}, "bad-id"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	actor := users.User{ID: "9d18f13d-9f79-40a2-831b-c4350f806555", Roles: map[users.Role]struct{}{users.RoleOrganizer: {}}}
	if _, err := service.EnableEntrance(context.Background(), actor, "bad-id"); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
}
