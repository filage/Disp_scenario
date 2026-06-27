//go:build integration

package recording

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestRepositoryAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	connectionString := os.Getenv("TEST_DATABASE_URL")
	if connectionString == "" {
		container, err := postgres.Run(
			ctx,
			"postgres:16-alpine",
			postgres.WithDatabase("analyst"),
			postgres.WithUsername("analyst"),
			postgres.WithPassword("analyst"),
			postgres.WithInitScripts("../../migrations/000001_init.up.sql"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := container.Terminate(ctx); err != nil {
				t.Errorf("terminate postgres: %v", err)
			}
		})
		connectionString, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
	}
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	organizationID := uuid.New()
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO organizations (id, name) VALUES ($1, $2)`,
		organizationID, "Integration test "+organizationID.String(),
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID); err != nil {
			t.Errorf("delete test organization: %v", err)
		}
	})

	repository := NewRepository(pool)
	created, err := repository.Create(
		ctx, organizationID, "sample.mp4", "video/mp4",
		1024, "recordings/"+uuid.NewString()+"/source.mp4", "integration-test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "PENDING_UPLOAD" {
		t.Fatalf("unexpected status: %s", created.Status)
	}
	completed, err := repository.Complete(
		ctx, organizationID, created.ID, "integration-test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "UPLOADED" {
		t.Fatalf("unexpected completed status: %s", completed.Status)
	}
	items, err := repository.List(ctx, organizationID)
	if err != nil || len(items) != 1 {
		t.Fatalf("unexpected list result: %d, %v", len(items), err)
	}
	second, err := repository.Create(
		ctx, organizationID, "second.mp4", "video/mp4",
		2048, "recordings/"+uuid.NewString()+"/source.mp4", "integration-test",
	)
	if err != nil {
		t.Fatal(err)
	}
	templateID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO scenario_templates (
			id, organization_id, code, name, issue_type, signature, frequency,
			average_duration_ms, median_duration_ms, p95_duration_ms,
			manual_check_count, automation_score
		) VALUES ($1,$2,'LATE_DELIVERY','Late delivery','late delivery',
			'LATE_DELIVERY>CHECK',2,2000,2000,3000,2,0.7)`,
		templateID, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO automation_candidates (
			template_id, title, rationale, affected_steps, impact, confidence, score
		) VALUES ($1,'Stale candidate','Before deletion','["CHECK"]','test',0.8,0.7)`,
		templateID); err != nil {
		t.Fatal(err)
	}
	firstEventID := uuid.New()
	secondEventID := uuid.New()
	for _, event := range []struct {
		id, recordingID uuid.UUID
	}{
		{id: firstEventID, recordingID: created.ID},
		{id: secondEventID, recordingID: second.ID},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO action_events (
				id, recording_id, timestamp_ms, canonical_action, event_type,
				screen, issue_type, target, confidence
			) VALUES ($1,$2,0,'CHECK','manual','orders','late delivery','check',0.8)`,
			event.id, event.recordingID); err != nil {
			t.Fatal(err)
		}
	}
	for _, instance := range []struct {
		recordingID uuid.UUID
		eventID     uuid.UUID
		durationMS  int
	}{
		{recordingID: created.ID, eventID: firstEventID, durationMS: 1000},
		{recordingID: second.ID, eventID: secondEventID, durationMS: 3000},
	} {
		eventIDs, err := json.Marshal([]uuid.UUID{instance.eventID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO scenario_instances (
				recording_id, template_id, known_scenario_code, issue_type,
				started_at_ms, ended_at_ms, duration_ms, event_ids,
				outcome, confidence
			) VALUES ($1,$2,'LATE_DELIVERY','late delivery',0,$3,$3,$4,'completed',0.8)`,
			instance.recordingID, templateID, instance.durationMS, eventIDs); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repository.Delete(ctx, organizationID, created.ID); err != nil {
		t.Fatal(err)
	}
	var remainingInstances int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM scenario_instances WHERE template_id = $1`,
		templateID).Scan(&remainingInstances); err != nil {
		t.Fatal(err)
	}
	if remainingInstances != 1 {
		t.Fatalf("unexpected remaining scenario instances: %d", remainingInstances)
	}

	if _, err := repository.Delete(ctx, organizationID, second.ID); err != nil {
		t.Fatal(err)
	}
	var templateCount, candidateCount int
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM scenario_templates WHERE id = $1),
		  (SELECT count(*) FROM automation_candidates WHERE template_id = $1)`,
		templateID).Scan(&templateCount, &candidateCount); err != nil {
		t.Fatal(err)
	}
	if templateCount != 0 || candidateCount != 0 {
		t.Fatalf("orphan group data remains: templates=%d candidates=%d", templateCount, candidateCount)
	}
}
