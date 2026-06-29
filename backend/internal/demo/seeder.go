package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/platform"
	"github.com/example/dispscenario-analyst-v2/internal/storage"
)

type manifest struct {
	Version int `json:"version"`
	Videos  []struct {
		File            string  `json:"file"`
		ObjectKey       string  `json:"objectKey"`
		ContentType     string  `json:"contentType"`
		SizeBytes       int64   `json:"sizeBytes"`
		SHA256          string  `json:"sha256"`
		DurationSeconds float64 `json:"durationSeconds"`
	} `json:"videos"`
}

func SeedFixtures(
	ctx context.Context,
	pool *pgxpool.Pool,
	objectStorage *storage.Storage,
	manifestPath string,
	logger *slog.Logger,
) error {
	if manifestPath == "" {
		return fmt.Errorf("DEMO_FIXTURE_MANIFEST is required when fixture seeding is enabled")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read demo fixture manifest: %w", err)
	}
	var fixtures manifest
	if err := json.Unmarshal(data, &fixtures); err != nil {
		return fmt.Errorf("decode demo fixture manifest: %w", err)
	}
	if fixtures.Version != 1 || len(fixtures.Videos) == 0 {
		return fmt.Errorf("unsupported or empty demo fixture manifest")
	}

	for _, video := range fixtures.Videos {
		if video.File == "" || video.ObjectKey == "" || video.SizeBytes <= 0 {
			return fmt.Errorf("invalid demo fixture manifest entry")
		}
		info, err := objectStorage.Stat(ctx, video.ObjectKey)
		if err != nil {
			return fmt.Errorf("stat demo fixture %s: %w", video.ObjectKey, err)
		}
		if info.Size != video.SizeBytes {
			return fmt.Errorf(
				"demo fixture size mismatch for %s: expected %d, got %d",
				video.ObjectKey, video.SizeBytes, info.Size,
			)
		}
		result, err := pool.Exec(ctx, `
			INSERT INTO recordings (
				organization_id, original_name, mime_type, size_bytes, duration_sec,
				status, source, object_key, checksum_sha256, created_by, updated_by
			) VALUES ($1,$2,$3,$4,$5,'UPLOADED','demo_fixture',$6,$7,'demo-seed','demo-seed')
			ON CONFLICT (object_key) DO NOTHING`,
			platform.LocalOrganizationID,
			video.File,
			video.ContentType,
			video.SizeBytes,
			video.DurationSeconds,
			video.ObjectKey,
			video.SHA256,
		)
		if err != nil {
			return fmt.Errorf("seed demo fixture %s: %w", video.File, err)
		}
		if result.RowsAffected() > 0 {
			logger.Info("demo fixture seeded", "file", video.File, "object_key", video.ObjectKey)
		}
	}
	return nil
}
