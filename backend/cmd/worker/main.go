package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/example/dispscenario-analyst-v2/internal/config"
	"github.com/example/dispscenario-analyst-v2/internal/database"
	"github.com/example/dispscenario-analyst-v2/internal/jobs"
	"github.com/example/dispscenario-analyst-v2/internal/observability"
	"github.com/example/dispscenario-analyst-v2/internal/pipeline"
	"github.com/example/dispscenario-analyst-v2/internal/storage"
	"github.com/example/dispscenario-analyst-v2/internal/vision"
)

type analysisPayload struct {
	JobID         string `json:"jobId"`
	AnalysisRunID string `json:"analysisRunId"`
	RecordingID   string `json:"recordingId"`
	CorrelationID string `json:"correlationId"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	if cfg.GeminiAPIKey == "" {
		logger.Error("configuration failed", "error", "GEMINI_API_KEY is required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.OpenPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	objectStorage, err := storage.New(
		cfg.S3Endpoint, cfg.S3PublicEndpoint, cfg.S3AccessKey,
		cfg.S3SecretKey, cfg.S3Bucket, cfg.S3Region, cfg.S3UseSSL,
	)
	if err != nil {
		logger.Error("storage configuration failed", "error", err)
		os.Exit(1)
	}
	var provider vision.Provider = vision.GeminiProvider{
		APIKey: cfg.GeminiAPIKey,
		Model:  cfg.GeminiModel,
	}
	const providerName = "gemini"
	pipelineService := pipeline.NewService(pool, objectStorage, provider, logger)
	metricsServer := &http.Server{
		Addr: cfg.WorkerMetricsAddr, Handler: promhttp.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("worker metrics server failed", "error", err)
			stop()
		}
	}()

	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB},
		asynq.Config{
			Concurrency:     2,
			Queues:          map[string]int{"analysis": 10},
			ShutdownTimeout: 30 * time.Second,
		},
	)
	mux := asynq.NewServeMux()
	mux.HandleFunc(jobs.AnalysisTask, func(taskCtx context.Context, task *asynq.Task) error {
		startedAt := time.Now()
		resultLabel := "failed"
		defer func() {
			observability.AnalysisJobs.WithLabelValues(resultLabel, providerName).Inc()
			observability.AnalysisDuration.WithLabelValues(providerName).Observe(time.Since(startedAt).Seconds())
		}()
		var payload analysisPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("decode task: %w", err)
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
		taskCtx = observability.WithCorrelationID(taskCtx, payload.CorrelationID)

		logger.Info("analysis job started",
			"job_id", jobID,
			"analysis_run_id", runID,
			"recording_id", recordingID,
			"correlation_id", payload.CorrelationID,
		)
		var attempt int
		err = pool.QueryRow(taskCtx, `
			UPDATE analysis_jobs
			SET status = 'PROCESSING', progress = 5, started_at = COALESCE(started_at, now()),
			    attempt_count = attempt_count + 1, updated_at = now()
			WHERE id = $1 AND status IN ('QUEUED','FAILED')
			RETURNING attempt_count`, jobID).Scan(&attempt)
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Info(
				"analysis job delivery skipped",
				"job_id", jobID,
				"correlation_id", payload.CorrelationID,
			)
			return nil
		}
		if err != nil {
			return err
		}
		_, _ = pool.Exec(taskCtx, `
			INSERT INTO job_attempts (analysis_job_id, attempt, status)
			VALUES ($1,$2,'PROCESSING')
			ON CONFLICT (analysis_job_id, attempt) DO UPDATE SET
				status='PROCESSING', started_at=now(), completed_at=NULL, error=NULL`,
			jobID, attempt)
		_, err = pool.Exec(taskCtx, `
			UPDATE analysis_runs
			SET status = 'PROCESSING', started_at = COALESCE(started_at, now()), updated_at = now()
			WHERE id = $1`, runID)
		if err != nil {
			return err
		}

		err = pipelineService.Process(taskCtx, pipeline.Job{
			ID: jobID, AnalysisRunID: runID, RecordingID: recordingID,
			CorrelationID: payload.CorrelationID,
		})
		if err != nil {
			_, _ = pool.Exec(taskCtx, `
				UPDATE analysis_jobs
				SET status = 'FAILED', last_error = $2, updated_at = now()
				WHERE id = $1`, jobID, err.Error())
			_, _ = pool.Exec(taskCtx, `
				UPDATE analysis_runs
				SET status = 'FAILED', error = $2, completed_at = now(), updated_at = now()
				WHERE id = $1`, runID, err.Error())
			_, _ = pool.Exec(taskCtx, `
				UPDATE recordings SET status = 'FAILED', updated_at = now() WHERE id = $1`, recordingID)
			_, _ = pool.Exec(taskCtx, `
				UPDATE job_attempts
				SET status='FAILED', error=$3, completed_at=now()
				WHERE analysis_job_id=$1 AND attempt=$2`, jobID, attempt, err.Error())
			return err
		}
		_, _ = pool.Exec(taskCtx, `
			UPDATE job_attempts
			SET status='COMPLETED', completed_at=now()
			WHERE analysis_job_id=$1 AND attempt=$2`, jobID, attempt)
		resultLabel = "completed"
		logger.Info("analysis job completed",
			"job_id", jobID,
			"analysis_run_id", runID,
			"recording_id", recordingID,
			"correlation_id", payload.CorrelationID,
		)
		return nil
	})

	go func() {
		<-ctx.Done()
		server.Shutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()

	logger.Info("worker started")
	if err := server.Run(mux); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("worker failed", "error", err)
		os.Exit(1)
	}
}
