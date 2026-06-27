package vision

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/example/dispscenario-analyst-v2/internal/media"
)

func TestDecodeGeminiOutput(t *testing.T) {
	payload := `{"rawEvents":[{"timestampMs":10,"screen":"Deliveries","eventTypeGuess":"click","confidence":0.9}]}`
	tests := []struct {
		name string
		raw  string
	}{
		{name: "plain JSON", raw: payload},
		{name: "markdown fence", raw: "```json\n" + payload + "\n```"},
		{name: "surrounding prose", raw: "Structured result:\n" + payload + "\nEnd."},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			output, err := decodeGeminiOutput(current.raw)
			if err != nil {
				t.Fatal(err)
			}
			if len(output.RawEvents) != 1 || output.RawEvents[0].TimestampMS != 10 {
				t.Fatalf("unexpected output: %#v", output)
			}
		})
	}
}

func TestDecodeGeminiOutputRejectsInvalidJSON(t *testing.T) {
	if _, err := decodeGeminiOutput("not JSON"); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestGenerateRetriesTemporaryGeminiFailure(t *testing.T) {
	oldDelay := geminiRetryDelay
	geminiRetryDelay = func(int) time.Duration { return 0 }
	t.Cleanup(func() { geminiRetryDelay = oldDelay })

	requests := 0
	provider := GeminiProvider{
		APIKey: "test-key",
		Model:  "gemini-3.5-flash",
		Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			if requests == 1 {
				return stringResponse(http.StatusServiceUnavailable, `temporary overload`), nil
			}
			return stringResponse(http.StatusOK, `{
				"candidates":[{"content":{"parts":[{"text":"{\"rawEvents\":[]}"}]}}],
				"usageMetadata":{
					"promptTokenCount":1000,
					"candidatesTokenCount":200,
					"thoughtsTokenCount":50,
					"totalTokenCount":1250
				}
			}`), nil
		})},
	}
	generated, err := provider.generate(context.Background(), geminiFile{
		URI: "gemini://file", MimeType: "video/mp4",
	}, media.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if generated.Text != `{"rawEvents":[]}` {
		t.Fatalf("unexpected generated text: %s", generated.Text)
	}
	if generated.Usage == nil || generated.Usage.TotalTokens != 1250 {
		t.Fatalf("unexpected generated usage: %#v", generated.Usage)
	}
	if requests != 2 {
		t.Fatalf("expected one retry after temporary failure, got %d requests", requests)
	}
}

func TestEstimateGemini35FlashStandardCost(t *testing.T) {
	estimate := estimateGeminiCost("gemini-3.5-flash", &Usage{
		InputTokens:    1_000_000,
		OutputTokens:   100_000,
		ThinkingTokens: 50_000,
		TotalTokens:    1_150_000,
	})
	if estimate == nil {
		t.Fatal("expected cost estimate")
	}
	const expected = 2.85
	if math.Abs(estimate.USD-expected) > 1e-9 {
		t.Fatalf("unexpected cost: got %.8f want %.8f", estimate.USD, expected)
	}
	if estimate.PricingVersion != gemini35FlashStandardPricingVersion {
		t.Fatalf("unexpected pricing version: %s", estimate.PricingVersion)
	}
}

func TestEstimateGeminiCostSkipsUnknownModel(t *testing.T) {
	if estimate := estimateGeminiCost("custom-model", &Usage{InputTokens: 100}); estimate != nil {
		t.Fatalf("expected no estimate for unknown model, got %#v", estimate)
	}
}

func TestRawExtractionPromptPreservesLegacyCriticalRules(t *testing.T) {
	prompt := buildRawExtractionPrompt(media.Metadata{
		DurationSec: 42.5,
		Width:       1920,
		Height:      1080,
		Codec:       "h264",
	})
	requiredFragments := []string{
		`clicking the "Attention Required" tab/filter`,
		`opening "Take Action" and selecting an option inside it as two separate events`,
		`Use eventTypeGuess "hover"`,
		`payload.orderIdEvidence`,
		`Never guess the orderId from temporal proximity`,
		`Resolve late price`,
		`success or error state factually`,
		`Do not describe the issue as resolved`,
		`payload must always include orderId, orderIdEvidence, possibleIssueType, and notes`,
		PromptVersion,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt is missing critical legacy rule %q", fragment)
		}
	}
}

func TestRawExtractionSchemaRequiresCompleteObservableEvent(t *testing.T) {
	schema := rawExtractionSchema()
	properties := schema["properties"].(map[string]any)
	rawEvents := properties["rawEvents"].(map[string]any)
	item := rawEvents["items"].(map[string]any)
	required := toStringSet(item["required"].([]string))
	for _, field := range []string{
		"timestampMs", "screen", "visibleText", "target", "eventTypeGuess",
		"colorCues", "stateChange", "confidence", "payload",
	} {
		if !required[field] {
			t.Errorf("event schema does not require %q", field)
		}
	}

	itemProperties := item["properties"].(map[string]any)
	payload := itemProperties["payload"].(map[string]any)
	payloadRequired := toStringSet(payload["required"].([]string))
	for _, field := range []string{"orderId", "orderIdEvidence", "possibleIssueType", "notes"} {
		if !payloadRequired[field] {
			t.Errorf("payload schema does not require %q", field)
		}
	}
}

func toStringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func stringResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
