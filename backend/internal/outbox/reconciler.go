package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/observability"
	"github.com/example/dispscenario-analyst-v2/internal/storage"
)

type Reconciler struct {
	pool    *pgxpool.Pool
	logger  *slog.Logger
	storage *storage.Storage
}

func NewReconciler(pool *pgxpool.Pool, objectStorage *storage.Storage, logger *slog.Logger) *Reconciler {
	return &Reconciler{pool: pool, storage: objectStorage, logger: logger}
}

func (r *Reconciler) Run(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.reconcile(ctx); err != nil {
				r.logger.Error("job reconciliation failed", "error", err)
			}
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, analysis_run_id, recording_id, COALESCE(correlation_id, '')
		FROM analysis_jobs j
		WHERE j.status = 'QUEUED'
		  AND j.queue_task_id IS NULL
		  AND j.created_at < now() - interval '30 seconds'
		  AND NOT EXISTS (
		    SELECT 1 FROM outbox_events o
		    WHERE o.aggregate_type='analysis_job' AND o.aggregate_id=j.id
		      AND o.published_at IS NULL
		  )
		LIMIT 100`)
	if err != nil {
		return err
	}
	type job struct {
		id, runID, recordingID uuid.UUID
		correlationID          string
	}
	jobs := []job{}
	for rows.Next() {
		var item job
		if err := rows.Scan(&item.id, &item.runID, &item.recordingID, &item.correlationID); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, item)
	}
	rows.Close()
	for _, item := range jobs {
		payload, _ := json.Marshal(map[string]string{
			"jobId": item.id.String(), "analysisRunId": item.runID.String(),
			"recordingId":   item.recordingID.String(),
			"correlationId": item.correlationID,
		})
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
			VALUES ('analysis_job',$1,'analysis.requested',$2)`, item.id, payload); err != nil {
			observability.ReconciliationActions.WithLabelValues("missing_publication", "failed").Inc()
			return err
		}
		observability.ReconciliationActions.WithLabelValues("missing_publication", "recovered").Inc()
		r.logger.Warn(
			"missing queue publication reconciled",
			"job_id", item.id,
			"correlation_id", item.correlationID,
		)
	}

	result, err := r.pool.Exec(ctx, `
		WITH stale AS (
			UPDATE analysis_jobs
			SET status='FAILED', last_error='worker timeout', completed_at=now(), updated_at=now()
			WHERE status='PROCESSING' AND started_at < now() - interval '50 minutes'
			RETURNING analysis_run_id, recording_id
		),
		runs AS (
			UPDATE analysis_runs a
			SET status='FAILED', error='worker timeout', completed_at=now(), updated_at=now()
			FROM stale s WHERE a.id=s.analysis_run_id
			RETURNING s.recording_id
		)
		UPDATE recordings r
		SET status='FAILED', updated_at=now()
		FROM runs WHERE r.id=runs.recording_id`)
	if err != nil {
		observability.ReconciliationActions.WithLabelValues("worker_timeout", "failed").Inc()
		return err
	}
	if result.RowsAffected() > 0 {
		observability.ReconciliationActions.WithLabelValues("worker_timeout", "recovered").Add(float64(result.RowsAffected()))
	}
	expiredRows, err := r.pool.Query(ctx, `
		SELECT id FROM recordings
		WHERE status='PENDING_UPLOAD' AND created_at < now() - interval '24 hours'`)
	if err != nil {
		return err
	}
	defer expiredRows.Close()
	for expiredRows.Next() {
		var recordingID uuid.UUID
		if err := expiredRows.Scan(&recordingID); err != nil {
			return err
		}
		if err := r.storage.DeletePrefix(ctx, "recordings/"+recordingID.String()+"/"); err != nil {
			observability.ReconciliationActions.WithLabelValues("expired_upload", "failed").Inc()
			observability.CleanupFailures.WithLabelValues("expired_upload").Inc()
			r.logger.Warn("expired upload object cleanup failed",
				"recording_id", recordingID, "error", err)
			continue
		}
		if _, err := r.pool.Exec(ctx, `
			DELETE FROM recordings WHERE id=$1 AND status='PENDING_UPLOAD'`,
			recordingID); err != nil {
			return err
		}
		observability.ReconciliationActions.WithLabelValues("expired_upload", "recovered").Inc()
	}
	return expiredRows.Err()
}
