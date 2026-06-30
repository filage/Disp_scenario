package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/example/dispscenario-analyst-v2/internal/config"
	"github.com/example/dispscenario-analyst-v2/internal/credentials"
	"github.com/example/dispscenario-analyst-v2/internal/database"
	"github.com/example/dispscenario-analyst-v2/internal/jobs"
	"github.com/example/dispscenario-analyst-v2/internal/pipeline"
	"github.com/example/dispscenario-analyst-v2/internal/storage"
	"github.com/example/dispscenario-analyst-v2/internal/vision"
)

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
	credentialSecret := cfg.CredentialSecret
	if credentialSecret == "" {
		credentialSecret = cfg.APISharedSecret
	}
	if credentialSecret == "" {
		credentialSecret = cfg.GeminiAPIKey
	}
	credentialStore, err := credentials.NewStore(pool, credentialSecret)
	if err != nil {
		logger.Error("credential storage initialization failed", "error", err)
		os.Exit(1)
	}

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
	processor := jobs.NewProcessorWithCredentials(
		pool, pipelineService, logger, providerName, cfg.GeminiModel, credentialStore,
	)
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
		return processor.Process(taskCtx, task.Payload())
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
