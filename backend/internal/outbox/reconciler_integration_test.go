//go:build integration

package outbox

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestReconcilerMarksStaleProcessingJobFailed(t *testing.T) {
	ctx := context.Background()
	pool := outboxTestPool(t, ctx)

	organizationID := uuid.New()
	recordingID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM recordings WHERE id=$1`, recordingID); err != nil {
			t.Errorf("delete test recording: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, organizationID); err != nil {
			t.Errorf("delete test organization: %v", err)
		}
	})

	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`,
		organizationID, "Outbox recovery test "+organizationID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO recordings (
			id, organization_id, original_name, mime_type, size_bytes, status, object_key
		) VALUES ($1,$2,'worker-crash.mp4','video/mp4',1024,'PROCESSING',$3)`,
		recordingID, organizationID, "recordings/"+recordingID.String()+"/source.mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO analysis_runs (
			id, organization_id, recording_id, provider, model, prompt_version,
			normalization_version, grouping_version, status, started_at
		) VALUES ($1,$2,$3,'gemini','gemini-3.5-flash','video-raw-extractor-v8',
			'vision-normalizer-v10','scenario-grouping-v6','PROCESSING',now() - interval '1 hour')`,
		runID, organizationID, recordingID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO analysis_jobs (
			id, organization_id, recording_id, analysis_run_id, status, progress,
			idempotency_key, attempt_count, started_at
		) VALUES ($1,$2,$3,$4,'PROCESSING',5,$5,1,now() - interval '1 hour')`,
		jobID, organizationID, recordingID, runID, "analysis:"+runID.String()); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reconciler := NewReconciler(pool, nil, logger)
	if err := reconciler.reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	assertStatus(t, ctx, pool, "analysis_jobs", jobID, "FAILED")
	assertStatus(t, ctx, pool, "analysis_runs", runID, "FAILED")
	assertStatus(t, ctx, pool, "recordings", recordingID, "FAILED")

	var jobError, runError string
	if err := pool.QueryRow(ctx, `
		SELECT last_error FROM analysis_jobs WHERE id=$1`, jobID).Scan(&jobError); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT error FROM analysis_runs WHERE id=$1`, runID).Scan(&runError); err != nil {
		t.Fatal(err)
	}
	if jobError != "worker timeout" || runError != "worker timeout" {
		t.Fatalf("unexpected timeout errors: job=%q run=%q", jobError, runError)
	}
}

func outboxTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
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
		var connErr error
		connectionString, connErr = container.ConnectionString(ctx, "sslmode=disable")
		if connErr != nil {
			t.Fatal(connErr)
		}
	}
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func assertStatus(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table string,
	id uuid.UUID,
	expected string,
) {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM `+table+` WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != expected {
		t.Fatalf("unexpected %s status: got %s want %s", table, status, expected)
	}
}
