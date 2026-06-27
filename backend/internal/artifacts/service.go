package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
)

var ErrNotFound = errors.New("artifact not found")

type Service struct {
	pool         *pgxpool.Pool
	geminiAPIKey string
	geminiModel  string
}

func New(pool *pgxpool.Pool, geminiAPIKey, geminiModel string) *Service {
	return &Service{pool: pool, geminiAPIKey: geminiAPIKey, geminiModel: geminiModel}
}

type ActionEvent struct {
	ID                uuid.UUID       `json:"id"`
	RecordingID       uuid.UUID       `json:"recordingId"`
	AnalysisRunID     *uuid.UUID      `json:"analysisRunId,omitempty"`
	TimestampMS       int             `json:"timestampMs"`
	CanonicalAction   string          `json:"canonicalAction"`
	EventType         string          `json:"eventType"`
	Screen            string          `json:"screen"`
	EntityType        *string         `json:"entityType,omitempty"`
	EntityID          *string         `json:"entityId,omitempty"`
	OrderID           *string         `json:"orderId,omitempty"`
	IssueType         *string         `json:"issueType,omitempty"`
	Target            string          `json:"target"`
	Confidence        float64         `json:"confidence"`
	SourceRawEventIDs json.RawMessage `json:"sourceRawEventIds"`
	QualityFlags      json.RawMessage `json:"qualityFlags"`
	Payload           json.RawMessage `json:"payload"`
	Source            string          `json:"source"`
	QAStatus          *string         `json:"qaStatus,omitempty"`
	QAComment         *string         `json:"qaComment,omitempty"`
	Version           int             `json:"version"`
}

type RawEvent struct {
	ID             uuid.UUID       `json:"id"`
	AnalysisRunID  uuid.UUID       `json:"analysisRunId"`
	TimestampMS    int             `json:"timestampMs"`
	Screen         string          `json:"screen"`
	VisibleText    *string         `json:"visibleText,omitempty"`
	Target         *string         `json:"target,omitempty"`
	EventTypeGuess string          `json:"eventTypeGuess"`
	ColorCues      json.RawMessage `json:"colorCues"`
	StateChange    *string         `json:"stateChange,omitempty"`
	Confidence     float64         `json:"confidence"`
	Payload        json.RawMessage `json:"payload"`
}

type QualityIssue struct {
	ID               uuid.UUID       `json:"id"`
	RecordingID      uuid.UUID       `json:"recordingId"`
	AnalysisRunID    *uuid.UUID      `json:"analysisRunId,omitempty"`
	RawVisionEventID *uuid.UUID      `json:"rawVisionEventId,omitempty"`
	ActionEventID    *uuid.UUID      `json:"actionEventId,omitempty"`
	Type             string          `json:"type"`
	Severity         string          `json:"severity"`
	Message          string          `json:"message"`
	TimestampMS      int             `json:"timestampMs"`
	Resolved         bool            `json:"resolved"`
	Payload          json.RawMessage `json:"payload"`
}

type ScenarioInstance struct {
	ID                  uuid.UUID       `json:"id"`
	RecordingID         uuid.UUID       `json:"recordingId"`
	AnalysisRunID       *uuid.UUID      `json:"analysisRunId,omitempty"`
	TemplateID          *uuid.UUID      `json:"templateId,omitempty"`
	KnownScenarioCode   *string         `json:"knownScenarioCode,omitempty"`
	OrderID             *string         `json:"orderId,omitempty"`
	EntityType          *string         `json:"entityType,omitempty"`
	EntityID            *string         `json:"entityId,omitempty"`
	IssueType           string          `json:"issueType"`
	StartedAtMS         int             `json:"startedAtMs"`
	EndedAtMS           int             `json:"endedAtMs"`
	DurationMS          int             `json:"durationMs"`
	EventIDs            json.RawMessage `json:"eventIds"`
	Outcome             string          `json:"outcome"`
	Status              string          `json:"status"`
	Confidence          float64         `json:"confidence"`
	BoundaryRuleVersion *string         `json:"boundaryRuleVersion,omitempty"`
	QualityFlags        json.RawMessage `json:"qualityFlags"`
}

