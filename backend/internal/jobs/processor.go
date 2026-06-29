package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/observability"
	"github.com/example/dispscenario-analyst-v2/internal/pipeline"
)

type AnalysisPipeline interface {
	Process(context.Context, pipeline.Job) error
}

type Processor struct {
	pool         *pgxpool.Pool
	pipeline     AnalysisPipeline
	logger       *slog.Logger
	providerName string
}

func NewProcessor(
	pool *pgxpool.Pool,
	analysisPipeline AnalysisPipeline,
	logger *slog.Logger,
	providerName string,
) *Processor {
	return &Processor{
		pool: pool, pipeline: analysisPipeline, logger: logger, providerName: providerName,
	}
}

func (p *Processor) Process(ctx context.Context, raw json.RawMessage) error {
	startedAt := time.Now()
	resultLabel := "failed"
	defer func() {
		observability.AnalysisJobs.WithLabelValues(resultLabel, p.providerName).Inc()
		observability.AnalysisDuration.WithLabelValues(p.providerName).Observe(time.Since(startedAt).Seconds())
	}()

	payload, err := DecodeAnalysisPayload(raw)
	if err != nil {
		return err
	}
	jobID, err := uuid.Parse(payload.JobID)
	if err != nil {
		return fmt.Errorf("invalid job id: %w", err)
	}
	runID, err := uuid.Parse(payload.AnalysisRunID)
	if err != nil {
		return fmt.Errorf("invalid run id: %w", err)
	}
	recordingID, err := uuid.Parse(payload.RecordingID)
	if err != nil {
		return fmt.Errorf("invalid recording id: %w", err)
	}
	if payload.CorrelationID == "" {
		payload.CorrelationID = jobID.String()
	}
	ctx = observability.WithCorrelationID(ctx, payload.CorrelationID)

	p.logger.Info("analysis job started",
		"job_id", jobID,
		"analysis_run_id", runID,
		"recording_id", recordingID,
		"correlation_id", payload.CorrelationID,
	)
	var attempt int
	err = p.pool.QueryRow(ctx, `
		UPDATE analysis_jobs
		SET status='PROCESSING', progress=5, started_at=now(), completed_at=NULL,
		    attempt_count=attempt_count + 1, last_error=NULL, updated_at=now()
		WHERE id=$1 AND status IN ('QUEUED','FAILED')
		RETURNING attempt_count`, jobID).Scan(&attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		p.logger.Info("analysis job delivery skipped",
			"job_id", jobID,
			"correlation_id", payload.CorrelationID,
		)
		resultLabel = "skipped"
		return nil
	}
	if err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = p.pool.Exec(unlockCtx, `UPDATE analysis_jobs SET locked_at=NULL WHERE id=$1`, jobID)
	}()
	_, _ = p.pool.Exec(ctx, `
		INSERT INTO job_attempts (analysis_job_id, attempt, status)
		VALUES ($1,$2,'PROCESSING')
		ON CONFLICT (analysis_job_id, attempt) DO UPDATE SET
			status='PROCESSING', started_at=now(), completed_at=NULL, error=NULL`,
		jobID, attempt)
	if _, err = p.pool.Exec(ctx, `
		UPDATE analysis_runs
		SET status='PROCESSING', started_at=now(), completed_at=NULL,
		    error=NULL, updated_at=now()
		WHERE id=$1`, runID); err != nil {
		return err
	}
	if _, err = p.pool.Exec(ctx, `
		UPDATE recordings SET status='PROCESSING', updated_at=now() WHERE id=$1`,
		recordingID); err != nil {
		return err
	}

	err = p.pipeline.Process(ctx, pipeline.Job{
		ID: jobID, AnalysisRunID: runID, RecordingID: recordingID,
		CorrelationID: payload.CorrelationID,
	})
	if err != nil {
		_, _ = p.pool.Exec(ctx, `
			UPDATE analysis_jobs
			SET status='FAILED', last_error=$2, locked_at=NULL,
			    completed_at=now(), updated_at=now()
			WHERE id=$1`, jobID, err.Error())
		_, _ = p.pool.Exec(ctx, `
			UPDATE analysis_runs
			SET status='FAILED', error=$2, completed_at=now(), updated_at=now()
			WHERE id=$1`, runID, err.Error())
		_, _ = p.pool.Exec(ctx, `
			UPDATE recordings SET status='FAILED', updated_at=now() WHERE id=$1`, recordingID)
		_, _ = p.pool.Exec(ctx, `
			UPDATE job_attempts
			SET status='FAILED', error=$3, completed_at=now()
			WHERE analysis_job_id=$1 AND attempt=$2`, jobID, attempt, err.Error())
		return err
	}
	_, _ = p.pool.Exec(ctx, `
		UPDATE job_attempts
		SET status='COMPLETED', completed_at=now()
		WHERE analysis_job_id=$1 AND attempt=$2`, jobID, attempt)
	resultLabel = "completed"
	p.logger.Info("analysis job completed",
		"job_id", jobID,
		"analysis_run_id", runID,
		"recording_id", recordingID,
		"correlation_id", payload.CorrelationID,
	)
	return nil
}
