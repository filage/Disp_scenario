package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment         string
	APIAddr             string
	WorkerMetricsAddr   string
	WebOrigin           string
	APISharedSecret     string
	DatabaseURL         string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	JobBackend          string
	S3Endpoint          string
	S3PublicEndpoint    string
	S3AccessKey         string
	S3SecretKey         string
	S3Bucket            string
	S3UseSSL            bool
	S3Region            string
	AuthDisabled        bool
	OIDCIssuer          string
	OIDCClientID        string
	GeminiAPIKey        string
	GeminiModel         string
	SeedDemoFixtures    bool
	DemoFixtureManifest string
}

func Load() (Config, error) {
	redisDB, err := strconv.Atoi(value("REDIS_DB", "0"))
	if err != nil {
		return Config{}, fmt.Errorf("REDIS_DB: %w", err)
	}
	s3SSL, err := strconv.ParseBool(value("S3_USE_SSL", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("S3_USE_SSL: %w", err)
	}
	authDisabled, err := strconv.ParseBool(value("AUTH_DISABLED", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_DISABLED: %w", err)
	}
	seedDemoFixtures, err := strconv.ParseBool(value("SEED_DEMO_FIXTURES", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("SEED_DEMO_FIXTURES: %w", err)
	}
	jobBackend := strings.ToLower(value("JOB_BACKEND", "redis"))
	if jobBackend != "redis" && jobBackend != "postgres" {
		return Config{}, fmt.Errorf("JOB_BACKEND must be redis or postgres")
	}
	apiAddr := os.Getenv("API_ADDR")
	if apiAddr == "" {
		if port := os.Getenv("PORT"); port != "" {
			apiAddr = ":" + port
		} else {
			apiAddr = ":8787"
		}
	}

	cfg := Config{
		Environment:         value("APP_ENV", "development"),
		APIAddr:             apiAddr,
		WorkerMetricsAddr:   value("WORKER_METRICS_ADDR", ":9091"),
		WebOrigin:           value("WEB_ORIGIN", "http://localhost:3000"),
		APISharedSecret:     os.Getenv("API_SHARED_SECRET"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisAddr:           value("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       os.Getenv("REDIS_PASSWORD"),
		RedisDB:             redisDB,
		JobBackend:          jobBackend,
		S3Endpoint:          value("S3_ENDPOINT", "http://localhost:9000"),
		S3PublicEndpoint:    value("S3_PUBLIC_ENDPOINT", "http://localhost:9000"),
		S3AccessKey:         os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:         os.Getenv("S3_SECRET_KEY"),
		S3Bucket:            value("S3_BUCKET", "analyst-recordings"),
		S3UseSSL:            s3SSL,
		S3Region:            value("S3_REGION", "us-east-1"),
		AuthDisabled:        authDisabled,
		OIDCIssuer:          os.Getenv("OIDC_ISSUER"),
		OIDCClientID:        os.Getenv("OIDC_CLIENT_ID"),
		GeminiAPIKey:        os.Getenv("GEMINI_API_KEY"),
		GeminiModel:         value("GEMINI_MODEL", "gemini-3.5-flash"),
		SeedDemoFixtures:    seedDemoFixtures,
		DemoFixtureManifest: os.Getenv("DEMO_FIXTURE_MANIFEST"),
	}

	for name, current := range map[string]string{
		"DATABASE_URL":  cfg.DatabaseURL,
		"S3_ACCESS_KEY": cfg.S3AccessKey,
		"S3_SECRET_KEY": cfg.S3SecretKey,
	} {
		if current == "" {
			return Config{}, fmt.Errorf("%s is required", name)
		}
	}

	return cfg, nil
}

func value(name, fallback string) string {
	if current := os.Getenv(name); current != "" {
		return current
	}
	return fallback
}
