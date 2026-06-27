package vision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
	"github.com/example/dispscenario-analyst-v2/internal/media"
)

const geminiBaseURL = "https://generativelanguage.googleapis.com"
const PromptVersion = "video-raw-extractor-v8"
const gemini35FlashStandardPricingVersion = "gemini-3.5-flash-standard-2026-06-26"

var geminiRetryDelay = func(attempt int) time.Duration {
	return time.Duration(attempt+1) * 2 * time.Second
}

var uiVocabulary = []string{
	"Deliveries",
	"Attention Required",
	"Order details",
	"Schedule",
	"Pickup Window",
	"Dropoff Window",
	"Driver info",
	"Send Order",
	"To Drivers",
	"Take Action",
	"Resolve late pickup",
	"Mark pickup completed",
	"Confirm courier assigned",
	"Cancel",
	"Save",
	"Edit",
}

var uiColorTriggers = []string{
	"red or orange badges, borders, icons, warning rows: attention required, late pickup, blocked order, manual escalation risk",
	"yellow or amber highlights: warning, pending review, schedule conflict, needs dispatcher check",
	"green badges, checks, success buttons, resolved rows: completed, resolved, pickup completed, courier confirmed",
	"blue selected rows, active tabs, primary buttons: current selection, navigation target, active workflow step",
	"gray disabled controls, muted rows, dimmed buttons: unavailable action, inactive state, already processed or not selectable",
}

type GeminiProvider struct {
	APIKey string
	Model  string
	Client *http.Client
}

type geminiFile struct {
	Name     string `json:"name"`
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	State    string `json:"state"`
	Error    any    `json:"error,omitempty"`
}

type geminiUploadResponse struct {
	File geminiFile `json:"file"`
}

type geminiOutput struct {
	RawEvents []struct {
		TimestampMS    int            `json:"timestampMs"`
		Screen         string         `json:"screen"`
		VisibleText    string         `json:"visibleText"`
		Target         string         `json:"target"`
		EventTypeGuess string         `json:"eventTypeGuess"`
		ColorCues      []string       `json:"colorCues"`
		StateChange    string         `json:"stateChange"`
		Confidence     float64        `json:"confidence"`
		Payload        map[string]any `json:"payload"`
	} `json:"rawEvents"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback any                  `json:"promptFeedback,omitempty"`
	UsageMetadata  *geminiUsageMetadata `json:"usageMetadata,omitempty"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int64 `json:"promptTokenCount"`
	CandidatesTokenCount int64 `json:"candidatesTokenCount"`
	ThoughtsTokenCount   int64 `json:"thoughtsTokenCount"`
	TotalTokenCount      int64 `json:"totalTokenCount"`
}

type generatedContent struct {
	Text  string
	Usage *Usage
}

func (provider GeminiProvider) Extract(
	ctx context.Context,
	analysisRunID uuid.UUID,
	videoPath string,
	metadata media.Metadata,
) (Result, error) {
	if provider.APIKey == "" {
		return Result{}, fmt.Errorf("GEMINI_API_KEY is empty")
	}
	if provider.Model == "" {
		provider.Model = "gemini-3.5-flash"
	}
	if provider.Client == nil {
		provider.Client = &http.Client{Timeout: 10 * time.Minute}
	}

	file, err := provider.upload(ctx, videoPath)
	if err != nil {
		return Result{}, err
	}
	defer provider.deleteFile(context.Background(), file.Name)

	file, err = provider.waitActive(ctx, file)
	if err != nil {
		return Result{}, err
	}
	generated, err := provider.generate(ctx, file, metadata)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Provider: "gemini",
		Model:    provider.Model,
		RawText:  generated.Text,
		Usage:    generated.Usage,
		Cost:     estimateGeminiCost(provider.Model, generated.Usage),
	}
	output, err := decodeGeminiOutput(generated.Text)
	if err != nil {
		return result, fmt.Errorf("decode Gemini structured output: %w", err)
	}
	events := make([]domain.RawVisionEvent, 0, len(output.RawEvents))
	for _, item := range output.RawEvents {
		if item.TimestampMS < 0 || strings.TrimSpace(item.Screen) == "" {
			continue
		}
		confidence := item.Confidence
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		if item.Payload == nil {
			item.Payload = map[string]any{}
		}
		events = append(events, domain.RawVisionEvent{
			ID: uuid.New(), AnalysisRunID: analysisRunID,
			TimestampMS: item.TimestampMS, Screen: item.Screen,
			VisibleText: item.VisibleText, Target: item.Target,
			EventTypeGuess: item.EventTypeGuess, ColorCues: item.ColorCues,
			StateChange: item.StateChange, Confidence: confidence, Payload: item.Payload,
		})
	}
	if len(events) == 0 {
		return result, fmt.Errorf("gemini returned no valid raw events")
	}
	result.RawEvents = events
	return result, nil
}

