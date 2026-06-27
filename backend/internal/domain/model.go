package domain

import (
	"encoding/json"

	"github.com/google/uuid"
)

type RawVisionEvent struct {
	ID             uuid.UUID      `json:"id"`
	AnalysisRunID  uuid.UUID      `json:"analysisRunId"`
	TimestampMS    int            `json:"timestampMs"`
	Screen         string         `json:"screen"`
	VisibleText    string         `json:"visibleText"`
	Target         string         `json:"target"`
	EventTypeGuess string         `json:"eventTypeGuess"`
	ColorCues      []string       `json:"colorCues"`
	StateChange    string         `json:"stateChange"`
	Confidence     float64        `json:"confidence"`
	Payload        map[string]any `json:"payload"`
}

type ActionEvent struct {
	ID                uuid.UUID      `json:"id"`
	RecordingID       uuid.UUID      `json:"recordingId"`
	AnalysisRunID     uuid.UUID      `json:"analysisRunId"`
	TimestampMS       int            `json:"timestampMs"`
	CanonicalAction   string         `json:"canonicalAction"`
	EventType         string         `json:"eventType"`
	Screen            string         `json:"screen"`
	EntityType        string         `json:"entityType,omitempty"`
	EntityID          string         `json:"entityId,omitempty"`
	OrderID           string         `json:"orderId,omitempty"`
	IssueType         string         `json:"issueType,omitempty"`
	Target            string         `json:"target"`
	VisibleText       string         `json:"visibleText,omitempty"`
	StateChange       string         `json:"stateChange,omitempty"`
	Confidence        float64        `json:"confidence"`
	SourceRawEventIDs []uuid.UUID    `json:"sourceRawEventIds"`
	QualityFlags      []string       `json:"qualityFlags"`
	Payload           map[string]any `json:"payload"`
	Source            string         `json:"source"`
}

type DataQualityIssue struct {
	ID               uuid.UUID      `json:"id"`
	RecordingID      uuid.UUID      `json:"recordingId"`
	AnalysisRunID    uuid.UUID      `json:"analysisRunId"`
	RawVisionEventID *uuid.UUID     `json:"rawVisionEventId,omitempty"`
	ActionEventID    *uuid.UUID     `json:"actionEventId,omitempty"`
	Type             string         `json:"type"`
	Severity         string         `json:"severity"`
	Message          string         `json:"message"`
	TimestampMS      int            `json:"timestampMs"`
	Resolved         bool           `json:"resolved"`
	Payload          map[string]any `json:"payload"`
}

type ScenarioInstance struct {
	ID                  uuid.UUID   `json:"id"`
	RecordingID         uuid.UUID   `json:"recordingId"`
	AnalysisRunID       uuid.UUID   `json:"analysisRunId"`
	TemplateID          *uuid.UUID  `json:"templateId,omitempty"`
	KnownScenarioCode   string      `json:"knownScenarioCode,omitempty"`
	OrderID             string      `json:"orderId,omitempty"`
	EntityType          string      `json:"entityType,omitempty"`
	EntityID            string      `json:"entityId,omitempty"`
	IssueType           string      `json:"issueType"`
	StartedAtMS         int         `json:"startedAtMs"`
	EndedAtMS           int         `json:"endedAtMs"`
	DurationMS          int         `json:"durationMs"`
	EventIDs            []uuid.UUID `json:"eventIds"`
	Outcome             string      `json:"outcome"`
	Status              string      `json:"status"`
	Confidence          float64     `json:"confidence"`
	BoundaryRuleVersion string      `json:"boundaryRuleVersion"`
	QualityFlags        []string    `json:"qualityFlags"`
}

type ScenarioMetrics struct {
	AverageDurationMS               int            `json:"averageDurationMs"`
	MedianDurationMS                int            `json:"medianDurationMs"`
	P95DurationMS                   int            `json:"p95DurationMs"`
	ActionFrequency                 map[string]int `json:"actionFrequency"`
	ActionStats                     []ActionStat   `json:"actionStats"`
	PairStats                       []PairStat     `json:"pairStats"`
	RepeatedActions                 map[string]int `json:"repeatedActions"`
	RepeatedActionCount             int            `json:"repeatedActionCount"`
	ManualCheckCount                int            `json:"manualCheckCount"`
	ManualActionCount               int            `json:"manualActionCount"`
	AverageManualChecksPerInstance  float64        `json:"averageManualChecksPerInstance"`
	AverageManualActionsPerInstance float64        `json:"averageManualActionsPerInstance"`
	ConfidenceAverage               float64        `json:"confidenceAverage"`
	AmbiguousCount                  int            `json:"ambiguousCount"`
	InterruptedCount                int            `json:"interruptedCount"`
}

type ActionStat struct {
	Action            string   `json:"action"`
	Count             int      `json:"count"`
	InstanceCount     int      `json:"instanceCount"`
	RepeatedCount     int      `json:"repeatedCount"`
	AverageDurationMS int      `json:"averageDurationMs"`
	ConfidenceAverage float64  `json:"confidenceAverage"`
	IsManualCheck     bool     `json:"isManualCheck"`
	IsManualWork      bool     `json:"isManualWork"`
	Examples          []string `json:"examples"`
}

type PairStat struct {
	Key               string   `json:"key"`
	Steps             []string `json:"steps"`
	Count             int      `json:"count"`
	InstanceCount     int      `json:"instanceCount"`
	AverageDurationMS int      `json:"averageDurationMs"`
	ConfidenceAverage float64  `json:"confidenceAverage"`
}

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID                 string         `json:"id"`
	Label              string         `json:"label"`
	Type               string         `json:"type"`
	Severity           string         `json:"severity"`
	Frequency          int            `json:"frequency"`
	IssueTypes         []string       `json:"issueTypes"`
	Confidence         float64        `json:"confidence"`
	Examples           []GraphExample `json:"examples"`
	RelatedScenarioIDs []uuid.UUID    `json:"relatedScenarioIds"`
}

type GraphExample struct {
	ScenarioInstanceID uuid.UUID `json:"scenarioInstanceId"`
	TimestampMS        int       `json:"timestampMs"`
	Screen             string    `json:"screen"`
	Target             string    `json:"target"`
	IssueType          string    `json:"issueType,omitempty"`
}

type GraphEdge struct {
	ID                  string `json:"id"`
	From                string `json:"from"`
	To                  string `json:"to"`
	Weight              int    `json:"weight"`
	Frequency           int    `json:"frequency"`
	AverageTimeToNextMS int    `json:"averageTimeToNextMs"`
}

func JSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
