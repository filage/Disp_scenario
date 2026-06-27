//go:build integration

package analysis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestRetryReusesActiveRetryRun(t *testing.T) {
	ctx := context.Background()
	pool := analysisTestPool(t, ctx)

	organizationID := uuid.New()
	recordingID := uuid.New()
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM recordings WHERE id=$1`, recordingID); err != nil {
			t.Errorf("delete test recording: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, organizationID); err != nil {
			t.Errorf("delete test organization: %v", err)
		}
	})
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO organizations (id, name) VALUES ($1, $2)`,
		organizationID, "Analysis retry test "+organizationID.String(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO recordings (
			id, organization_id, original_name, mime_type, size_bytes, status, object_key
		) VALUES ($1,$2,'retry.mp4','video/mp4',1024,'UPLOADED',$3)`,
		recordingID, organizationID, "recordings/"+recordingID.String()+"/source.mp4",
	); err != nil {
		t.Fatal(err)
	}

	service := NewService(pool, "gemini", "gemini-3.5-flash")
	original, err := service.Create(ctx, organizationID, recordingID, "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE analysis_runs
		SET status='FAILED', error='temporary provider failure', completed_at=now(), updated_at=now()
		WHERE id=$1`, original.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE analysis_jobs
		SET status='FAILED', last_error='temporary provider failure', completed_at=now(), updated_at=now()
		WHERE analysis_run_id=$1`, original.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE recordings
		SET status='FAILED', updated_at=now()
		WHERE id=$1`, recordingID); err != nil {
		t.Fatal(err)
	}

	firstRetry, err := service.Retry(ctx, organizationID, original.ID, "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	secondRetry, err := service.Retry(ctx, organizationID, original.ID, "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	if secondRetry.ID != firstRetry.ID {
		t.Fatalf("expected retry replay to reuse active run %s, got %s", firstRetry.ID, secondRetry.ID)
	}

	assertCount(t, ctx, pool, `
		SELECT count(*)
		FROM analysis_runs
		WHERE recording_id=$1 AND status='QUEUED'`, recordingID, 1)
	assertCount(t, ctx, pool, `
		SELECT count(*)
		FROM analysis_runs
		WHERE recording_id=$1`, recordingID, 2)
	assertCount(t, ctx, pool, `
		SELECT count(*)
		FROM analysis_jobs
		WHERE recording_id=$1`, recordingID, 2)
	assertCount(t, ctx, pool, `
		SELECT count(*)
		FROM outbox_events
		WHERE aggregate_type='analysis_job'
		  AND aggregate_id IN (SELECT id FROM analysis_jobs WHERE recording_id=$1)`, recordingID, 2)
}

func analysisTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
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

func assertCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	query string,
	arg uuid.UUID,
	expected int,
) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, arg).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("unexpected count for %q: got %d want %d", query, count, expected)
	}
}
