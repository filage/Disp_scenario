package artifacts

import (
	"testing"

	"github.com/google/uuid"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
)

func TestTemplatesFromCurrentDataExcludeDeletedRecording(t *testing.T) {
	firstRecordingID := uuid.New()
	secondRecordingID := uuid.New()
	firstEventID := uuid.New()
	secondEventID := uuid.New()
	events := []domain.ActionEvent{
		{
			ID: firstEventID, RecordingID: firstRecordingID,
			CanonicalAction: "CHECK", EventType: "manual", Confidence: 0.8,
		},
		{
			ID: secondEventID, RecordingID: secondRecordingID,
			CanonicalAction: "CHECK", EventType: "manual", Confidence: 0.8,
		},
	}
	instances := []domain.ScenarioInstance{
		{
			ID: uuid.New(), RecordingID: firstRecordingID,
			KnownScenarioCode: "LATE_DELIVERY", IssueType: "late delivery",
			DurationMS: 1000, EventIDs: []uuid.UUID{firstEventID},
			Outcome: "completed", Status: "confirmed", Confidence: 0.8,
		},
		{
			ID: uuid.New(), RecordingID: secondRecordingID,
			KnownScenarioCode: "LATE_DELIVERY", IssueType: "late delivery",
			DurationMS: 3000, EventIDs: []uuid.UUID{secondEventID},
			Outcome: "completed", Status: "confirmed", Confidence: 0.8,
		},
	}

	before := templatesFromDomain(domain.BuildScenarioGroups(instances, events))
	if len(before) != 1 {
		t.Fatalf("unexpected group count before deletion: %d", len(before))
	}
	if before[0].Frequency != 2 || before[0].AverageDurationMS != 2000 || before[0].ManualCheckCount != 2 {
		t.Fatalf("unexpected metrics before deletion: %+v", before[0])
	}

	after := templatesFromDomain(domain.BuildScenarioGroups(instances[1:], events[1:]))
	if len(after) != 1 {
		t.Fatalf("unexpected group count after deletion: %d", len(after))
	}
	if after[0].Frequency != 1 || after[0].AverageDurationMS != 3000 || after[0].ManualCheckCount != 1 {
		t.Fatalf("unexpected metrics after deletion: %+v", after[0])
	}
	if after[0].AutomationScore >= before[0].AutomationScore {
		t.Fatalf(
			"automation score was not recalculated: before=%v after=%v",
			before[0].AutomationScore, after[0].AutomationScore,
		)
	}
}
