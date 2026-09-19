//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
)

func TestEntranceSetupIsIdempotentAndSummaryIndependentOfCheckpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	organizerID := createUser(t, ctx, pool, "setup-organizer")
	attendeeID := createUser(t, ctx, pool, "setup-attendee")
	actor := users.User{ID: organizerID, Roles: map[users.Role]struct{}{users.RoleOrganizer: {}}}
	service := operations.NewService(pool, 5*time.Second, 15*time.Second)
	if _, err := service.GetAttendanceSummary(ctx, actor); !errors.Is(err, operations.ErrNotFound) {
		t.Fatalf("expected no active event: %v", err)
	}
	cycle := insertCycle(t, ctx, pool, "setup-current", true)
	if _, err := pool.Exec(ctx, `UPDATE ats.application_cycles SET applications_open_at=now()-interval '2 days', applications_close_at=now()-interval '1 day' WHERE id=$1`, cycle); err != nil {
		t.Fatal(err)
	}
	form := insertForm(t, ctx, pool, cycle, 1, organizerID)
	var application, decision string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.applications(cycle_id,form_id,applicant_user_id) VALUES($1,$2,$3) RETURNING id::text`, cycle, form, attendeeID).Scan(&application); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO ats.decisions(application_id,outcome,decided_by,released_at,released_by) VALUES($1,'accepted',$2,now(),$2) RETURNING id::text`, application, organizerID).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ats.applications SET status='accepted',decision_released_at=now(),current_decision_id=$2 WHERE id=$1`, application, decision); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.attendance_responses(decision_id,status,responded_by) VALUES($1,'confirmed',$2)`, decision, attendeeID); err != nil {
		t.Fatal(err)
	}
	summary, err := service.GetAttendanceSummary(ctx, actor)
	if err != nil || summary.CycleID != cycle || summary.ConfirmedRSVPs != 1 {
		t.Fatalf("closed-form event without checkpoints: %+v %v", summary, err)
	}

	const attempts = 10
	points := make([]operations.Checkpoint, attempts)
	failures := make([]error, attempts)
	var group sync.WaitGroup
	for i := 0; i < attempts; i++ {
		group.Add(1)
		go func(i int) { defer group.Done(); points[i], failures[i] = service.EnableEntrance(ctx, actor, cycle) }(i)
	}
	group.Wait()
	for i, point := range points {
		if failures[i] != nil {
			t.Fatal(failures[i])
		}
		if point.ID != points[0].ID || !point.Active || !point.DefaultAllowed || point.DefaultMaxRedemptions != 1 || point.Slug != "main-entrance" {
			t.Fatalf("unexpected entrance: %+v", point)
		}
	}
	var created, audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.checkpoints WHERE cycle_id=$1`, cycle).Scan(&created); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE actor_user_id=$1 AND event_type='checkpoint_created'`, organizerID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if created != 1 || audits != 1 {
		t.Fatalf("duplicate setup: points=%d audits=%d", created, audits)
	}
	if _, err := pool.Exec(ctx, `UPDATE ats.checkpoints SET active=false,default_allowed=false,default_max_redemptions=3,opens_at=now()+interval '1 day' WHERE id=$1`, points[0].ID); err != nil {
		t.Fatal(err)
	}
	replay, err := service.EnableEntrance(ctx, actor, cycle)
	if err != nil || replay.ID != points[0].ID || replay.Active || replay.DefaultAllowed || replay.DefaultMaxRedemptions != 3 || replay.OpensAt == nil {
		t.Fatalf("replay changed rules: %+v %v", replay, err)
	}

	// A new active event must not inherit the prior event's RSVP or entrance.
	if _, err := pool.Exec(ctx, `UPDATE ats.application_cycles SET active=false WHERE id=$1`, cycle); err != nil {
		t.Fatal(err)
	}
	next := insertCycle(t, ctx, pool, "setup-next", true)
	summary, err = service.GetAttendanceSummary(ctx, actor)
	if err != nil || summary.CycleID != next || summary.ConfirmedRSVPs != 0 {
		t.Fatalf("old responses leaked: %+v %v", summary, err)
	}
	if _, err := service.EnableEntrance(ctx, actor, cycle); !errors.Is(err, operations.ErrConflict) {
		t.Fatalf("inactive cycle setup allowed: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.checkpoints(cycle_id,slug,name) VALUES($1,'custom-entrance','Custom entrance')`, next); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnableEntrance(ctx, actor, next); !errors.Is(err, operations.ErrConflict) {
		t.Fatalf("existing configuration overwritten: %v", err)
	}
}