func decodeGeminiOutput(rawText string) (geminiOutput, error) {
	candidates := []string{strings.TrimSpace(rawText)}
	trimmed := strings.TrimSpace(rawText)
	if strings.HasPrefix(trimmed, "```") {
		if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
			fenced := strings.TrimSpace(trimmed[newline+1:])
			fenced = strings.TrimSuffix(fenced, "```")
			candidates = append(candidates, strings.TrimSpace(fenced))
		}
	}
	if start, end := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}"); start >= 0 && end > start {
		candidates = append(candidates, trimmed[start:end+1])
	}

	var lastErr error
	for _, candidate := range candidates {
		var output geminiOutput
		if err := json.Unmarshal([]byte(candidate), &output); err == nil {
			return output, nil
		} else {
			lastErr = err
		}
	}
	return geminiOutput{}, lastErr
}

func (provider GeminiProvider) upload(ctx context.Context, videoPath string) (geminiFile, error) {
	info, err := os.Stat(videoPath)
	if err != nil {
		return geminiFile{}, err
	}
	mimeType := mimeTypeForPath(videoPath)
	body, _ := json.Marshal(map[string]any{"file": map[string]string{"display_name": filepath.Base(videoPath)}})
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, geminiBaseURL+"/upload/v1beta/files", bytes.NewReader(body))
	request.Header.Set("x-goog-api-key", provider.APIKey)
	request.Header.Set("X-Goog-Upload-Protocol", "resumable")
	request.Header.Set("X-Goog-Upload-Command", "start")
	request.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprint(info.Size()))
	request.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.Client.Do(request)
	if err != nil {
		return geminiFile{}, fmt.Errorf("start Gemini upload: %w", err)
	}
	defer func(body io.ReadCloser) { _ = body.Close() }(response.Body)
	if response.StatusCode/100 != 2 {
		return geminiFile{}, responseError("start Gemini upload", response)
	}
	uploadURL := response.Header.Get("X-Goog-Upload-URL")
	if uploadURL == "" {
		return geminiFile{}, fmt.Errorf("gemini upload URL is missing")
	}

	fileBody, err := os.Open(videoPath)
	if err != nil {
		return geminiFile{}, err
	}
	defer func() { _ = fileBody.Close() }()
	request, _ = http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, fileBody)
	request.ContentLength = info.Size()
	request.Header.Set("Content-Type", mimeType)
	request.Header.Set("X-Goog-Upload-Offset", "0")
	request.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	response, err = provider.Client.Do(request)
	if err != nil {
		return geminiFile{}, fmt.Errorf("upload video to Gemini: %w", err)
	}
	defer func(body io.ReadCloser) { _ = body.Close() }(response.Body)
	if response.StatusCode/100 != 2 {
		return geminiFile{}, responseError("upload video to Gemini", response)
	}
	var uploaded geminiUploadResponse
	if err := json.NewDecoder(response.Body).Decode(&uploaded); err != nil {
		return geminiFile{}, fmt.Errorf("decode Gemini upload: %w", err)
	}
	return uploaded.File, nil
}

func (provider GeminiProvider) waitActive(ctx context.Context, file geminiFile) (geminiFile, error) {
	for {
		switch file.State {
		case "ACTIVE":
			return file, nil
		case "FAILED":
			return geminiFile{}, fmt.Errorf("gemini file processing failed: %v", file.Error)
		}
		select {
		case <-ctx.Done():
			return geminiFile{}, ctx.Err()
		case <-time.After(5 * time.Second):
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, geminiBaseURL+"/v1beta/"+file.Name, nil)
		request.Header.Set("x-goog-api-key", provider.APIKey)
		response, err := provider.Client.Do(request)
		if err != nil {
			return geminiFile{}, fmt.Errorf("poll Gemini file: %w", err)
		}
		if response.StatusCode/100 != 2 {
			err = responseError("poll Gemini file", response)
			_ = response.Body.Close()
			return geminiFile{}, err
		}
		err = json.NewDecoder(response.Body).Decode(&file)
		closeErr := response.Body.Close()
		if err != nil {
			return geminiFile{}, fmt.Errorf("decode Gemini file status: %w", err)
		}
		if closeErr != nil {
			return geminiFile{}, fmt.Errorf("close Gemini file status response: %w", closeErr)
		}
	}
}

