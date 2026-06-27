package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
	"github.com/example/dispscenario-analyst-v2/internal/scenario"
)

type runMetadata struct {
	ID                   uuid.UUID
	Provider             string
	Model                *string
	PromptVersion        string
	NormalizationVersion string
	GroupingVersion      string
}

func (s *Service) Rebuild(ctx context.Context, recordingID uuid.UUID) (map[string]any, error) {
	run, err := s.latestRun(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	storedEvents, err := s.Events(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	events := make([]domain.ActionEvent, 0, len(storedEvents))
	for _, item := range storedEvents {
		if item.QAStatus != nil && *item.QAStatus == "ignored" {
			continue
		}
		var rawIDs []uuid.UUID
		var flags []string
		var payload map[string]any
		_ = json.Unmarshal(item.SourceRawEventIDs, &rawIDs)
		_ = json.Unmarshal(item.QualityFlags, &flags)
		_ = json.Unmarshal(item.Payload, &payload)
		events = append(events, domain.ActionEvent{
			ID: item.ID, RecordingID: item.RecordingID,
			AnalysisRunID: valueUUID(item.AnalysisRunID, run.ID),
			TimestampMS:   item.TimestampMS, CanonicalAction: item.CanonicalAction,
			EventType: item.EventType, Screen: item.Screen,
			EntityType: valueString(item.EntityType), EntityID: valueString(item.EntityID),
			OrderID: valueString(item.OrderID), IssueType: valueString(item.IssueType),
			Target: item.Target, Confidence: item.Confidence,
			SourceRawEventIDs: rawIDs, QualityFlags: flags, Payload: payload, Source: item.Source,
		})
	}
	newIssues := []domain.DataQualityIssue{}
	scenarioConfig, err := scenario.LoadConfig(ctx, s.pool)
	if err != nil {
		return nil, err
	}
	instances := domain.BuildScenarioInstancesWithConfig(
		recordingID, events, &newIssues, scenarioConfig,
	)
	groups := domain.BuildScenarioGroups(instances, events)
	metrics := domain.CalculateMetrics(instances, events)
	graph := domain.BuildGraph(instances, events)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		DELETE FROM data_quality_issues
		WHERE recording_id = $1 AND type = 'MISSING_SCENARIO_END'`, recordingID); err != nil {
		return nil, err
	}
	for _, statement := range []string{
		"DELETE FROM scenario_instances WHERE recording_id = $1",
		"DELETE FROM scenario_graphs WHERE recording_id = $1",
		"DELETE FROM analyst_reports WHERE recording_id = $1",
	} {
		if _, err := tx.Exec(ctx, statement, recordingID); err != nil {
			return nil, err
		}
	}
	templateByInstance, err := persistGroups(ctx, tx, groups)
	if err != nil {
		return nil, err
	}
	if err := persistInstances(ctx, tx, instances, templateByInstance); err != nil {
		return nil, err
	}
	for _, issue := range newIssues {
		if _, err := tx.Exec(ctx, `
			INSERT INTO data_quality_issues (
				id, recording_id, analysis_run_id, raw_vision_event_id, action_event_id,
				type, severity, message, timestamp_ms, resolved, payload
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			issue.ID, recordingID, run.ID, issue.RawVisionEventID, issue.ActionEventID,
			issue.Type, issue.Severity, issue.Message, issue.TimestampMS,
			issue.Resolved, domain.JSON(issue.Payload)); err != nil {
			return nil, err
		}
	}
	graphData, _ := json.Marshal(graph)
	metricsData, _ := json.Marshal(metrics)
	if _, err := tx.Exec(ctx, `
		INSERT INTO scenario_graphs (recording_id, analysis_run_id, graph, metrics)
		VALUES ($1,$2,$3,$4)`, recordingID, run.ID, graphData, metricsData); err != nil {
		return nil, err
	}
	model := "unknown"
	if run.Model != nil {
		model = *run.Model
	}
	summary := fmt.Sprintf(
		"Построено %d экземпляров и %d групп сценариев после QA rebuild.",
		len(instances), len(groups),
	)
	if _, err := tx.Exec(ctx, `
		INSERT INTO analyst_reports (
			recording_id, analysis_run_id, summary, observations, recommendations,
			metrics, graph_summary, model, provider, prompt_version,
			normalization_version, grouping_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		recordingID, run.ID, summary,
		domain.JSON([]string{fmt.Sprintf("Ручных проверок: %d.", metrics.ManualCheckCount)}),
		domain.JSON([]string{"Проверить неразрешённые quality issues."}),
		metricsData, domain.JSON(map[string]int{"nodes": len(graph.Nodes), "edges": len(graph.Edges)}),
		model, run.Provider, run.PromptVersion, run.NormalizationVersion, run.GroupingVersion,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM scenario_templates t
		WHERE t.organization_id = $1
		  AND NOT EXISTS (SELECT 1 FROM scenario_instances i WHERE i.template_id = t.id)`,
		platform.LocalOrganizationID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Bundle(ctx, recordingID)
}

func (s *Service) Renormalize(ctx context.Context, recordingID uuid.UUID) (map[string]any, error) {
	run, err := s.latestRun(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	stored, err := s.RawEvents(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, ErrNotFound
	}
	rawEvents := make([]domain.RawVisionEvent, 0, len(stored))
	for _, item := range stored {
		var cues []string
		var payload map[string]any
		_ = json.Unmarshal(item.ColorCues, &cues)
		_ = json.Unmarshal(item.Payload, &payload)
		rawEvents = append(rawEvents, domain.RawVisionEvent{
			ID: item.ID, AnalysisRunID: item.AnalysisRunID,
			TimestampMS: item.TimestampMS, Screen: item.Screen,
			VisibleText: valueString(item.VisibleText), Target: valueString(item.Target),
			EventTypeGuess: item.EventTypeGuess, ColorCues: cues,
			StateChange: valueString(item.StateChange), Confidence: item.Confidence, Payload: payload,
		})
	}
	normalized := domain.Normalize(recordingID, run.ID, rawEvents)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, statement := range []string{
		"DELETE FROM data_quality_issues WHERE recording_id = $1",
		"DELETE FROM scenario_instances WHERE recording_id = $1",
		"DELETE FROM scenario_graphs WHERE recording_id = $1",
		"DELETE FROM analyst_reports WHERE recording_id = $1",
		"DELETE FROM action_events WHERE recording_id = $1",
	} {
		if _, err := tx.Exec(ctx, statement, recordingID); err != nil {
			return nil, err
		}
	}
	for _, event := range normalized.ActionEvents {
		if _, err := tx.Exec(ctx, `
			INSERT INTO action_events (
				id, recording_id, analysis_run_id, timestamp_ms, canonical_action,
				event_type, screen, entity_type, entity_id, order_id, issue_type,
				target, confidence, source_raw_event_ids, quality_flags, payload, source
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			event.ID, recordingID, run.ID, event.TimestampMS, event.CanonicalAction,
			event.EventType, event.Screen, nullString(event.EntityType), nullString(event.EntityID),
			nullString(event.OrderID), nullString(event.IssueType), event.Target, event.Confidence,
			domain.JSON(event.SourceRawEventIDs), domain.JSON(event.QualityFlags),
			domain.JSON(event.Payload), event.Source); err != nil {
			return nil, err
		}
	}
	for _, issue := range normalized.DataQualityIssues {
		if _, err := tx.Exec(ctx, `
			INSERT INTO data_quality_issues (
				id, recording_id, analysis_run_id, raw_vision_event_id, action_event_id,
				type, severity, message, timestamp_ms, resolved, payload
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			issue.ID, recordingID, run.ID, issue.RawVisionEventID, issue.ActionEventID,
			issue.Type, issue.Severity, issue.Message, issue.TimestampMS,
			issue.Resolved, domain.JSON(issue.Payload)); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE analysis_runs
		SET normalization_version = $2, updated_at = now()
		WHERE id = $1`, run.ID, domain.NormalizationVersion); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Rebuild(ctx, recordingID)
}

func (s *Service) AddBoundaryReviewIssue(ctx context.Context, recordingID uuid.UUID) (map[string]any, error) {
	run, err := s.latestRun(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	review, reviewErr := s.reviewBoundaries(ctx, recordingID)
	if reviewErr != nil {
		review = boundaryReview{
			Warnings: []boundaryWarning{{
				TimestampMS: 0, Severity: "warning",
				Message: "Проверка границ Gemini недоступна: " + reviewErr.Error(),
			}},
		}
	}
	events, err := s.Events(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	instances, err := s.Instances(ctx, &recordingID)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		DELETE FROM data_quality_issues
		WHERE recording_id=$1 AND type='GEMINI_BOUNDARY_REVIEW'`, recordingID); err != nil {
		return nil, err
	}
	for _, warning := range review.Warnings {
		if shouldSuppressBoundaryWarning(warning, events, instances) {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO data_quality_issues (
				recording_id, analysis_run_id, type, severity, message,
				timestamp_ms, resolved, payload
			) VALUES ($1,$2,'GEMINI_BOUNDARY_REVIEW',$3,$4,$5,false,$6)`,
			recordingID, run.ID, warning.Severity, warning.Message, warning.TimestampMS,
			domain.JSON(map[string]any{
				"source": "gemini-boundary-review", "provider": review.Provider,
				"model": review.Model,
			})); err != nil {
			return nil, err
		}
	}
	for _, proposal := range review.Proposals {
		if _, err := tx.Exec(ctx, `
			INSERT INTO data_quality_issues (
				recording_id, analysis_run_id, type, severity, message,
				timestamp_ms, resolved, payload
			) VALUES ($1,$2,'GEMINI_BOUNDARY_REVIEW',$3,$4,$5,false,$6)`,
			recordingID, run.ID, severityForConfidence(proposal.Confidence),
			"Gemini предлагает проверить границы сценария.", proposal.StartTimestampMS,
			domain.JSON(map[string]any{
				"source": "gemini-boundary-review", "provider": review.Provider,
				"model": review.Model, "proposal": proposal,
			})); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Rebuild(ctx, recordingID)
}

func severityForConfidence(confidence float64) string {
	if confidence >= .75 {
		return "warning"
	}
	return "info"
}

func (s *Service) latestRun(ctx context.Context, recordingID uuid.UUID) (runMetadata, error) {
	var run runMetadata
	err := s.pool.QueryRow(ctx, `
		SELECT id, provider, model, prompt_version, normalization_version, grouping_version
		FROM analysis_runs
		WHERE recording_id = $1
		ORDER BY created_at DESC LIMIT 1`, recordingID).Scan(
		&run.ID, &run.Provider, &run.Model, &run.PromptVersion,
		&run.NormalizationVersion, &run.GroupingVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return runMetadata{}, ErrNotFound
	}
	return run, err
}

func persistGroups(
	ctx context.Context,
	tx pgx.Tx,
	groups []domain.ScenarioTemplate,
) (map[uuid.UUID]uuid.UUID, error) {
	templateByInstance := map[uuid.UUID]uuid.UUID{}
	for _, group := range groups {
		var persistedTemplateID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO scenario_templates (
				id, organization_id, code, name, issue_type, signature, frequency,
				average_duration_ms, median_duration_ms, p95_duration_ms,
				manual_check_count, repeated_action_count, confidence_average,
				ambiguous_count, automation_score, action_sequence, metrics, status, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,now())
			ON CONFLICT (organization_id, signature) DO UPDATE SET
				code=EXCLUDED.code, name=EXCLUDED.name, issue_type=EXCLUDED.issue_type,
				frequency=EXCLUDED.frequency, average_duration_ms=EXCLUDED.average_duration_ms,
				median_duration_ms=EXCLUDED.median_duration_ms, p95_duration_ms=EXCLUDED.p95_duration_ms,
				manual_check_count=EXCLUDED.manual_check_count,
				repeated_action_count=EXCLUDED.repeated_action_count,
				confidence_average=EXCLUDED.confidence_average,
				ambiguous_count=EXCLUDED.ambiguous_count,
				automation_score=EXCLUDED.automation_score,
				action_sequence=EXCLUDED.action_sequence, metrics=EXCLUDED.metrics,
				status=EXCLUDED.status, updated_at=now()
			RETURNING id`,
			group.ID, platform.LocalOrganizationID, group.Code, group.Name, group.IssueType,
			group.Signature, group.Frequency, group.AverageDurationMS, group.MedianDurationMS,
			group.P95DurationMS, group.ManualCheckCount, group.RepeatedActionCount,
			group.ConfidenceAverage, group.AmbiguousCount, group.AutomationScore,
			domain.JSON(group.ActionSequence), domain.JSON(group.Metrics), group.Status,
		).Scan(&persistedTemplateID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM automation_candidates WHERE template_id = $1", persistedTemplateID); err != nil {
			return nil, err
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
				return nil, err
			}
		}
		for _, instanceID := range group.InstanceIDs {
			templateByInstance[instanceID] = persistedTemplateID
		}
	}
	return templateByInstance, nil
}

func persistInstances(
	ctx context.Context,
	tx pgx.Tx,
	instances []domain.ScenarioInstance,
	templateByInstance map[uuid.UUID]uuid.UUID,
) error {
	for _, instance := range instances {
		if _, err := tx.Exec(ctx, `
			INSERT INTO scenario_instances (
				id, recording_id, analysis_run_id, template_id, known_scenario_code,
				order_id, entity_type, entity_id, issue_type, started_at_ms,
				ended_at_ms, duration_ms, event_ids, outcome, status, confidence,
				boundary_rule_version, quality_flags
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			instance.ID, instance.RecordingID, instance.AnalysisRunID,
			templateByInstance[instance.ID], nullString(instance.KnownScenarioCode),
			nullString(instance.OrderID), nullString(instance.EntityType), nullString(instance.EntityID),
			instance.IssueType, instance.StartedAtMS, instance.EndedAtMS, instance.DurationMS,
			domain.JSON(instance.EventIDs), instance.Outcome, instance.Status, instance.Confidence,
			instance.BoundaryRuleVersion, domain.JSON(instance.QualityFlags)); err != nil {
			return err
		}
	}
	return nil
}

func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func valueUUID(value *uuid.UUID, fallback uuid.UUID) uuid.UUID {
	if value == nil {
		return fallback
	}
	return *value
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
