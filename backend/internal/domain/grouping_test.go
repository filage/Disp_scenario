package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestBuildScenarioGroupsCreatesStableAutomationCandidate(t *testing.T) {
	recordingID := uuid.New()
	runID := uuid.New()
	events := []ActionEvent{
		{ID: uuid.New(), RecordingID: recordingID, AnalysisRunID: runID, TimestampMS: 0, CanonicalAction: "CHECK", Target: "order", Confidence: .9},
		{ID: uuid.New(), RecordingID: recordingID, AnalysisRunID: runID, TimestampMS: 120_000, CanonicalAction: "EDIT_FIELD", Target: "pickup", Confidence: .9},
		{ID: uuid.New(), RecordingID: recordingID, AnalysisRunID: runID, TimestampMS: 180_000, CanonicalAction: "RESOLVE_ISSUE", Target: "resolve", Confidence: .9},
	}
	instances := make([]ScenarioInstance, 3)
	for index := range instances {
		instances[index] = ScenarioInstance{
			ID: uuid.New(), RecordingID: recordingID, AnalysisRunID: runID,
			KnownScenarioCode: "LATE_PICKUP", IssueType: "Late pickup",
			StartedAtMS: 0, EndedAtMS: 180_000, DurationMS: 180_000,
			EventIDs: []uuid.UUID{events[0].ID, events[1].ID, events[2].ID},
			Outcome:  "resolved", Status: "confirmed", Confidence: .9,
		}
	}

	first := BuildScenarioGroups(instances, events)
	second := BuildScenarioGroups(instances, events)
	if len(first) != 1 || len(first[0].AutomationCandidates) < 3 {
		t.Fatalf("expected one group and rich candidate set, got %#v", first)
	}
	if first[0].ID != second[0].ID {
		t.Fatalf("group id is not deterministic: %s != %s", first[0].ID, second[0].ID)
	}
	for index := range first[0].AutomationCandidates {
		if first[0].AutomationCandidates[index].ID != second[0].AutomationCandidates[index].ID {
			t.Fatal("candidate id is not deterministic")
		}
	}
	candidate := first[0].AutomationCandidates[0]
	for _, key := range []string{
		"level", "frequency", "averageDurationMs", "durationImpactMs",
		"repeatability", "manualCheckImpact", "errorReduction",
		"dataQualityConfidence", "factors", "weights", "sampleSize",
	} {
		if _, ok := candidate.Breakdown[key]; !ok {
			t.Fatalf("candidate breakdown is missing %q: %#v", key, candidate.Breakdown)
		}
	}
}