func (provider GeminiProvider) generate(ctx context.Context, file geminiFile, metadata media.Metadata) (generatedContent, error) {
	prompt := buildRawExtractionPrompt(metadata)
	schema := rawExtractionSchema()
	payload := map[string]any{
		"contents": []any{map[string]any{"role": "user", "parts": []any{
			map[string]any{"fileData": map[string]string{"fileUri": file.URI, "mimeType": file.MimeType}},
			map[string]any{"text": prompt},
		}}},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json", "responseJsonSchema": schema,
			"temperature": 0.1, "maxOutputTokens": 32768,
		},
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", geminiBaseURL, provider.Model)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		request.Header.Set("x-goog-api-key", provider.APIKey)
		request.Header.Set("Content-Type", "application/json")
		response, err := provider.Client.Do(request)
		if err != nil {
			lastErr = err
		} else if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			lastErr = responseError("generate Gemini content", response)
			_ = response.Body.Close()
		} else if response.StatusCode/100 != 2 {
			err = responseError("generate Gemini content", response)
			_ = response.Body.Close()
			return generatedContent{}, err
		} else {
			var generated generateResponse
			err = json.NewDecoder(response.Body).Decode(&generated)
			closeErr := response.Body.Close()
			if err != nil {
				return generatedContent{}, fmt.Errorf("decode Gemini response: %w", err)
			}
			if closeErr != nil {
				return generatedContent{}, fmt.Errorf("close Gemini response: %w", closeErr)
			}
			if len(generated.Candidates) == 0 || len(generated.Candidates[0].Content.Parts) == 0 {
				return generatedContent{}, fmt.Errorf("gemini returned no candidate: %v", generated.PromptFeedback)
			}
			return generatedContent{
				Text:  generated.Candidates[0].Content.Parts[0].Text,
				Usage: usageFromGemini(generated.UsageMetadata),
			}, nil
		}
		select {
		case <-ctx.Done():
			return generatedContent{}, ctx.Err()
		case <-time.After(geminiRetryDelay(attempt)):
		}
	}
	return generatedContent{}, fmt.Errorf("gemini request failed after retries: %w", lastErr)
}

func usageFromGemini(metadata *geminiUsageMetadata) *Usage {
	if metadata == nil {
		return nil
	}
	return &Usage{
		InputTokens:    metadata.PromptTokenCount,
		OutputTokens:   metadata.CandidatesTokenCount,
		ThinkingTokens: metadata.ThoughtsTokenCount,
		TotalTokens:    metadata.TotalTokenCount,
	}
}

func estimateGeminiCost(model string, usage *Usage) *CostEstimate {
	if usage == nil || !strings.HasPrefix(model, "gemini-3.5-flash") {
		return nil
	}
	const (
		inputUSDPerMillion  = 1.50
		outputUSDPerMillion = 9.00
	)
	outputTokens := usage.OutputTokens + usage.ThinkingTokens
	return &CostEstimate{
		USD: (float64(usage.InputTokens)*inputUSDPerMillion +
			float64(outputTokens)*outputUSDPerMillion) / 1_000_000,
		PricingVersion: gemini35FlashStandardPricingVersion,
	}
}

