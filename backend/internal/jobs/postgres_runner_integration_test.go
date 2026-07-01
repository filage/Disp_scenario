//go:build integration

package jobs

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type captureProcessor struct {
	payload AnalysisPayload
}

func (p *captureProcessor) Process(_ context.Context, raw json.RawMessage) error {
	payload, err := DecodeAnalysisPayload(raw)
	if err != nil {
		return err
	}
	p.payload = payload
	return nil
}

func TestPostgresRunnerClaimsJobAndPublishesOutbox(t *testing.T) {
	ctx := context.Background()
	container, err := postgres.Run(
		ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("analyst"),
		postgres.WithUsername("analyst"),
		postgres.WithPassword("analyst"),
		postgres.WithInitScripts(
			"../../migrations/000001_init.up.sql",
			"../../migrations/000002_runtime_boundary_rules.up.sql",
			"../../migrations/000003_analysis_correlation_id.up.sql",
			"../../migrations/000004_analysis_run_cost.up.sql",
			"../../migrations/000005_strict_known_scenario_boundaries.up.sql",
			"../../migrations/000006_atomic_action_taxonomy.up.sql",
			"../../migrations/000007_user_credentials.up.sql",
			"../../migrations/000008_routine_scenarios.up.sql",
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connectionString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	recordingID, runID, jobID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO recordings (
			id, organization_id, original_name, mime_type, size_bytes, status, object_key
		) VALUES ($1,$2,'fixture.webm','video/webm',1024,'UPLOADED',$3)`,
		recordingID, organizationID, "recordings/"+recordingID.String()+"/source.webm"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO analysis_runs (
			id, organization_id, recording_id, provider, prompt_version,
			normalization_version, grouping_version, status
		) VALUES ($1,$2,$3,'gemini','v1','v1','v1','QUEUED')`,
		runID, organizationID, recordingID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO analysis_jobs (
			id, organization_id, recording_id, analysis_run_id,
			idempotency_key, correlation_id
		) VALUES ($1,$2,$3,$4,$5,'runner-test')`,
		jobID, organizationID, recordingID, runID, "analysis:"+runID.String()); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(AnalysisPayload{
		JobID: jobID.String(), AnalysisRunID: runID.String(),
		RecordingID: recordingID.String(), CorrelationID: "runner-test",
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ('analysis_job',$1,'analysis.requested',$2)`, jobID, payload); err != nil {
		t.Fatal(err)
	}

	processor := &captureProcessor{}
	runner := NewPostgresRunner(pool, processor, slog.New(slog.NewTextHandler(io.Discard, nil)))
	processed, err := runner.runOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !processed || processor.payload.JobID != jobID.String() {
		t.Fatalf("job was not delivered: processed=%v payload=%+v", processed, processor.payload)
	}
	var taskID string
	if err := pool.QueryRow(ctx, `SELECT queue_task_id FROM analysis_jobs WHERE id=$1`, jobID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(taskID, "postgres:") {
		t.Fatalf("unexpected queue task id: %s", taskID)
	}
	var unlocked bool
	if err := pool.QueryRow(ctx, `
		SELECT locked_at IS NULL FROM analysis_jobs WHERE id=$1`, jobID).Scan(&unlocked); err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Fatal("job lock was not released")
	}
	var published bool
	if err := pool.QueryRow(ctx, `
		SELECT published_at IS NOT NULL FROM outbox_events WHERE aggregate_id=$1`, jobID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if !published {
		t.Fatal("outbox event was not marked published")
	}
}