type Template struct {
	ID                   uuid.UUID       `json:"id"`
	Code                 *string         `json:"code,omitempty"`
	Name                 string          `json:"name"`
	IssueType            string          `json:"issueType"`
	Signature            string          `json:"signature"`
	Frequency            int             `json:"frequency"`
	AverageDurationMS    int             `json:"averageDurationMs"`
	MedianDurationMS     int             `json:"medianDurationMs"`
	P95DurationMS        int             `json:"p95DurationMs"`
	ManualCheckCount     int             `json:"manualCheckCount"`
	RepeatedActionCount  int             `json:"repeatedActionCount"`
	ConfidenceAverage    float64         `json:"confidenceAverage"`
	AmbiguousCount       int             `json:"ambiguousCount"`
	AutomationScore      float64         `json:"automationScore"`
	ActionSequence       json.RawMessage `json:"actionSequence"`
	Metrics              json.RawMessage `json:"metrics"`
	Status               string          `json:"status"`
	AutomationCandidates []Candidate     `json:"automationCandidates"`
}

type Candidate struct {
	ID            uuid.UUID       `json:"id"`
	TemplateID    uuid.UUID       `json:"templateId"`
	Title         string          `json:"title"`
	Type          *string         `json:"type,omitempty"`
	Rationale     string          `json:"rationale"`
	AffectedSteps json.RawMessage `json:"affectedSteps"`
	Impact        string          `json:"impact"`
	Confidence    float64         `json:"confidence"`
	Score         float64         `json:"score"`
	Status        string          `json:"status"`
	Breakdown     json.RawMessage `json:"breakdown"`
}

type Report struct {
	ID                   uuid.UUID       `json:"id"`
	RecordingID          uuid.UUID       `json:"recordingId"`
	AnalysisRunID        *uuid.UUID      `json:"analysisRunId,omitempty"`
	Summary              string          `json:"summary"`
	Observations         json.RawMessage `json:"observations"`
	Recommendations      json.RawMessage `json:"recommendations"`
	Metrics              json.RawMessage `json:"metrics"`
	GraphSummary         json.RawMessage `json:"graphSummary"`
	Model                string          `json:"model"`
	Provider             *string         `json:"provider,omitempty"`
	PromptVersion        *string         `json:"promptVersion,omitempty"`
	NormalizationVersion *string         `json:"normalizationVersion,omitempty"`
	GroupingVersion      *string         `json:"groupingVersion,omitempty"`
}

