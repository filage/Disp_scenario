package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
	"github.com/example/dispscenario-analyst-v2/internal/media"
	"github.com/example/dispscenario-analyst-v2/internal/observability"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
	"github.com/example/dispscenario-analyst-v2/internal/scenario"
	"github.com/example/dispscenario-analyst-v2/internal/storage"
	"github.com/example/dispscenario-analyst-v2/internal/vision"
)

type Job struct {
	ID            uuid.UUID
	AnalysisRunID uuid.UUID
	RecordingID   uuid.UUID
	CorrelationID string
	Provider      vision.Provider
}

type Service struct {
	pool     *pgxpool.Pool
	storage  *storage.Storage
	provider vision.Provider
	logger   *slog.Logger
}

func NewService(
	pool *pgxpool.Pool,
	objectStorage *storage.Storage,
	provider vision.Provider,
	logger *slog.Logger,
) *Service {
	return &Service{pool: pool, storage: objectStorage, provider: provider, logger: logger}
}

func (s *Service) Process(ctx context.Context, job Job) error {
	var objectKey, mimeType string
	if err := s.pool.QueryRow(ctx, `
		SELECT object_key, mime_type FROM recordings WHERE id = $1`, job.RecordingID).Scan(&objectKey, &mimeType); err != nil {
		return fmt.Errorf("load recording: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "analyst-"+job.ID.String()+"-")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			observability.CleanupFailures.WithLabelValues("pipeline_temp").Inc()
			s.logger.Warn(
				"pipeline temporary directory cleanup failed",
				"path", tempDir,
				"correlation_id", job.CorrelationID,
				"job_id", job.ID,
				"error", err,
			)
		}
	}()
	extension := ".mp4"
	if mimeType == "video/webm" {
		extension = ".webm"
	}
	videoPath := filepath.Join(tempDir, "source"+extension)
	if err := s.storage.Download(ctx, objectKey, videoPath); err != nil {
		return fmt.Errorf("download recording: %w", err)
	}
	metadata, err := media.Probe(ctx, videoPath)
	if err != nil {
		return err
	}
	geminiStarted := time.Now()
	provider := job.Provider
	if provider == nil {
		provider = s.provider
	}
	extraction, err := provider.Extract(ctx, job.AnalysisRunID, videoPath, metadata)
	observability.ObserveDependency("gemini", "extract_video", geminiStarted, err)
	if err != nil {
		if persistErr := s.persistRunUsage(ctx, job.AnalysisRunID, extraction); persistErr != nil {
			s.logger.Warn(
				"persist analysis run usage after provider error failed",
				"analysis_run_id", job.AnalysisRunID,
				"error", persistErr,
			)
		}
		return err
	}

	normalized := domain.Normalize(job.RecordingID, job.AnalysisRunID, extraction.RawEvents)
	s.prewarmEvidenceFrames(ctx, job.RecordingID, videoPath, tempDir, normalized.ActionEvents)
	issues := append([]domain.DataQualityIssue(nil), normalized.DataQualityIssues...)
	scenarioConfig, err := scenario.LoadConfig(ctx, s.pool)
	if err != nil {
		return fmt.Errorf("load scenario config: %w", err)
	}
	instances := domain.BuildScenarioInstancesWithConfig(
		job.RecordingID, normalized.ActionEvents, &issues, scenarioConfig,
	)
	groups := domain.BuildScenarioGroups(instances, normalized.ActionEvents)
	metrics := domain.CalculateMetrics(instances, normalized.ActionEvents)
	graph := domain.BuildGraph(instances, normalized.ActionEvents)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.replaceArtifacts(ctx, tx, job, extraction, metadata, normalized, instances, groups, issues, metrics, graph); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) prewarmEvidenceFrames(
	ctx context.Context,
	recordingID uuid.UUID,
	videoPath string,
	tempDir string,
	events []domain.ActionEvent,
) {
	seen := make(map[int]struct{}, len(events))
	generated := 0
	for _, event := range events {
		if _, exists := seen[event.TimestampMS]; exists {
			continue
		}
		seen[event.TimestampMS] = struct{}{}

		framePath := filepath.Join(tempDir, fmt.Sprintf("evidence-%d.jpg", event.TimestampMS))
		if err := media.ExtractEvidenceFrame(ctx, videoPath, framePath, event.TimestampMS); err != nil {
			s.logger.Warn(
				"prewarm evidence frame failed",
				"recording_id", recordingID,
				"timestamp_ms", event.TimestampMS,
				"error", err,
			)
			continue
		}
		key := fmt.Sprintf("recordings/%s/evidence/%d.jpg", recordingID, event.TimestampMS)
		if err := s.storage.Upload(ctx, key, framePath, "image/jpeg"); err != nil {
			s.logger.Warn(
				"upload prewarmed evidence frame failed",
				"recording_id", recordingID,
				"timestamp_ms", event.TimestampMS,
				"error", err,
			)
			continue
		}
		generated++
	}
	if generated > 0 {
		s.logger.Info(
			"evidence frames prewarmed",
			"recording_id", recordingID,
			"frames", generated,
		)
	}
}

