//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/decisions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/reviews"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/rsvps"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRSVPLifecycleAuthorizationConcurrencyAndDecisionChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	adminID := createUser(t, ctx, pool, "rsvp-admin")
	ownerID := createUser(t, ctx, pool, "rsvp-owner")
	otherID := createUser(t, ctx, pool, "rsvp-other")
	scannerID := createUser(t, ctx, pool, "rsvp-scanner")
	admin := decisionTestUser(adminID, "rsvp-admin", users.RoleAdmin)
	owner := decisionTestUser(ownerID, "rsvp-owner", users.RoleApplicant)
	other := decisionTestUser(otherID, "rsvp-other", users.RoleApplicant)
	scanner := decisionTestUser(scannerID, "rsvp-scanner", users.RoleScanner)
	cycle := insertCycle(t, ctx, pool, "rsvp-cycle", true)
	form := insertWorkflowForm(t, ctx, pool, cycle, adminID)
	applicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycle, form, ownerID, "Synthetic RSVP candidate")
	// Exercise the real restricted runtime role, not only migration-owner access.
	config := pool.Config()
	config.ConnConfig.RuntimeParams["application_name"] = "hackatlantic-rsvp-test"
	config.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		_, err := connection.Exec(ctx, "SET ROLE hackatlantic_app")
		return err
	}
	runtime, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	rsvp := rsvps.NewService(runtime, 5*time.Second, 15*time.Second)
	decisionService := decisions.NewService(runtime, 5*time.Second, 15*time.Second)
	passService, err := passes.NewService(runtime, 5*time.Second, 15*time.Second, passes.Config{
		QRTokenPepper: encodedTestPepper(41), ClaimTokenPepper: encodedTestPepper(42), AppBaseURL: "https://example.invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Verifier: intakeVerifier{}, Users: intakeUserResolver{users: map[string]users.User{
			"rsvp-admin": admin, "rsvp-owner": owner, "rsvp-other": other, "rsvp-scanner": scanner,
		}}, RSVPs: rsvp, Decisions: decisionService, Passes: passService, Reviews: reviews.NewService(runtime, 5*time.Second, 15*time.Second),
	}))
	defer server.Close()
	path := "/v1/applications/" + applicationID + "/rsvp"
	request := func(method, token string, body any) *http.Response {
		return intakeRequest(t, server.URL, method, path, token, body)
	}
	read := func() rsvps.Response {
		response := request(http.MethodGet, "rsvp-owner", nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("read RSVP status: %d", response.StatusCode)
		}
		var value rsvps.Response
		decodeIntakeResponse(t, response, &value)
		return value
	}
	for _, token := range []string{"rsvp-owner", "rsvp-other"} {
		assertDecisionStatus(t, request(http.MethodGet, token, nil), http.StatusNotFound)
	}
	assertDecisionStatus(t, request(http.MethodGet, "rsvp-scanner", nil), http.StatusForbidden)
	assertDecisionStatus(t, request(http.MethodGet, "", nil), http.StatusUnauthorized)
	first := recordLifecycleDecision(t, server.URL, applicationID, "rsvp-admin", "accepted", "private reason")
	assertDecisionStatus(t, request(http.MethodGet, "rsvp-owner", nil), http.StatusNotFound)
	releaseLifecycleDecision(t, server.URL, first.ID, "rsvp-admin")
	attendeeID := attendeeForApplication(t, ctx, pool, applicationID)
	issuePath := "/v1/admin/attendees/" + attendeeID + "/passes"
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, issuePath, "rsvp-admin", nil), http.StatusConflict)
	pending := read()
	if pending.Status != "pending" || pending.LockVersion != 0 || pending.RespondedAt != nil {
		t.Fatalf("unexpected pending response: %+v", pending)
	}
	payload := map[string]any{"decisionId": first.ID, "status": "confirmed", "lockVersion": 0}
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-other", payload), http.StatusNotFound)
	assertDecisionStatus(t, request(http.MethodPut, "", payload), http.StatusUnauthorized)
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-scanner", payload), http.StatusForbidden)
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-owner", map[string]any{"status": "confirmed"}), http.StatusBadRequest)
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-owner", map[string]any{"decisionId": first.ID, "status": "pending", "lockVersion": 0}), http.StatusUnprocessableEntity)
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-owner", payload), http.StatusOK)
	confirmed := read()
	if confirmed.Status != "confirmed" || confirmed.LockVersion != 1 || confirmed.RespondedAt == nil {
		t.Fatalf("unexpected confirmed response: %+v", confirmed)
	}
	var automaticPasses int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.passes WHERE attendee_id=$1`, attendeeID).Scan(&automaticPasses); err != nil || automaticPasses != 0 {
		t.Fatalf("RSVP must not issue a pass: count=%d err=%v", automaticPasses, err)
	}
	issuedResponse := intakeRequest(t, server.URL, http.MethodPost, issuePath, "rsvp-admin", nil)
	if issuedResponse.StatusCode != http.StatusCreated {
		t.Fatalf("manual pass release status: %d", issuedResponse.StatusCode)
	}
	var issued lifecyclePassResponse
	decodeIntakeResponse(t, issuedResponse, &issued)
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-owner", payload), http.StatusOK)
	if again := read(); !again.RespondedAt.Equal(*confirmed.RespondedAt) || again.LockVersion != 1 {
		t.Fatal("repeated response was not idempotent")
	}
	payload["status"] = "declined"
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-owner", payload), http.StatusConflict)
	payload["lockVersion"] = 1
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-owner", payload), http.StatusOK)
	if read().Status != "declined" {
		t.Fatal("decline was not persisted")
	}
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, issuePath, "rsvp-admin", nil), http.StatusConflict)
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/passes/"+issued.ID+"/reissue", "rsvp-admin", nil), http.StatusConflict)
	var existingPassStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM ats.passes WHERE id=$1`, issued.ID).Scan(&existingPassStatus); err != nil || existingPassStatus != "active" {
		t.Fatalf("RSVP silently changed an issued pass: %s %v", existingPassStatus, err)
	}
	assertDecisionReleasePersistence(t, ctx, pool, applicationID, first.ID)
	assertAttendeePersistence(t, ctx, pool, applicationID, 1, 1)
	detailResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications/"+applicationID, "rsvp-admin", nil)
	if detailResponse.StatusCode != http.StatusOK {
		t.Fatalf("admin detail status: %d", detailResponse.StatusCode)
	}
	var detail struct {
		RSVP *rsvps.Response `json:"rsvp"`
	}
	decodeIntakeResponse(t, detailResponse, &detail)
	if detail.RSVP == nil || detail.RSVP.Status != "declined" {
		t.Fatal("admin detail omitted RSVP")
	}
	for _, state := range []string{"pending", "confirmed", "declined"} {
		response := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications?rsvp="+state, "rsvp-admin", nil)
		var queue struct {
			Items []struct {
				ID   string          `json:"id"`
				RSVP *rsvps.Response `json:"rsvp"`
			} `json:"items"`
		}
		decodeIntakeResponse(t, response, &queue)
		if state == "declined" {
			if len(queue.Items) != 1 || queue.Items[0].RSVP == nil || queue.Items[0].RSVP.Status != state {
				t.Fatal("admin RSVP filter omitted response")
			}
		} else if len(queue.Items) != 0 {
			t.Fatal("admin RSVP filter included wrong response")
		}
	}
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications?rsvp=invalid", "rsvp-admin", nil), http.StatusUnprocessableEntity)
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications?rsvp=declined", "rsvp-owner", nil), http.StatusForbidden)
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = 'attendance.rsvp_changed' AND subject_id = $1`, applicationID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("expected two actual response changes, audits=%d err=%v", audits, err)
	}
	// A new acceptance is pending; an older tab must not confirm the new offer.
	second := recordLifecycleDecision(t, server.URL, applicationID, "rsvp-admin", "accepted", "new offer")
	assertDecisionStatus(t, request(http.MethodGet, "rsvp-owner", nil), http.StatusNotFound)
	releaseLifecycleDecision(t, server.URL, second.ID, "rsvp-admin")
	if read().Status != "pending" {
		t.Fatal("new acceptance reused old RSVP")
	}
	assertDecisionStatus(t, request(http.MethodPut, "rsvp-owner", payload), http.StatusConflict)
	// Simultaneous opposite choices from the same version: exactly one applies.
	start := make(chan struct{})
	results := make(chan error, 2)
	var writers sync.WaitGroup
	for _, status := range []string{"confirmed", "declined"} {
		writers.Add(1)
		go func(status string) {
			defer writers.Done()
			<-start
			_, err := rsvp.Respond(ctx, owner, rsvps.Input{ApplicationID: applicationID, DecisionID: second.ID, Status: status, LockVersion: 0})
			results <- err
		}(status)
	}
	close(start)
	writers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, rsvps.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 || read().LockVersion != 1 {
		t.Fatalf("concurrent response race: successes=%d conflicts=%d", successes, conflicts)
	}
	// Hold the application lock during a replacement rejection. RSVP must wait,
	// then see the committed decision rather than save against the old acceptance.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM ats.applications WHERE id = $1 FOR UPDATE`, applicationID); err != nil {
		t.Fatal(err)
	}
	var rejectionID string
	if err := tx.QueryRow(ctx, `INSERT INTO ats.decisions(application_id,outcome,decided_by,supersedes_id) VALUES($1,'rejected',$2,$3) RETURNING id::text`, applicationID, adminID, second.ID).Scan(&rejectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE ats.applications SET status='rejected', current_decision_id=$2, decision_released_at=NULL WHERE id=$1`, applicationID, rejectionID); err != nil {
		t.Fatal(err)
	}
	blockedResult := make(chan error, 1)
	go func() {
		_, err := rsvp.Respond(ctx, owner, rsvps.Input{ApplicationID: applicationID, DecisionID: second.ID, Status: "confirmed", LockVersion: 1})
		blockedResult <- err
	}()
	waiting := false
	for i := 0; i < 100; i++ {
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='hackatlantic-rsvp-test' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("RSVP did not wait for the decision transaction")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-blockedResult; !errors.Is(err, rsvps.ErrNotFound) {
		t.Fatalf("RSVP raced past rejection: %v", err)
	}
	if _, err := rsvp.GetForApplicant(ctx, owner, applicationID); !errors.Is(err, rsvps.ErrNotFound) {
		t.Fatal("rejected applicant retained RSVP")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.attendance_responses WHERE decision_id IN ($1,$2)`, first.ID, second.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("history was not retained: %d %v", count, err)
	}
	for _, role := range []string{"anon", "authenticated", "service_role"} {
		var access bool
		if err := pool.QueryRow(ctx, `SELECT has_table_privilege($1,'ats.attendance_responses','SELECT,INSERT,UPDATE,DELETE')`, role).Scan(&access); err != nil || access {
			t.Fatalf("unexpected Data API access for %s: %v", role, err)
		}
	}
	// A pass issuer waiting behind an RSVP decline must read the committed
	// response, not issue using the previously confirmed snapshot.
	otherApplication := insertSubmittedWorkflowApplication(t, ctx, pool, cycle, form, otherID, "RSVP issuance race")
	otherDecision := recordLifecycleDecision(t, server.URL, otherApplication, "rsvp-admin", "accepted", "issuance race fixture")
	releaseLifecycleDecision(t, server.URL, otherDecision.ID, "rsvp-admin")
	confirmRSVPFixture(t, ctx, pool, otherDecision.ID)
	otherAttendee := attendeeForApplication(t, ctx, pool, otherApplication)
	declineTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer declineTx.Rollback(ctx)
	if _, err := declineTx.Exec(ctx, `SELECT id FROM ats.applications WHERE id=$1 FOR UPDATE`, otherApplication); err != nil {
		t.Fatal(err)
	}
	if _, err := declineTx.Exec(ctx, `UPDATE ats.attendance_responses SET status='declined', lock_version=lock_version+1 WHERE decision_id=$1`, otherDecision.ID); err != nil {
		t.Fatal(err)
	}
	issueResult := make(chan error, 1)
	go func() { _, err := passService.Issue(ctx, admin, otherAttendee); issueResult <- err }()
	waiting = false
	for i := 0; i < 100; i++ {
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='hackatlantic-rsvp-test' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("pass issue did not wait for the RSVP transaction")
	}
	if err := declineTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-issueResult; !errors.Is(err, passes.ErrRSVPRequired) {
		t.Fatalf("pass was issued despite concurrent decline: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.passes WHERE attendee_id=$1`, otherAttendee).Scan(&count); err != nil || count != 0 {
		t.Fatalf("ineligible pass persisted: %d %v", count, err)
	}
}

// Existing pass/scanner suites explicitly seed confirmed attendance. The RSVP
// lifecycle test above exercises the real applicant API instead of this fixture.
func confirmRSVPFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, decisionID string) {
	t.Helper()
	result, err := pool.Exec(ctx, `INSERT INTO ats.attendance_responses(decision_id,status,responded_by)
        SELECT d.id, 'confirmed', a.applicant_user_id FROM ats.decisions d
        JOIN ats.applications a ON a.id=d.application_id
        WHERE d.id=$1 AND d.outcome='accepted' AND d.released_at IS NOT NULL`, decisionID)
	if err != nil || result.RowsAffected() != 1 {
		t.Fatalf("seed confirmed RSVP fixture: %v", err)
	}
}