func (s *Service) Events(ctx context.Context, recordingID uuid.UUID) ([]ActionEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, recording_id, analysis_run_id, timestamp_ms, canonical_action,
		       event_type, screen, entity_type, entity_id, order_id, issue_type,
		       target, confidence, source_raw_event_ids, quality_flags, payload,
		       source, qa_status, qa_comment, version
		FROM action_events
		WHERE recording_id = $1
		ORDER BY timestamp_ms, created_at`, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ActionEvent{}
	for rows.Next() {
		var item ActionEvent
		if err := rows.Scan(
			&item.ID, &item.RecordingID, &item.AnalysisRunID, &item.TimestampMS,
			&item.CanonicalAction, &item.EventType, &item.Screen, &item.EntityType,
			&item.EntityID, &item.OrderID, &item.IssueType, &item.Target,
			&item.Confidence, &item.SourceRawEventIDs, &item.QualityFlags,
			&item.Payload, &item.Source, &item.QAStatus, &item.QAComment, &item.Version,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) RawEvents(ctx context.Context, recordingID uuid.UUID) ([]RawEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.analysis_run_id, r.timestamp_ms, r.screen, r.visible_text,
		       r.target, r.event_type_guess, r.color_cues, r.state_change,
		       r.confidence, r.payload
		FROM raw_vision_events r
		JOIN analysis_runs a ON a.id = r.analysis_run_id
		WHERE a.recording_id = $1
		ORDER BY r.timestamp_ms, r.created_at`, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RawEvent{}
	for rows.Next() {
		var item RawEvent
		if err := rows.Scan(
			&item.ID, &item.AnalysisRunID, &item.TimestampMS, &item.Screen,
			&item.VisibleText, &item.Target, &item.EventTypeGuess, &item.ColorCues,
			&item.StateChange, &item.Confidence, &item.Payload,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Issues(ctx context.Context, recordingID uuid.UUID) ([]QualityIssue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, recording_id, analysis_run_id, raw_vision_event_id,
		       action_event_id, type, severity, message, timestamp_ms, resolved, payload
		FROM data_quality_issues
		WHERE recording_id = $1
		ORDER BY resolved, timestamp_ms, created_at`, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []QualityIssue{}
	for rows.Next() {
		var item QualityIssue
		if err := rows.Scan(
			&item.ID, &item.RecordingID, &item.AnalysisRunID, &item.RawVisionEventID,
			&item.ActionEventID, &item.Type, &item.Severity, &item.Message,
			&item.TimestampMS, &item.Resolved, &item.Payload,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Instances(ctx context.Context, recordingID *uuid.UUID) ([]ScenarioInstance, error) {
	query := `
		SELECT id, recording_id, analysis_run_id, template_id, known_scenario_code,
		       order_id, entity_type, entity_id, issue_type, started_at_ms,
		       ended_at_ms, duration_ms, event_ids, outcome, status, confidence,
		       boundary_rule_version, quality_flags
		FROM scenario_instances`
	args := []any{}
	if recordingID != nil {
		query += " WHERE recording_id = $1"
		args = append(args, *recordingID)
	}
	query += " ORDER BY started_at_ms, created_at"
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ScenarioInstance{}
	for rows.Next() {
		var item ScenarioInstance
		if err := rows.Scan(
			&item.ID, &item.RecordingID, &item.AnalysisRunID, &item.TemplateID,
			&item.KnownScenarioCode, &item.OrderID, &item.EntityType, &item.EntityID,
			&item.IssueType, &item.StartedAtMS, &item.EndedAtMS, &item.DurationMS,
			&item.EventIDs, &item.Outcome, &item.Status, &item.Confidence,
			&item.BoundaryRuleVersion, &item.QualityFlags,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Templates(ctx context.Context) ([]Template, error) {
	instances, events, err := s.projectScenarioData(ctx)
	if err != nil {
		return nil, err
	}
	domainInstances := make([]domain.ScenarioInstance, 0, len(instances))
	for _, item := range instances {
		domainInstances = append(domainInstances, toDomainInstance(item))
	}
	domainEvents := make([]domain.ActionEvent, 0, len(events))
	for _, item := range events {
		domainEvents = append(domainEvents, toDomainEvent(item))
	}
	return templatesFromDomain(domain.BuildScenarioGroups(domainInstances, domainEvents)), nil
}

func (s *Service) projectScenarioData(
	ctx context.Context,
) ([]ScenarioInstance, []ActionEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM recordings
		WHERE organization_id = $1`, platform.LocalOrganizationID)
	if err != nil {
		return nil, nil, err
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	instances := []ScenarioInstance{}
	events := []ActionEvent{}
	for _, id := range ids {
		recordingInstances, err := s.Instances(ctx, &id)
		if err != nil {
			return nil, nil, err
		}
		recordingEvents, err := s.Events(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		instances = append(instances, recordingInstances...)
		events = append(events, recordingEvents...)
	}
	return instances, events, nil
}

func templatesFromDomain(groups []domain.ScenarioTemplate) []Template {
	items := make([]Template, 0, len(groups))
	for _, group := range groups {
		item := Template{
			ID: group.ID, Name: group.Name, IssueType: group.IssueType,
			Signature: group.Signature, Frequency: group.Frequency,
			AverageDurationMS:    group.AverageDurationMS,
			MedianDurationMS:     group.MedianDurationMS,
			P95DurationMS:        group.P95DurationMS,
			ManualCheckCount:     group.ManualCheckCount,
			RepeatedActionCount:  group.RepeatedActionCount,
			ConfidenceAverage:    group.ConfidenceAverage,
			AmbiguousCount:       group.AmbiguousCount,
			AutomationScore:      group.AutomationScore,
			ActionSequence:       domain.JSON(group.ActionSequence),
			Metrics:              domain.JSON(group.Metrics),
			Status:               group.Status,
			AutomationCandidates: make([]Candidate, 0, len(group.AutomationCandidates)),
		}
		if group.Code != "" {
			code := group.Code
			item.Code = &code
		}
		for _, candidate := range group.AutomationCandidates {
			converted := Candidate{
				ID: candidate.ID, TemplateID: group.ID, Title: candidate.Title,
				Rationale:     candidate.Rationale,
				AffectedSteps: domain.JSON(candidate.AffectedSteps),
				Impact:        candidate.Impact, Confidence: candidate.Confidence,
				Score: candidate.Score, Status: candidate.Status,
				Breakdown: domain.JSON(candidate.Breakdown),
			}
			if candidate.Type != "" {
				candidateType := candidate.Type
				converted.Type = &candidateType
			}
			item.AutomationCandidates = append(item.AutomationCandidates, converted)
		}
		items = append(items, item)
	}
	return items
}

func toDomainEvent(item ActionEvent) domain.ActionEvent {
	var eventIDs []uuid.UUID
	var flags []string
	var payload map[string]any
	_ = json.Unmarshal(item.SourceRawEventIDs, &eventIDs)
	_ = json.Unmarshal(item.QualityFlags, &flags)
	_ = json.Unmarshal(item.Payload, &payload)
	return domain.ActionEvent{
		ID: item.ID, RecordingID: item.RecordingID,
		AnalysisRunID: valueUUID(item.AnalysisRunID, uuid.Nil),
		TimestampMS:   item.TimestampMS, CanonicalAction: item.CanonicalAction,
		EventType: item.EventType, Screen: item.Screen,
		EntityType: valueString(item.EntityType), EntityID: valueString(item.EntityID),
		OrderID: valueString(item.OrderID), IssueType: valueString(item.IssueType),
		Target: item.Target, Confidence: item.Confidence,
		SourceRawEventIDs: eventIDs, QualityFlags: flags, Payload: payload, Source: item.Source,
	}
}

func toDomainInstance(item ScenarioInstance) domain.ScenarioInstance {
	var eventIDs []uuid.UUID
	var flags []string
	_ = json.Unmarshal(item.EventIDs, &eventIDs)
	_ = json.Unmarshal(item.QualityFlags, &flags)
	return domain.ScenarioInstance{
		ID: item.ID, RecordingID: item.RecordingID,
		AnalysisRunID: valueUUID(item.AnalysisRunID, uuid.Nil),
		TemplateID:    item.TemplateID, KnownScenarioCode: valueString(item.KnownScenarioCode),
		OrderID: valueString(item.OrderID), EntityType: valueString(item.EntityType),
		EntityID: valueString(item.EntityID), IssueType: item.IssueType,
		StartedAtMS: item.StartedAtMS, EndedAtMS: item.EndedAtMS,
		DurationMS: item.DurationMS, EventIDs: eventIDs,
		Outcome: item.Outcome, Status: item.Status, Confidence: item.Confidence,
		BoundaryRuleVersion: valueString(item.BoundaryRuleVersion), QualityFlags: flags,
	}
}

func (s *Service) TemplatesForRecording(
	ctx context.Context,
	recordingID uuid.UUID,
) ([]Template, error) {
	return s.templates(ctx, &recordingID)
}

func (s *Service) templates(ctx context.Context, recordingID *uuid.UUID) ([]Template, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, code, name, issue_type, signature, frequency, average_duration_ms,
		       median_duration_ms, p95_duration_ms, manual_check_count,
		       repeated_action_count, confidence_average, ambiguous_count,
		       automation_score, action_sequence, metrics, status
		FROM scenario_templates
		WHERE organization_id = $1
		  AND (
		    $2::uuid IS NULL OR EXISTS (
		      SELECT 1
		      FROM scenario_instances i
		      WHERE i.template_id = scenario_templates.id
		        AND i.recording_id = $2
		    )
		  )
		ORDER BY frequency DESC, automation_score DESC`,
		platform.LocalOrganizationID, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Template{}
	byID := map[uuid.UUID]int{}
	for rows.Next() {
		var item Template
		if err := rows.Scan(
			&item.ID, &item.Code, &item.Name, &item.IssueType, &item.Signature,
			&item.Frequency, &item.AverageDurationMS, &item.MedianDurationMS,
			&item.P95DurationMS, &item.ManualCheckCount, &item.RepeatedActionCount,
			&item.ConfidenceAverage, &item.AmbiguousCount, &item.AutomationScore,
			&item.ActionSequence, &item.Metrics, &item.Status,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
		byID[item.ID] = len(items) - 1
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	candidateRows, err := s.pool.Query(ctx, `
		SELECT c.id, c.template_id, c.title, c.type, c.rationale, c.affected_steps,
		       c.impact, c.confidence, c.score, c.status, c.breakdown
		FROM automation_candidates c
		JOIN scenario_templates t ON t.id = c.template_id
		WHERE t.organization_id = $1
		  AND (
		    $2::uuid IS NULL OR EXISTS (
		      SELECT 1
		      FROM scenario_instances i
		      WHERE i.template_id = t.id
		        AND i.recording_id = $2
		    )
		  )
		ORDER BY c.score DESC`, platform.LocalOrganizationID, recordingID)
	if err != nil {
		return nil, err
	}
	defer candidateRows.Close()
	for candidateRows.Next() {
		var item Candidate
		if err := candidateRows.Scan(
			&item.ID, &item.TemplateID, &item.Title, &item.Type, &item.Rationale,
			&item.AffectedSteps, &item.Impact, &item.Confidence, &item.Score,
			&item.Status, &item.Breakdown,
		); err != nil {
			return nil, err
		}
		if index, ok := byID[item.TemplateID]; ok {
			items[index].AutomationCandidates = append(items[index].AutomationCandidates, item)
		}
	}
	return items, candidateRows.Err()
}

func (s *Service) Graph(ctx context.Context, recordingID uuid.UUID) (map[string]any, error) {
	var graph, metrics json.RawMessage
	err := s.pool.QueryRow(ctx, `
		SELECT graph, metrics
		FROM scenario_graphs
		WHERE recording_id = $1
		ORDER BY created_at DESC LIMIT 1`, recordingID).Scan(&graph, &metrics)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{"graph": domain.Graph{Nodes: []domain.GraphNode{}, Edges: []domain.GraphEdge{}}, "metrics": nil}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"graph": graph, "metrics": metrics}, nil
}

func (s *Service) Report(ctx context.Context, id uuid.UUID) (Report, error) {
	var item Report
	err := s.pool.QueryRow(ctx, `
		SELECT id, recording_id, analysis_run_id, summary, observations,
		       recommendations, metrics, graph_summary, model, provider,
		       prompt_version, normalization_version, grouping_version
		FROM analyst_reports
		WHERE id = $1`, id).Scan(
		&item.ID, &item.RecordingID, &item.AnalysisRunID, &item.Summary,
		&item.Observations, &item.Recommendations, &item.Metrics, &item.GraphSummary,
		&item.Model, &item.Provider, &item.PromptVersion, &item.NormalizationVersion,
		&item.GroupingVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	return item, err
}

func (s *Service) LatestReport(ctx context.Context, recordingID uuid.UUID) (*Report, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM analyst_reports WHERE recording_id = $1
		ORDER BY created_at DESC LIMIT 1`, recordingID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item, err := s.Report(ctx, id)
	return &item, err
}

func (s *Service) Bundle(ctx context.Context, recordingID uuid.UUID) (map[string]any, error) {
	events, err := s.Events(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	rawEvents, err := s.RawEvents(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	issues, err := s.Issues(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	instances, err := s.Instances(ctx, &recordingID)
	if err != nil {
		return nil, err
	}
	templates, err := s.TemplatesForRecording(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	graph, err := s.Graph(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	report, err := s.LatestReport(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	groundTruth, err := s.GroundTruth(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"rawEvents": rawEvents, "events": events, "dataQualityIssues": issues,
		"groundTruth": groundTruth,
		"scenarios":   map[string]any{"templates": templates, "instances": instances},
		"graph":       graph["graph"], "metrics": graph["metrics"], "report": report,
	}, nil
}

type EventPatch struct {
	CanonicalAction *string          `json:"canonicalAction"`
	EventType       *string          `json:"eventType"`
	Screen          *string          `json:"screen"`
	EntityType      *string          `json:"entityType"`
	EntityID        *string          `json:"entityId"`
	OrderID         *string          `json:"orderId"`
	IssueType       *string          `json:"issueType"`
	Target          *string          `json:"target"`
	Confidence      *float64         `json:"confidence"`
	QualityFlags    *json.RawMessage `json:"qualityFlags"`
	QAStatus        *string          `json:"qaStatus"`
	QAComment       *string          `json:"qaComment"`
	SaveGroundTruth bool             `json:"saveGroundTruth"`
}

func (s *Service) PatchEvent(
	ctx context.Context,
	recordingID, eventID uuid.UUID,
	patch EventPatch,
) (ActionEvent, error) {
	if patch.Confidence != nil && (*patch.Confidence < 0 || *patch.Confidence > 1) {
		return ActionEvent{}, fmt.Errorf("confidence must be between 0 and 1")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ActionEvent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var item ActionEvent
	err = tx.QueryRow(ctx, `
		UPDATE action_events SET
			canonical_action = COALESCE($3, canonical_action),
			event_type = COALESCE($4, event_type),
			screen = COALESCE($5, screen),
			entity_type = CASE WHEN $6::text IS NULL THEN entity_type ELSE NULLIF($6, '') END,
			entity_id = CASE WHEN $7::text IS NULL THEN entity_id ELSE NULLIF($7, '') END,
			order_id = CASE WHEN $8::text IS NULL THEN order_id ELSE NULLIF($8, '') END,
			issue_type = CASE WHEN $9::text IS NULL THEN issue_type ELSE NULLIF($9, '') END,
			target = COALESCE($10, target),
			confidence = COALESCE($11, confidence),
			quality_flags = COALESCE($12, quality_flags),
			qa_status = CASE WHEN $13::text IS NULL THEN qa_status ELSE NULLIF($13, '') END,
			qa_comment = CASE WHEN $14::text IS NULL THEN qa_comment ELSE NULLIF($14, '') END,
			version = version + 1, updated_at = now()
		WHERE recording_id = $1 AND id = $2
		RETURNING id, recording_id, analysis_run_id, timestamp_ms, canonical_action,
		          event_type, screen, entity_type, entity_id, order_id, issue_type,
		          target, confidence, source_raw_event_ids, quality_flags, payload,
		          source, qa_status, qa_comment, version`,
		recordingID, eventID, patch.CanonicalAction, patch.EventType, patch.Screen,
		patch.EntityType, patch.EntityID, patch.OrderID, patch.IssueType, patch.Target,
		patch.Confidence, patch.QualityFlags, patch.QAStatus, patch.QAComment,
	).Scan(
		&item.ID, &item.RecordingID, &item.AnalysisRunID, &item.TimestampMS,
		&item.CanonicalAction, &item.EventType, &item.Screen, &item.EntityType,
		&item.EntityID, &item.OrderID, &item.IssueType, &item.Target,
		&item.Confidence, &item.SourceRawEventIDs, &item.QualityFlags,
		&item.Payload, &item.Source, &item.QAStatus, &item.QAComment, &item.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActionEvent{}, ErrNotFound
	}
	if err != nil {
		return ActionEvent{}, err
	}
	if patch.SaveGroundTruth {
		payload, marshalErr := json.Marshal(map[string]any{
			"sourceActionEventId": item.ID,
			"analysisRunId":       item.AnalysisRunID,
			"timestampMs":         item.TimestampMS,
			"canonicalAction":     item.CanonicalAction,
			"eventType":           item.EventType,
			"screen":              item.Screen,
			"entityType":          item.EntityType,
			"entityId":            item.EntityID,
			"orderId":             item.OrderID,
			"issueType":           item.IssueType,
			"target":              item.Target,
			"confidence":          item.Confidence,
			"qualityFlags":        item.QualityFlags,
			"qaStatus":            item.QAStatus,
			"qaComment":           item.QAComment,
			"source":              "ground-truth",
		})
		if marshalErr != nil {
			return ActionEvent{}, marshalErr
		}
		if _, err = tx.Exec(ctx, `
			DELETE FROM ground_truth_events
			WHERE organization_id = $1 AND recording_id = $2
			  AND payload->>'sourceActionEventId' = $3`,
			platform.LocalOrganizationID, recordingID, item.ID.String()); err != nil {
			return ActionEvent{}, err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO ground_truth_events (
				organization_id, recording_id, timestamp_ms, payload
			) VALUES ($1,$2,$3,$4)`,
			platform.LocalOrganizationID, recordingID, item.TimestampMS, payload); err != nil {
			return ActionEvent{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ActionEvent{}, err
	}
	return item, nil
}

func (s *Service) GroundTruth(ctx context.Context, recordingID uuid.UUID) ([]json.RawMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT payload
		FROM ground_truth_events
		WHERE organization_id = $1 AND recording_id = $2
		ORDER BY timestamp_ms, created_at`,
		platform.LocalOrganizationID, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var item json.RawMessage
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type IssuePatch struct {
	Resolved *bool   `json:"resolved"`
	Severity *string `json:"severity"`
	Message  *string `json:"message"`
}

func (s *Service) PatchIssue(
	ctx context.Context,
	recordingID, issueID uuid.UUID,
	patch IssuePatch,
) (QualityIssue, error) {
	var item QualityIssue
	err := s.pool.QueryRow(ctx, `
		UPDATE data_quality_issues SET
			resolved = COALESCE($3, resolved),
			severity = COALESCE($4, severity),
			message = COALESCE($5, message),
			updated_at = now()
		WHERE recording_id = $1 AND id = $2
		RETURNING id, recording_id, analysis_run_id, raw_vision_event_id,
		          action_event_id, type, severity, message, timestamp_ms, resolved, payload`,
		recordingID, issueID, patch.Resolved, patch.Severity, patch.Message,
	).Scan(
		&item.ID, &item.RecordingID, &item.AnalysisRunID, &item.RawVisionEventID,
		&item.ActionEventID, &item.Type, &item.Severity, &item.Message,
		&item.TimestampMS, &item.Resolved, &item.Payload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualityIssue{}, ErrNotFound
	}
	return item, err
}

func (s *Service) CompleteQA(ctx context.Context, recordingID uuid.UUID, issueIDs []uuid.UUID) error {
	if len(issueIDs) == 0 {
		_, err := s.pool.Exec(ctx, `
			UPDATE data_quality_issues SET resolved = true, updated_at = now()
			WHERE recording_id = $1`, recordingID)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE data_quality_issues SET resolved = true, updated_at = now()
		WHERE recording_id = $1 AND id = ANY($2)`, recordingID, issueIDs)
	return err
}

func (s *Service) ImportGroundTruth(
	ctx context.Context,
	recordingID *uuid.UUID,
	events []json.RawMessage,
) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, event := range events {
		var envelope struct {
			TimestampMS int `json:"timestampMs"`
		}
		if err := json.Unmarshal(event, &envelope); err != nil || envelope.TimestampMS < 0 {
			return 0, fmt.Errorf("each event requires non-negative timestampMs")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ground_truth_events (
				organization_id, recording_id, timestamp_ms, payload
			) VALUES ($1,$2,$3,$4)`,
			platform.LocalOrganizationID, recordingID, envelope.TimestampMS, event); err != nil {
			return 0, err
		}
	}
	return len(events), tx.Commit(ctx)
}

func (s *Service) ProjectAnalysis(ctx context.Context) (map[string]any, error) {
	eventsRows, err := s.pool.Query(ctx, `SELECT id FROM recordings WHERE organization_id = $1`, platform.LocalOrganizationID)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for eventsRows.Next() {
		var id uuid.UUID
		if err := eventsRows.Scan(&id); err != nil {
			eventsRows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	eventsRows.Close()
	allEvents := []ActionEvent{}
	allRawEvents := []RawEvent{}
	allIssues := []QualityIssue{}
	for _, id := range ids {
		events, err := s.Events(ctx, id)
		if err != nil {
			return nil, err
		}
		issues, err := s.Issues(ctx, id)
		if err != nil {
			return nil, err
		}
		rawEvents, err := s.RawEvents(ctx, id)
		if err != nil {
			return nil, err
		}
		allEvents = append(allEvents, events...)
		allRawEvents = append(allRawEvents, rawEvents...)
		allIssues = append(allIssues, issues...)
	}
	instances, err := s.Instances(ctx, nil)
	if err != nil {
		return nil, err
	}
	runsRows, err := s.pool.Query(ctx, `
		SELECT id, recording_id, status::text, provider, model, prompt_version,
		       normalization_version, grouping_version, raw_text, error,
		       created_at, started_at, completed_at
		FROM analysis_runs
		WHERE organization_id = $1
		ORDER BY created_at DESC`, platform.LocalOrganizationID)
	if err != nil {
		return nil, err
	}
	defer runsRows.Close()
	runs := []map[string]any{}
	for runsRows.Next() {
		var (
			id, recordingID                       uuid.UUID
			status, provider, promptVersion       string
			normalizationVersion, groupingVersion string
			model, rawText, runError              *string
			createdAt                             time.Time
			startedAt, completedAt                *time.Time
		)
		if err := runsRows.Scan(
			&id, &recordingID, &status, &provider, &model, &promptVersion,
			&normalizationVersion, &groupingVersion, &rawText, &runError,
			&createdAt, &startedAt, &completedAt,
		); err != nil {
			return nil, err
		}
		runs = append(runs, map[string]any{
			"id": id, "recordingId": recordingID, "status": status,
			"provider": provider, "model": model, "promptVersion": promptVersion,
			"normalizationVersion": normalizationVersion, "groupingVersion": groupingVersion,
			"rawText": rawText, "error": runError, "createdAt": createdAt,
			"startedAt": startedAt, "completedAt": completedAt,
		})
	}
	if err := runsRows.Err(); err != nil {
		return nil, err
	}
	domainEvents := make([]domain.ActionEvent, 0, len(allEvents))
	for _, item := range allEvents {
		var eventIDs []uuid.UUID
		var flags []string
		var payload map[string]any
		_ = json.Unmarshal(item.SourceRawEventIDs, &eventIDs)
		_ = json.Unmarshal(item.QualityFlags, &flags)
		_ = json.Unmarshal(item.Payload, &payload)
		domainEvents = append(domainEvents, domain.ActionEvent{
			ID: item.ID, RecordingID: item.RecordingID,
			AnalysisRunID: valueUUID(item.AnalysisRunID, uuid.Nil),
			TimestampMS:   item.TimestampMS, CanonicalAction: item.CanonicalAction,
			EventType: item.EventType, Screen: item.Screen,
			EntityType: valueString(item.EntityType), EntityID: valueString(item.EntityID),
			OrderID: valueString(item.OrderID), IssueType: valueString(item.IssueType),
			Target: item.Target, Confidence: item.Confidence,
			SourceRawEventIDs: eventIDs, QualityFlags: flags, Payload: payload, Source: item.Source,
		})
	}
	domainInstances := make([]domain.ScenarioInstance, 0, len(instances))
	for _, item := range instances {
		var eventIDs []uuid.UUID
		var flags []string
		_ = json.Unmarshal(item.EventIDs, &eventIDs)
		_ = json.Unmarshal(item.QualityFlags, &flags)
		domainInstances = append(domainInstances, domain.ScenarioInstance{
			ID: item.ID, RecordingID: item.RecordingID,
			AnalysisRunID: valueUUID(item.AnalysisRunID, uuid.Nil),
			TemplateID:    item.TemplateID, KnownScenarioCode: valueString(item.KnownScenarioCode),
			OrderID: valueString(item.OrderID), EntityType: valueString(item.EntityType),
			EntityID: valueString(item.EntityID), IssueType: item.IssueType,
			StartedAtMS: item.StartedAtMS, EndedAtMS: item.EndedAtMS,
			DurationMS: item.DurationMS, EventIDs: eventIDs,
			Outcome: item.Outcome, Status: item.Status, Confidence: item.Confidence,
			BoundaryRuleVersion: valueString(item.BoundaryRuleVersion), QualityFlags: flags,
		})
	}
	projectTemplates := domain.BuildProjectScenarioGroups(domainInstances, domainEvents)
	return map[string]any{
		"runs": runs, "rawEvents": allRawEvents, "events": allEvents,
		"dataQualityIssues": allIssues,
		"scenarios":         map[string]any{"templates": projectTemplates, "instances": instances},
		"graph":             domain.BuildGraph(domainInstances, domainEvents),
		"metrics":           domain.CalculateMetrics(domainInstances, domainEvents),
		"report":            nil,
		"scope":             "project",
	}, nil
}

func CSV(rows []map[string]any) string {
	if len(rows) == 0 {
		return ""
	}
	headersSet := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			headersSet[key] = true
		}
	}
	headers := make([]string, 0, len(headersSet))
	for key := range headersSet {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	lines := []string{strings.Join(headers, ",")}
	for _, row := range rows {
		values := make([]string, len(headers))
		for index, header := range headers {
			value := row[header]
			if value == nil {
				continue
			}
			var text string
			switch current := value.(type) {
			case string:
				text = current
			default:
				data, _ := json.Marshal(current)
				text = string(data)
			}
			values[index] = `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
		}
		lines = append(lines, strings.Join(values, ","))
	}
	return strings.Join(lines, "\n")
}
