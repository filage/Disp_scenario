package jobs

import (
	"encoding/json"
	"testing"
)

func TestDecodeAnalysisPayload(t *testing.T) {
	payload, err := DecodeAnalysisPayload(json.RawMessage(`{
		"jobId":"00000000-0000-0000-0000-000000000001",
		"analysisRunId":"00000000-0000-0000-0000-000000000002",
		"recordingId":"00000000-0000-0000-0000-000000000003"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if payload.JobID == "" || payload.AnalysisRunID == "" || payload.RecordingID == "" {
		t.Fatalf("decoded payload is incomplete: %+v", payload)
	}
}

func TestDecodeAnalysisPayloadRequiresIDs(t *testing.T) {
	if _, err := DecodeAnalysisPayload(json.RawMessage(`{"jobId":"job"}`)); err == nil {
		t.Fatal("expected incomplete payload to fail")
	}
}
