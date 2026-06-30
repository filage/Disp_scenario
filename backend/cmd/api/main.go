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
	"github.com/redis/go-redis/v9"

	"github.com/example/dispscenario-analyst-v2/internal/analysis"
	"github.com/example/dispscenario-analyst-v2/internal/artifacts"
	authn "github.com/example/dispscenario-analyst-v2/internal/auth"
	"github.com/example/dispscenario-analyst-v2/internal/config"
	"github.com/example/dispscenario-analyst-v2/internal/credentials"
	"github.com/example/dispscenario-analyst-v2/internal/database"
	"github.com/example/dispscenario-analyst-v2/internal/demo"
	"github.com/example/dispscenario-analyst-v2/internal/httpserver"
	"github.com/example/dispscenario-analyst-v2/internal/jobs"
	"github.com/example/dispscenario-analyst-v2/internal/outbox"
	"github.com/example/dispscenario-analyst-v2/internal/pipeline"
	"github.com/example/dispscenario-analyst-v2/internal/recording"
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
	if cfg.S3EnsureBucket {
		if err := objectStorage.EnsureBucket(ctx); err != nil {
			logger.Error("storage initialization failed", "error", err)
			os.Exit(1)
		}
	}
	if cfg.SeedDemoFixtures {
		if err := demo.SeedFixtures(
			ctx, pool, objectStorage, cfg.DemoFixtureManifest, logger,
		); err != nil {
			logger.Error("demo fixture initialization failed", "error", err)
			os.Exit(1)
		}
	}
	authMiddleware, err := authn.New(ctx, cfg.AuthDisabled, cfg.OIDCIssuer, cfg.OIDCClientID)
	if err != nil {
		logger.Error("authentication initialization failed", "error", err)
		os.Exit(1)
	}

	var redisClient *redis.Client
	if cfg.JobBackend == "redis" {
		redisOptions := asynq.RedisClientOpt{
			Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB,
		}
		redisClient = redis.NewClient(&redis.Options{
			Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB,
		})
		defer func() {
			if err := redisClient.Close(); err != nil {
				logger.Error("redis client close failed", "error", err)
			}
		}()
		queueClient := asynq.NewClient(redisOptions)
		defer func() {
			if err := queueClient.Close(); err != nil {
				logger.Error("queue client close failed", "error", err)
			}
		}()
		dispatcher := outbox.NewDispatcher(pool, jobs.NewAsynqQueue(queueClient), logger)
		go func() {
			if err := dispatcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("outbox dispatcher stopped", "error", err)
			}
		}()
	} else {
		analysisPipeline := pipeline.NewService(
			pool,
			objectStorage,
			vision.GeminiProvider{APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel},
			logger,
		)
		processor := jobs.NewProcessorWithCredentials(
			pool, analysisPipeline, logger, "gemini", cfg.GeminiModel, credentialStore,
		)
		runner := jobs.NewPostgresRunner(pool, processor, logger)
		go func() {
			if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("postgres job runner stopped", "error", err)
			}
		}()
	}
	reconciler := outbox.NewReconciler(pool, objectStorage, logger)
	go func() {
		if err := reconciler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("job reconciler stopped", "error", err)
		}
	}()
	handler := httpserver.New(
		pool,
		recording.NewRepository(pool),
		analysis.NewService(pool, "gemini", cfg.GeminiModel),
		credentialStore,
		artifacts.New(pool, cfg.GeminiAPIKey, cfg.GeminiModel),
		authMiddleware,
		objectStorage,
		redisClient,
		cfg.APISharedSecret,
		logger,
		cfg.WebOrigin,
	)
	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           handler.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("api started", "address", cfg.APIAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("api shutdown failed", "error", err)
	}
}
