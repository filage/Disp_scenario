package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PayloadProcessor interface {
	Process(context.Context, json.RawMessage) error
}

type PostgresRunner struct {
	pool         *pgxpool.Pool
	processor    PayloadProcessor
	logger       *slog.Logger
	pollEvery    time.Duration
	maxAttempts  int
	lastRecovery time.Time
}

func NewPostgresRunner(
	pool *pgxpool.Pool,
	processor PayloadProcessor,
	logger *slog.Logger,
) *PostgresRunner {
	return &PostgresRunner{
		pool: pool, processor: processor, logger: logger,
		pollEvery: time.Second, maxAttempts: 5,
	}
}

func (r *PostgresRunner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.pollEvery)
	defer ticker.Stop()

	for {
		processed, err := r.runOne(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			r.logger.Error("postgres job runner failed", "error", err)
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *PostgresRunner) runOne(ctx context.Context) (bool, error) {
	if time.Since(r.lastRecovery) >= time.Minute {
		if err := r.recoverStale(ctx); err != nil {
			return false, err
		}
		r.lastRecovery = time.Now()
	}
	var payload AnalysisPayload
	err := r.pool.QueryRow(ctx, `
		UPDATE analysis_jobs
		SET locked_at=now(), queue_task_id=COALESCE(queue_task_id, 'postgres:' || id::text),
		    updated_at=now()
		WHERE id = (
			SELECT id
			FROM analysis_jobs
			WHERE (
				status='QUEUED'
				OR (
					status='FAILED' AND attempt_count < $1
					AND updated_at < now() - interval '15 seconds'
				)
			)
			AND (locked_at IS NULL OR locked_at < now() - interval '30 minutes')
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id::text, analysis_run_id::text, recording_id::text,
		          COALESCE(correlation_id, ''), COALESCE(requested_by, '')`, r.maxAttempts).Scan(
		&payload.JobID,
		&payload.AnalysisRunID,
		&payload.RecordingID,
		&payload.CorrelationID,
		&payload.RequestedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer r.releaseLock(payload.JobID)

	_, err = r.pool.Exec(ctx, `
		UPDATE outbox_events
		SET published_at=COALESCE(published_at, now()),
		    attempts=attempts + CASE WHEN published_at IS NULL THEN 1 ELSE 0 END,
		    last_error=NULL
		WHERE aggregate_type='analysis_job' AND aggregate_id=$1::uuid`, payload.JobID)
	if err != nil {
		return true, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return true, err
	}
	if err := r.processor.Process(ctx, raw); err != nil {
		r.logger.Error("analysis job attempt failed",
			"job_id", payload.JobID,
			"analysis_run_id", payload.AnalysisRunID,
			"error", err,
		)
	}
	return true, nil
}

func (r *PostgresRunner) releaseLock(jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := r.pool.Exec(ctx, `
		UPDATE analysis_jobs SET locked_at=NULL, updated_at=now()
		WHERE id=$1::uuid AND locked_at IS NOT NULL`, jobID); err != nil {
		r.logger.Error("postgres job lock release failed", "job_id", jobID, "error", err)
	}
}

func (r *PostgresRunner) recoverStale(ctx context.Context) error {
	result, err := r.pool.Exec(ctx, `
		WITH stale AS (
			UPDATE analysis_jobs
			SET status='FAILED', locked_at=NULL, last_error='postgres runner lease expired',
			    completed_at=now(), updated_at=now()
			WHERE status='PROCESSING' AND started_at < now() - interval '20 minutes'
			RETURNING analysis_run_id, recording_id
		),
		runs AS (
			UPDATE analysis_runs a
			SET status='FAILED', error='postgres runner lease expired',
			    completed_at=now(), updated_at=now()
			FROM stale s WHERE a.id=s.analysis_run_id
			RETURNING s.recording_id
		)
		UPDATE recordings r
		SET status='FAILED', updated_at=now()
		FROM runs WHERE r.id=runs.recording_id`)
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		r.logger.Warn("stale postgres jobs recovered", "count", result.RowsAffected())
	}
	return nil
}
