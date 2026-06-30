package artifacts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type boundaryReview struct {
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	Proposals []boundaryProposal `json:"proposedScenarios"`
	Warnings  []boundaryWarning  `json:"boundaryWarnings"`
}

type boundaryProposal struct {
	StartTimestampMS  int     `json:"startTimestampMs"`
	EndTimestampMS    int     `json:"endTimestampMs"`
	IssueType         string  `json:"issueType"`
	KnownScenarioCode string  `json:"knownScenarioCode"`
	OrderID           string  `json:"orderId"`
	Outcome           string  `json:"outcome"`
	Confidence        float64 `json:"confidence"`
}

type boundaryWarning struct {
	TimestampMS int    `json:"timestampMs"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
}

func shouldSuppressBoundaryWarning(
	warning boundaryWarning,
	events []ActionEvent,
	instances []ScenarioInstance,
) bool {
	message := strings.ToLower(warning.Message)
	if strings.Contains(message, "send to selected") {
		assignment := nearestActionEvent(events, warning.TimestampMS)
		if assignment == nil ||
			(assignment.CanonicalAction != "SEND_TO_SELECTED_DRIVER" &&
				assignment.CanonicalAction != "ASSIGN_DRIVER") {
			return false
		}
		for _, instance := range instances {
			if valueString(instance.KnownScenarioCode) != "UNASSIGNED_COURIER" ||
				instance.Outcome != "resolved" || instance.Status != "confirmed" ||
				instance.StartedAtMS > assignment.TimestampMS ||
				instance.EndedAtMS <= assignment.TimestampMS {
				continue
			}
			var eventIDs []uuid.UUID
			_ = json.Unmarshal(instance.EventIDs, &eventIDs)
			for _, eventID := range eventIDs {
				if eventID == assignment.ID {
					return true
				}
			}
		}
		return false
	}
	if !isPostResolutionConfirmation(message) {
		return false
	}
	for _, instance := range instances {
		if instance.Outcome == "resolved" && instance.Status == "confirmed" &&
			instance.EndedAtMS <= warning.TimestampMS {
			return true
		}
	}
	return false
}

func nearestActionEvent(events []ActionEvent, timestampMS int) *ActionEvent {
	if len(events) == 0 {
		return nil
	}
	best := &events[0]
	bestDistance := absInt(best.TimestampMS - timestampMS)
	for index := 1; index < len(events); index++ {
		distance := absInt(events[index].TimestampMS - timestampMS)
		if distance < bestDistance {
			best, bestDistance = &events[index], distance
		}
	}
	return best
}

func isPostResolutionConfirmation(message string) bool {
	returnsToList := (containsAnyText(message, "return", "back", "возврат", "вернул") &&
		containsAnyText(message, "deliveries", "list", "список"))
	resolvedVisualState := containsAnyText(
		message,
		"without warning", "without badge", "badge disappear", "indicator disappear",
		"status 'working", `status "working`, "без предупреждающего индикатора",
		"индикатор исчез", "статус 'working", `статус "working`,
	)
	return returnsToList && resolvedVisualState
}

func containsAnyText(source string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(source, value) {
			return true
		}
	}
	return false
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (s *Service) reviewBoundaries(ctx context.Context, recordingID uuid.UUID, geminiAPIKey string) (boundaryReview, error) {
	if geminiAPIKey == "" {
		geminiAPIKey = s.geminiAPIKey
	}
	if geminiAPIKey == "" {
		return boundaryReview{}, fmt.Errorf("GEMINI_API_KEY is not configured")
	}
	rawEvents, err := s.RawEvents(ctx, recordingID)
	if err != nil {
		return boundaryReview{}, err
	}
	events, err := s.Events(ctx, recordingID)
	if err != nil {
		return boundaryReview{}, err
	}
	instances, err := s.Instances(ctx, &recordingID)
	if err != nil {
		return boundaryReview{}, err
	}
	input, _ := json.Marshal(map[string]any{
		"rawEvents": rawEvents, "events": events, "deterministicInstances": instances,
	})
	prompt := `Review deterministic scenario boundaries against the observed events.
Return only disagreements, missing scenarios, or ambiguous boundaries.
Do not rewrite events. Keep timestamps in milliseconds. Input: ` + string(input)
	schema := map[string]any{
		"type":     "object",
		"required": []string{"proposedScenarios", "boundaryWarnings"},
		"properties": map[string]any{
			"proposedScenarios": map[string]any{
				"type": "array", "items": map[string]any{
					"type":     "object",
					"required": []string{"startTimestampMs", "endTimestampMs", "confidence"},
					"properties": map[string]any{
						"startTimestampMs":  map[string]any{"type": "integer", "minimum": 0},
						"endTimestampMs":    map[string]any{"type": "integer", "minimum": 0},
						"issueType":         map[string]any{"type": "string"},
						"knownScenarioCode": map[string]any{"type": "string"},
						"orderId":           map[string]any{"type": "string"},
						"outcome":           map[string]any{"type": "string"},
						"confidence":        map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					},
				},
			},
			"boundaryWarnings": map[string]any{
				"type": "array", "items": map[string]any{
					"type":     "object",
					"required": []string{"timestampMs", "severity", "message"},
					"properties": map[string]any{
						"timestampMs": map[string]any{"type": "integer", "minimum": 0},
						"severity":    map[string]any{"type": "string", "enum": []string{"info", "warning", "error"}},
						"message":     map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	payload, _ := json.Marshal(map[string]any{
		"contents": []any{map[string]any{"role": "user", "parts": []any{
			map[string]any{"text": prompt},
		}}},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json", "responseJsonSchema": schema,
			"temperature": 0.1,
		},
	})
	model := s.geminiModel
	if model == "" {
		model = "gemini-3.5-flash"
	}
	request, _ := http.NewRequestWithContext(
		ctx, http.MethodPost,
		"https://generativelanguage.googleapis.com/v1beta/models/"+model+":generateContent",
		bytes.NewReader(payload),
	)
	request.Header.Set("x-goog-api-key", geminiAPIKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return boundaryReview{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 32*1024))
		return boundaryReview{}, fmt.Errorf(
			"gemini boundary review HTTP %d: %s",
			response.StatusCode, strings.TrimSpace(string(body)),
		)
	}
	var generated struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(response.Body).Decode(&generated); err != nil {
		return boundaryReview{}, err
	}
	if len(generated.Candidates) == 0 || len(generated.Candidates[0].Content.Parts) == 0 {
		return boundaryReview{}, fmt.Errorf("gemini returned no boundary review")
	}
	var review boundaryReview
	if err := json.Unmarshal([]byte(generated.Candidates[0].Content.Parts[0].Text), &review); err != nil {
		return boundaryReview{}, err
	}
	review.Provider, review.Model = "gemini", model
	return review, nil
}