func (s *Service) replaceArtifacts(
	ctx context.Context,
	tx pgx.Tx,
	job Job,
	extraction vision.Result,
	metadata media.Metadata,
	normalized domain.NormalizeResult,
	instances []domain.ScenarioInstance,
	groups []domain.ScenarioTemplate,
	issues []domain.DataQualityIssue,
	metrics domain.ScenarioMetrics,
	graph domain.Graph,
) error {
	// Match the legacy replaceAnalysis contract: a successful re-analysis
	// replaces the recording's active artifacts while run history remains.
	for _, statement := range []string{
		"DELETE FROM data_quality_issues WHERE recording_id = $1",
		"DELETE FROM scenario_instances WHERE recording_id = $1",
		"DELETE FROM scenario_graphs WHERE recording_id = $1",
		"DELETE FROM analyst_reports WHERE recording_id = $1",
		"DELETE FROM action_events WHERE recording_id = $1",
		`DELETE FROM raw_vision_events r
		 USING analysis_runs a
		 WHERE r.analysis_run_id = a.id AND a.recording_id = $1`,
	} {
		if _, err := tx.Exec(ctx, statement, job.RecordingID); err != nil {
			return err
		}
	}

	for _, event := range extraction.RawEvents {
		if _, err := tx.Exec(ctx, `
			INSERT INTO raw_vision_events (
				id, analysis_run_id, timestamp_ms, screen, visible_text, target,
				event_type_guess, color_cues, state_change, confidence, payload
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			event.ID, job.AnalysisRunID, event.TimestampMS, event.Screen, event.VisibleText,
			event.Target, event.EventTypeGuess, domain.JSON(event.ColorCues),
			event.StateChange, event.Confidence, domain.JSON(event.Payload)); err != nil {
			return err
		}
	}
	for _, event := range normalized.ActionEvents {
		if _, err := tx.Exec(ctx, `
			INSERT INTO action_events (
				id, recording_id, analysis_run_id, timestamp_ms, canonical_action,
				event_type, screen, entity_type, entity_id, order_id, issue_type,
				target, confidence, source_raw_event_ids, quality_flags, payload, source
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			event.ID, job.RecordingID, job.AnalysisRunID, event.TimestampMS,
			event.CanonicalAction, event.EventType, event.Screen, nullString(event.EntityType),
			nullString(event.EntityID), nullString(event.OrderID), nullString(event.IssueType),
			event.Target, event.Confidence, domain.JSON(event.SourceRawEventIDs),
			domain.JSON(event.QualityFlags), domain.JSON(event.Payload), event.Source); err != nil {
			return err
		}
	}

	templateByInstance := map[uuid.UUID]uuid.UUID{}
	for _, group := range groups {
		var persistedTemplateID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO scenario_templates (
				id, organization_id, code, name, issue_type, signature, frequency,
				average_duration_ms, median_duration_ms, p95_duration_ms,
				manual_check_count, repeated_action_count, confidence_average,
				ambiguous_count, automation_score, action_sequence, metrics, status,
				updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,now())
			ON CONFLICT (organization_id, signature) DO UPDATE SET
				code = EXCLUDED.code, name = EXCLUDED.name, issue_type = EXCLUDED.issue_type,
				frequency = EXCLUDED.frequency, average_duration_ms = EXCLUDED.average_duration_ms,
				median_duration_ms = EXCLUDED.median_duration_ms, p95_duration_ms = EXCLUDED.p95_duration_ms,
				manual_check_count = EXCLUDED.manual_check_count,
				repeated_action_count = EXCLUDED.repeated_action_count,
				confidence_average = EXCLUDED.confidence_average,
				ambiguous_count = EXCLUDED.ambiguous_count,
				automation_score = EXCLUDED.automation_score,
				action_sequence = EXCLUDED.action_sequence, metrics = EXCLUDED.metrics,
				status = EXCLUDED.status, updated_at = now()
			RETURNING id`,
			group.ID, platform.LocalOrganizationID, group.Code, group.Name, group.IssueType,
			group.Signature, group.Frequency, group.AverageDurationMS, group.MedianDurationMS,
			group.P95DurationMS, group.ManualCheckCount, group.RepeatedActionCount,
			group.ConfidenceAverage, group.AmbiguousCount, group.AutomationScore,
			domain.JSON(group.ActionSequence), domain.JSON(group.Metrics), group.Status,
		).Scan(&persistedTemplateID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM automation_candidates WHERE template_id = $1", persistedTemplateID); err != nil {
			return err
		}
		for _, candidate := range group.AutomationCandidates {
			if _, err := tx.Exec(ctx, `
				INSERT INTO automation_candidates (
					id, template_id, title, type, rationale, affected_steps,
					impact, confidence, score, status, breakdown
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
				candidate.ID, persistedTemplateID, candidate.Title, candidate.Type, candidate.Rationale,
				domain.JSON(candidate.AffectedSteps), candidate.Impact, candidate.Confidence,
				candidate.Score, candidate.Status, domain.JSON(candidate.Breakdown)); err != nil {
				return err
			}
		}
		for _, instanceID := range group.InstanceIDs {
			templateByInstance[instanceID] = persistedTemplateID
		}
	}

	for _, instance := range instances {
		templateID := templateByInstance[instance.ID]
		if _, err := tx.Exec(ctx, `
			INSERT INTO scenario_instances (
				id, recording_id, analysis_run_id, template_id, known_scenario_code, order_id,
				entity_type, entity_id, issue_type, started_at_ms, ended_at_ms,
				duration_ms, event_ids, outcome, status, confidence,
				boundary_rule_version, quality_flags
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			instance.ID, job.RecordingID, job.AnalysisRunID, templateID, nullString(instance.KnownScenarioCode),
			nullString(instance.OrderID), nullString(instance.EntityType), nullString(instance.EntityID),
			instance.IssueType, instance.StartedAtMS, instance.EndedAtMS, instance.DurationMS,
			domain.JSON(instance.EventIDs), instance.Outcome, instance.Status, instance.Confidence,
			instance.BoundaryRuleVersion, domain.JSON(instance.QualityFlags)); err != nil {
			return err
		}
	}
	for _, issue := range issues {
		if _, err := tx.Exec(ctx, `
			INSERT INTO data_quality_issues (
				id, recording_id, analysis_run_id, raw_vision_event_id, action_event_id,
				type, severity, message, timestamp_ms, resolved, payload
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			issue.ID, job.RecordingID, job.AnalysisRunID, issue.RawVisionEventID,
			issue.ActionEventID, issue.Type, issue.Severity, issue.Message,
			issue.TimestampMS, issue.Resolved, domain.JSON(issue.Payload)); err != nil {
			return err
		}
	}

	graphData, _ := json.Marshal(graph)
	metricsData, _ := json.Marshal(metrics)
	if _, err := tx.Exec(ctx, `
		INSERT INTO scenario_graphs (recording_id, analysis_run_id, graph, metrics)
		VALUES ($1, $2, $3, $4)`,
		job.RecordingID, job.AnalysisRunID, graphData, metricsData); err != nil {
		return err
	}

	summary := fmt.Sprintf(
		"Построено %d экземпляров и %d групп сценариев. Нормализация, границы и метрики рассчитаны детерминированным Go-кодом.",
		len(instances), len(groups),
	)
	observations := []string{
		fmt.Sprintf("Средняя длительность сценария: %d мс.", metrics.AverageDurationMS),
		fmt.Sprintf("Ручных проверок: %d.", metrics.ManualCheckCount),
	}
	recommendations := []string{"Продолжить накопление записей для устойчивого ранжирования автоматизации."}
	if _, err := tx.Exec(ctx, `
		INSERT INTO analyst_reports (
			recording_id, analysis_run_id, summary, observations, recommendations,
			metrics, graph_summary, model, provider, prompt_version,
			normalization_version, grouping_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		job.RecordingID, job.AnalysisRunID, summary, domain.JSON(observations),
		domain.JSON(recommendations), metricsData,
		domain.JSON(map[string]int{"nodes": len(graph.Nodes), "edges": len(graph.Edges)}),
		extraction.Model, extraction.Provider, vision.PromptVersion,
		domain.NormalizationVersion, "scenario-grouping-v6"); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE analysis_jobs
		SET status = 'COMPLETED', progress = 100, completed_at = now(), updated_at = now()
		WHERE id = $1`, job.ID); err != nil {
		return err
	}
	inputTokens, outputTokens, thinkingTokens, totalTokens, estimatedCostUSD, pricingVersion :=
		runUsageValues(extraction)
	if _, err := tx.Exec(ctx, `
		UPDATE analysis_runs
		SET status = 'COMPLETED', provider = $2, model = $3, raw_text = $4,
		    input_tokens = $5, output_tokens = $6, thinking_tokens = $7,
		    total_tokens = $8, estimated_cost_usd = $9, pricing_version = $10,
		    completed_at = now(), updated_at = now()
		WHERE id = $1`,
		job.AnalysisRunID, extraction.Provider, extraction.Model, extraction.RawText,
		inputTokens, outputTokens, thinkingTokens, totalTokens, estimatedCostUSD, pricingVersion,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE recordings
		SET status = 'ANALYZED', duration_sec = $2, updated_at = now()
		WHERE id = $1`, job.RecordingID, metadata.DurationSec); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM scenario_templates t
		WHERE t.organization_id = $1
		  AND NOT EXISTS (SELECT 1 FROM scenario_instances i WHERE i.template_id = t.id)`,
		platform.LocalOrganizationID); err != nil {
		return err
	}
	return nil
}

func (s *Service) persistRunUsage(ctx context.Context, runID uuid.UUID, extraction vision.Result) error {
	if extraction.Usage == nil {
		return nil
	}
	inputTokens, outputTokens, thinkingTokens, totalTokens, estimatedCostUSD, pricingVersion :=
		runUsageValues(extraction)
	_, err := s.pool.Exec(ctx, `
		UPDATE analysis_runs
		SET provider = COALESCE(NULLIF($2, ''), provider),
		    model = COALESCE(NULLIF($3, ''), model),
		    raw_text = COALESCE(NULLIF($4, ''), raw_text),
		    input_tokens = $5,
		    output_tokens = $6,
		    thinking_tokens = $7,
		    total_tokens = $8,
		    estimated_cost_usd = $9,
		    pricing_version = $10,
		    updated_at = now()
		WHERE id = $1`,
		runID, extraction.Provider, extraction.Model, extraction.RawText,
		inputTokens, outputTokens, thinkingTokens, totalTokens, estimatedCostUSD, pricingVersion,
	)
	return err
}

func runUsageValues(extraction vision.Result) (any, any, any, any, any, any) {
	if extraction.Usage == nil {
		return nil, nil, nil, nil, nil, nil
	}
	var estimatedCostUSD, pricingVersion any
	if extraction.Cost != nil {
		estimatedCostUSD = extraction.Cost.USD
		pricingVersion = extraction.Cost.PricingVersion
	}
	return extraction.Usage.InputTokens,
		extraction.Usage.OutputTokens,
		extraction.Usage.ThinkingTokens,
		extraction.Usage.TotalTokens,
		estimatedCostUSD,
		pricingVersion
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
