// seed-intake creates the deterministic local application-intake fixture.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const seedTimeout = 30 * time.Second

const (
	fixtureOrganizerID      = "60000000-0000-4000-8000-000000000001"
	fixtureCycleID          = "60000000-0000-4000-8000-000000000002"
	fixtureFormID           = "60000000-0000-4000-8000-000000000006"
	fixtureScannerID        = "60000000-0000-4000-8000-000000000004"
	fixtureCheckpointID     = "60000000-0000-4000-8000-000000000005"
	fixtureOrganizerClerkID = "user_dev_intake_organizer"
	fixtureScannerClerkID   = "user_dev_intake_scanner"
	fixtureCycleSlug        = "dev-intake"
)

var fixtureFormSchema = []byte(`{
  "resumeRequired": false,
  "resumeAfterQuestionKey": "school",
  "questions": [
    {
      "key": "fullName",
      "label": "Name",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "email",
      "label": "Email",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "email",
      "help": "Verified through your signed-in account."
    },
    {
      "key": "school",
      "label": "School",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "dietaryRestrictions",
      "label": "Dietary restrictions",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text",
      "help": "Enter None if you do not have any."
    },
    {
      "key": "hackAtlanticExcitement",
      "label": "What are you most excited about at Hack Atlantic?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "maxWords": 100,
      "help": "Maximum 100 words."
    },
    {
      "key": "priorHackathonExperience",
      "label": "Prior hackathon experience",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "select",
      "options": ["This is my first", "1–3", "3+"]
    },
    {
      "key": "desiredTeammateNames",
      "label": "Desired teammate names (Optional)",
      "type": "string",
      "required": false,
      "section": "Hackathon Specific Questions",
      "control": "text"
    },
    {
      "key": "hardwareProject",
      "label": "Are you looking to make a hardware project?",
      "type": "boolean",
      "required": true,
      "section": "Hackathon Specific Questions"
    },
    {
      "key": "hardwareEquipment",
      "label": "What equipment are you looking to use?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "showWhen": {
        "key": "hardwareProject",
        "equals": true
      }
    }
  ]
}`)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()

	pool, err := database.Open(ctx, database.Config{URL: os.Getenv("DATABASE_URL")})
	if err != nil {
		log.Fatalf("configure database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("connect database: %v", err)
	}

	if err := seed(ctx, pool.Pool); err != nil {
		log.Fatalf("seed intake fixture: %v", err)
	}
}

func seed(ctx context.Context, pool *pgxpool.Pool) error {
	organizerID, err := uuid(fixtureOrganizerID)
	if err != nil {
		return err
	}
	cycleID, err := uuid(fixtureCycleID)
	if err != nil {
		return err
	}
	formID, err := uuid(fixtureFormID)
	if err != nil {
		return err
	}
	scannerID, err := uuid(fixtureScannerID)
	if err != nil {
		return err
	}
	checkpointID, err := uuid(fixtureCheckpointID)
	if err != nil {
		return err
	}

	transaction, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)

	organizerID, err = queries.SeedIntakeFixtureUser(ctx, sqlc.SeedIntakeFixtureUserParams{
		ID:           organizerID,
		ClerkUserID:  fixtureOrganizerClerkID,
		PrimaryEmail: "dev-intake-organizer@example.test",
		DisplayName:  pgtype.Text{String: "Development Intake Organizer", Valid: true},
	})
	if err != nil {
		return err
	}
	if err := queries.AssignUserRole(ctx, sqlc.AssignUserRoleParams{
		UserID:    organizerID,
		Role:      "organizer",
		CreatedBy: organizerID,
	}); err != nil {
		return err
	}
	scannerID, err = queries.SeedIntakeFixtureUser(ctx, sqlc.SeedIntakeFixtureUserParams{
		ID:           scannerID,
		ClerkUserID:  fixtureScannerClerkID,
		PrimaryEmail: "dev-intake-scanner@example.test",
		DisplayName:  pgtype.Text{String: "Development Scanner", Valid: true},
	})
	if err != nil {
		return err
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role, created_by)
		VALUES ($1, 'scanner', $2)
		ON CONFLICT (user_id, role) DO NOTHING`, scannerID, organizerID); err != nil {
		return err
	}
	otherActive, err := queries.OtherActiveApplicationCycleExists(ctx, cycleID)
	if err != nil {
		return err
	}
	if otherActive {
		return fmt.Errorf("another active application cycle exists; refuse to replace it with the development fixture")
	}
	if err := queries.EnsureIntakeFixtureCycle(ctx, sqlc.EnsureIntakeFixtureCycleParams{
		ID:   cycleID,
		Slug: fixtureCycleSlug,
		Name: "HackAtlantic Development Intake",
	}); err != nil {
		return err
	}
	if err := queries.EnsureIntakeFixtureForm(ctx, sqlc.EnsureIntakeFixtureFormParams{
		ID:         formID,
		CycleID:    cycleID,
		SchemaJson: fixtureFormSchema,
		CreatedBy:  organizerID,
	}); err != nil {
		return err
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO ats.checkpoints (
		id, cycle_id, slug, name, default_allowed, default_max_redemptions, active
	) VALUES ($1, $2, 'event-entry', 'Event entry', true, 1, true)
	ON CONFLICT (cycle_id, slug) DO UPDATE
	SET name = EXCLUDED.name,
		default_allowed = EXCLUDED.default_allowed,
		default_max_redemptions = EXCLUDED.default_max_redemptions,
		active = EXCLUDED.active,
		updated_at = CURRENT_TIMESTAMP`, checkpointID, cycleID); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}

func uuid(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}
