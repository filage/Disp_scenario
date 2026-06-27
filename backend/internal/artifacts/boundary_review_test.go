package artifacts

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestSuppressIntermediateAssignmentWarning(t *testing.T) {
	assignmentID := uuid.New()
	code := "UNASSIGNED_COURIER"
	eventIDs, _ := json.Marshal([]uuid.UUID{assignmentID, uuid.New()})
	events := []ActionEvent{
		{ID: assignmentID, TimestampMS: 16500, CanonicalAction: "SEND_TO_SELECTED_DRIVER"},
		{ID: uuid.New(), TimestampMS: 19000, CanonicalAction: "RESOLVE_ISSUE"},
	}
	instances := []ScenarioInstance{{
		KnownScenarioCode: &code, Outcome: "resolved", Status: "confirmed",
		StartedAtMS: 7200, EndedAtMS: 19000, EventIDs: eventIDs,
	}}
	if !shouldSuppressBoundaryWarning(boundaryWarning{
		TimestampMS: 16500,
		Message:     "Action 'Send to Selected' starts assignment before confirmation.",
	}, events, instances) {
		t.Fatal("expected intermediate assignment warning to be suppressed")
	}
}

func TestSuppressPostResolutionListConfirmation(t *testing.T) {
	instances := []ScenarioInstance{{
		Outcome: "resolved", Status: "confirmed", StartedAtMS: 17800, EndedAtMS: 29300,
	}}
	if !shouldSuppressBoundaryWarning(boundaryWarning{
		TimestampMS: 33200,
		Message:     "Return to deliveries list confirms status 'Working' without warning.",
	}, nil, instances) {
		t.Fatal("expected post-resolution confirmation warning to be suppressed")
	}
}