func buildRawExtractionPrompt(metadata media.Metadata) string {
	return fmt.Sprintf(`You are analyzing a screen recording of a dispatcher dashboard.

Goal:
Watch the full video and extract only raw factual observations from it. The existing client UI cannot be instrumented, so the video is the main source of truth.

Important boundary:
You are not the authority for final scenario groups, metrics, automation score, or reports. Application code will normalize observations, detect scenario boundaries, match known scenarios, calculate metrics, and build reports deterministically.

Known UI vocabulary:
- %s

Known issue types:
- Late pickup
- Unassigned courier
- Unknown

Visual color/state triggers:
- %s.

Return only JSON matching the supplied response schema.

Rules:
- Use timestamps in milliseconds from video start.
- Watch the full video timeline, not only isolated frames. Capture transitions, modal openings, state changes, and short operator actions when visible.
- Record clicking the "Attention Required" tab/filter as its own filtering observation. It filters the list and does not open a specific order.
- Treat opening "Take Action" and selecting an option inside it as two separate events. Always capture the selected option, especially "Confirm courier assigned", "Resolve late pickup", and "Mark pickup completed", even when the menu is visible for less than one second.
- Treat driver assignment as separate events when visible: opening the driver assignment modal, selecting a driver checkbox, clicking "Send to Selected", and later confirming courier assignment.
- Treat schedule/field edits as separate events when visible: clicking "Edit", changing a value or time input, and clicking "Save".
- Treat hovering an issue badge until a tooltip appears as a separate event. Use eventTypeGuess "hover", include the exact tooltip text in visibleText, and include the corresponding orderId in payload.
- Operators may inspect several issue badges before choosing one order to process. Record every hover separately and never merge issue text between different orderIds.
- Detect every tooltip-producing badge hover independently of later clicks. Preserve the exact sequence, including repeated inspection of the same badge after its tooltip disappears.
- Bind a badge hover to the order row spatially under the pointer, using the order number visible in that same row. Do not infer the hover's orderId from the next or nearest order-row click.
- Put the exact row/order text used for badge ownership in payload.orderIdEvidence, for example "ORD-1014 visible in the hovered row".
- If the hovered row's order number is not readable, set payload.orderId to an empty string, explain the ambiguity in payload.notes, and set confidence below 0.6. Never guess the orderId from temporal proximity.
- Inspect the interval immediately after a successful assignment or save at higher temporal attention: operators often open "Take Action" and confirm resolution before closing the order.
- Do not replace a resolution-option click with a later navigation or panel-close event. Record both events with separate timestamps when both occur.
- Treat a selected menu option whose visible label starts with "Resolve" as a resolution action even when the remaining label contains a UI typo, such as "Resolve late price". Preserve the exact visible text and describe the resulting success or error state factually.
- Prefer explicit UI text visible in the video.
- Use color and visual state as supporting evidence when text is partially unreadable.
- Record relevant colors, badges, row highlights, disabled states, focus/selection changes, success toasts, error toasts, and validation messages in colorCues and stateChange.
- A number inside a red badge is a count, not an exclamation mark. Do not infer a specific issue type from the badge color or count alone.
- A generic red/orange badge does not identify an issue type. Set payload.possibleIssueType only to "Late pickup" or "Unassigned courier" when readable text or a later explicit action for the same order supports it; otherwise use "Unknown".
- A generic "Cancel" button shown beside "Save" in an edit form only cancels editing and does not identify a scenario.
- When the operator hovers an attention badge and a tooltip appears, record a separate observation containing the tooltip text. Treat that text as stronger evidence than color or icon shape.
- Treat red/orange attention indicators plus pickup-related text or action as supporting evidence for Late pickup.
- Treat green/check/resolved indicators after a click as evidence for a resolved outcome only if the UI state visibly changes.
- If an attempted resolution shows an error or validation toast, describe it as a failed attempt. Do not describe the issue as resolved.
- If a click target is uncertain, set confidence below 0.6.
- Do not create scenario summaries, reports, grouped workflows, automation candidates, or final metrics.
- Do not infer hidden business state that is not visible.
- Mark fast ambiguous actions with low confidence and factual stateChange text.
- Every event must include all schema fields. Use empty strings or empty arrays for fields with no observable value; never omit them.
- payload must always include orderId, orderIdEvidence, possibleIssueType, and notes. Use empty strings when unavailable and "Unknown" when the issue type is not supported.

Video metadata:
{
  "durationSec": %.2f,
  "width": %d,
  "height": %d,
  "codec": %q
}

Prompt version: %s`,
		strings.Join(uiVocabulary, "\n- "),
		strings.Join(uiColorTriggers, ".\n- "),
		metadata.DurationSec,
		metadata.Width,
		metadata.Height,
		metadata.Codec,
		PromptVersion,
	)
}

func rawExtractionSchema() map[string]any {
	return map[string]any{
		"type": "object", "required": []string{"rawEvents"},
		"properties": map[string]any{"rawEvents": map[string]any{
			"type": "array", "items": map[string]any{
				"type": "object",
				"required": []string{
					"timestampMs", "screen", "visibleText", "target", "eventTypeGuess",
					"colorCues", "stateChange", "confidence", "payload",
				},
				"properties": map[string]any{
					"timestampMs":    map[string]any{"type": "integer", "minimum": 0},
					"screen":         map[string]any{"type": "string"},
					"visibleText":    map[string]any{"type": "string"},
					"target":         map[string]any{"type": "string"},
					"eventTypeGuess": map[string]any{"type": "string"},
					"colorCues":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"stateChange":    map[string]any{"type": "string"},
					"confidence":     map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					"payload": map[string]any{
						"type": "object",
						"required": []string{
							"orderId", "orderIdEvidence", "possibleIssueType", "notes",
						},
						"properties": map[string]any{
							"orderId":         map[string]any{"type": "string"},
							"orderIdEvidence": map[string]any{"type": "string"},
							"possibleIssueType": map[string]any{
								"type": "string",
								"enum": []string{"Late pickup", "Unassigned courier", "Unknown"},
							},
							"notes": map[string]any{"type": "string"},
						},
						"additionalProperties": true,
					},
				},
			},
		}},
	}
}

func (provider GeminiProvider) deleteFile(ctx context.Context, name string) {
	if name == "" || provider.Client == nil {
		return
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodDelete, geminiBaseURL+"/v1beta/"+name, nil)
	request.Header.Set("x-goog-api-key", provider.APIKey)
	response, err := provider.Client.Do(request)
	if err == nil {
		_ = response.Body.Close()
	}
}

func mimeTypeForPath(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".webm") {
		return "video/webm"
	}
	return "video/mp4"
}

func responseError(operation string, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	return fmt.Errorf("%s: HTTP %d: %s", operation, response.StatusCode, strings.TrimSpace(string(body)))
}
