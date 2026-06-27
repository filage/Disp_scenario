package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/observability"
)

const (
	PromptVersion        = "video-raw-extractor-v8"
	NormalizationVersion = "vision-normalizer-v10"
	GroupingVersion      = "scenario-grouping-v6"
)

type Run struct {
	ID                   uuid.UUID  `json:"id"`
	RecordingID          uuid.UUID  `json:"recordingId"`
	Status               string     `json:"status"`
	Provider             string     `json:"provider"`
	Model                *string    `json:"model"`
	PromptVersion        string     `json:"promptVersion"`
	NormalizationVersion string     `json:"normalizationVersion"`
	GroupingVersion      string     `json:"groupingVersion"`
	RawText              *string    `json:"rawText,omitempty"`
	Error                *string    `json:"error"`
	InputTokens          *int64     `json:"inputTokens,omitempty"`
	OutputTokens         *int64     `json:"outputTokens,omitempty"`
	ThinkingTokens       *int64     `json:"thinkingTokens,omitempty"`
	TotalTokens          *int64     `json:"totalTokens,omitempty"`
	EstimatedCostUSD     *float64   `json:"estimatedCostUsd,omitempty"`
	PricingVersion       *string    `json:"pricingVersion,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	StartedAt            *time.Time `json:"startedAt,omitempty"`
	CompletedAt          *time.Time `json:"completedAt,omitempty"`
}

type Service struct {
	pool     *pgxpool.Pool
	provider string
	model    string
}

func NewService(pool *pgxpool.Pool, provider, model string) *Service {
	return &Service{pool: pool, provider: provider, model: model}
}

func (s *Service) Create(ctx context.Context, organizationID, recordingID uuid.UUID, actor string) (Run, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	run, err := s.createInTx(ctx, tx, organizationID, recordingID, actor)
	if err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Service) createInTx(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, recordingID uuid.UUID,
	actor string,
) (Run, error) {
	var run Run
	err := tx.QueryRow(ctx, `
		INSERT INTO analysis_runs (
			organization_id, recording_id, provider, model, prompt_version,
			normalization_version, grouping_version, status, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'QUEUED', $8)
		RETURNING id, recording_id, status::text, provider, model, prompt_version,
		          normalization_version, grouping_version, raw_text, error,
		          input_tokens, output_tokens, thinking_tokens, total_tokens,
		          estimated_cost_usd, pricing_version,
		          created_at, started_at, completed_at`,
		organizationID, recordingID, s.provider, s.model, PromptVersion,
		NormalizationVersion, GroupingVersion, actor,
	).Scan(
		&run.ID, &run.RecordingID, &run.Status, &run.Provider, &run.Model,
		&run.PromptVersion, &run.NormalizationVersion, &run.GroupingVersion,
		&run.RawText, &run.Error, &run.InputTokens, &run.OutputTokens,
		&run.ThinkingTokens, &run.TotalTokens, &run.EstimatedCostUSD, &run.PricingVersion,
		&run.CreatedAt, &run.StartedAt, &run.CompletedAt,
	)
	if err != nil {
		return Run{}, err
	}

	jobID := uuid.New()
	idempotencyKey := fmt.Sprintf("analysis:%s", run.ID)
	correlationID := observability.CorrelationID(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO analysis_jobs (
			id, organization_id, recording_id, analysis_run_id, idempotency_key,
			correlation_id
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		jobID, organizationID, recordingID, run.ID, idempotencyKey, nullString(correlationID))
	if err != nil {
		return Run{}, err
	}

	payload, err := json.Marshal(map[string]string{
		"jobId":         jobID.String(),
		"analysisRunId": run.ID.String(),
		"recordingId":   recordingID.String(),
		"correlationId": correlationID,
	})
	if err != nil {
		return Run{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ('analysis_job', $1, 'analysis.requested', $2)`, jobID, payload)
	if err != nil {
		return Run{}, err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE recordings
		SET status = 'PROCESSING', updated_at = now()
		WHERE id = $1 AND organization_id = $2`, recordingID, organizationID); err != nil {
		return Run{}, err
	}

	return run, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Service) List(ctx context.Context, organizationID uuid.UUID, recordingID *uuid.UUID) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, recording_id, status::text, provider, model, prompt_version,
		       normalization_version, grouping_version, raw_text, error,
		       input_tokens, output_tokens, thinking_tokens, total_tokens,
		       estimated_cost_usd, pricing_version,
		       created_at, started_at, completed_at
		FROM analysis_runs
		WHERE organization_id = $1
		  AND ($2::uuid IS NULL OR recording_id = $2)
		ORDER BY created_at DESC`, organizationID, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Run, 0)
	for rows.Next() {
		var item Run
		if err := rows.Scan(
			&item.ID, &item.RecordingID, &item.Status, &item.Provider, &item.Model,
			&item.PromptVersion, &item.NormalizationVersion, &item.GroupingVersion,
			&item.RawText, &item.Error, &item.InputTokens, &item.OutputTokens,
			&item.ThinkingTokens, &item.TotalTokens, &item.EstimatedCostUSD, &item.PricingVersion,
			&item.CreatedAt,
			&item.StartedAt, &item.CompletedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Get(ctx context.Context, organizationID, id uuid.UUID) (Run, error) {
	var item Run
	err := s.pool.QueryRow(ctx, `
		SELECT id, recording_id, status::text, provider, model, prompt_version,
		       normalization_version, grouping_version, raw_text, error,
		       input_tokens, output_tokens, thinking_tokens, total_tokens,
		       estimated_cost_usd, pricing_version,
		       created_at, started_at, completed_at
		FROM analysis_runs
		WHERE organization_id = $1 AND id = $2`, organizationID, id).Scan(
		&item.ID, &item.RecordingID, &item.Status, &item.Provider, &item.Model,
		&item.PromptVersion, &item.NormalizationVersion, &item.GroupingVersion,
		&item.RawText, &item.Error, &item.InputTokens, &item.OutputTokens,
		&item.ThinkingTokens, &item.TotalTokens, &item.EstimatedCostUSD, &item.PricingVersion,
		&item.CreatedAt,
		&item.StartedAt, &item.CompletedAt,
	)
	return item, err
}

func (s *Service) Cancel(ctx context.Context, organizationID, id uuid.UUID) (Run, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item Run
	err = tx.QueryRow(ctx, `
		UPDATE analysis_runs SET status='CANCELLED', completed_at=now(), updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND status IN ('QUEUED','PROCESSING','NORMALIZING')
		RETURNING id, recording_id, status::text, provider, model, prompt_version,
		          normalization_version, grouping_version, raw_text, error,
		          input_tokens, output_tokens, thinking_tokens, total_tokens,
		          estimated_cost_usd, pricing_version,
		          created_at, started_at, completed_at`,
		organizationID, id,
	).Scan(
		&item.ID, &item.RecordingID, &item.Status, &item.Provider, &item.Model,
		&item.PromptVersion, &item.NormalizationVersion, &item.GroupingVersion,
		&item.RawText, &item.Error, &item.InputTokens, &item.OutputTokens,
		&item.ThinkingTokens, &item.TotalTokens, &item.EstimatedCostUSD, &item.PricingVersion,
		&item.CreatedAt,
		&item.StartedAt, &item.CompletedAt,
	)
	if err != nil {
		return Run{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE analysis_jobs SET status='CANCELLED', completed_at=now(), updated_at=now()
		WHERE analysis_run_id=$1 AND status IN ('QUEUED','PROCESSING')`, id); err != nil {
		return Run{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE recordings SET status='UPLOADED', updated_at=now() WHERE id=$1`, item.RecordingID); err != nil {
		return Run{}, err
	}
	return item, tx.Commit(ctx)
}

func (s *Service) Retry(ctx context.Context, organizationID, id uuid.UUID, actor string) (Run, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var item Run
	err = tx.QueryRow(ctx, `
		SELECT id, recording_id, status::text, provider, model, prompt_version,
		       normalization_version, grouping_version, raw_text, error,
		       input_tokens, output_tokens, thinking_tokens, total_tokens,
		       estimated_cost_usd, pricing_version,
		       created_at, started_at, completed_at
		FROM analysis_runs
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, id).Scan(
		&item.ID, &item.RecordingID, &item.Status, &item.Provider, &item.Model,
		&item.PromptVersion, &item.NormalizationVersion, &item.GroupingVersion,
		&item.RawText, &item.Error, &item.InputTokens, &item.OutputTokens,
		&item.ThinkingTokens, &item.TotalTokens, &item.EstimatedCostUSD, &item.PricingVersion,
		&item.CreatedAt,
		&item.StartedAt, &item.CompletedAt,
	)
	if err != nil {
		return Run{}, err
	}
	if item.Status != "FAILED" && item.Status != "CANCELLED" {
		return Run{}, fmt.Errorf("only failed or cancelled runs can be retried")
	}
	var lockedRecordingID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id
		FROM recordings
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, item.RecordingID).Scan(&lockedRecordingID); err != nil {
		return Run{}, err
	}

	var existing Run
	err = tx.QueryRow(ctx, `
		SELECT id, recording_id, status::text, provider, model, prompt_version,
		       normalization_version, grouping_version, raw_text, error,
		       input_tokens, output_tokens, thinking_tokens, total_tokens,
		       estimated_cost_usd, pricing_version,
		       created_at, started_at, completed_at
		FROM analysis_runs
		WHERE organization_id = $1
		  AND recording_id = $2
		  AND id <> $3
		  AND created_at > $4
		  AND status IN ('QUEUED','PROCESSING','NORMALIZING')
		ORDER BY created_at ASC
		LIMIT 1`, organizationID, item.RecordingID, item.ID, item.CreatedAt).Scan(
		&existing.ID, &existing.RecordingID, &existing.Status, &existing.Provider, &existing.Model,
		&existing.PromptVersion, &existing.NormalizationVersion, &existing.GroupingVersion,
		&existing.RawText, &existing.Error, &existing.InputTokens, &existing.OutputTokens,
		&existing.ThinkingTokens, &existing.TotalTokens,
		&existing.EstimatedCostUSD, &existing.PricingVersion, &existing.CreatedAt,
		&existing.StartedAt, &existing.CompletedAt,
	)
	if err == nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return Run{}, commitErr
		}
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return Run{}, err
	}

	retry, err := s.createInTx(ctx, tx, organizationID, item.RecordingID, actor)
	if err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return retry, nil
}
