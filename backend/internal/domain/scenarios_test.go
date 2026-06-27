package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestKnownOrderScenarioUsesStrictOpenAndResolveBoundaries(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	events := []ActionEvent{
		scenarioAction(runID, 1000, "INSPECT_ISSUE", "Late pickup", "101", ""),
		scenarioAction(runID, 2000, "OPEN_ORDER", "Late pickup", "101", ""),
		scenarioAction(runID, 3000, "MARK_PICKUP_COMPLETED", "Late pickup", "101", ""),
		scenarioAction(runID, 4000, "RESOLVE_ISSUE", "Late pickup", "101", ""),
		scenarioAction(runID, 5000, "NAVIGATE", "Late pickup", "101", ""),
	}
	issues := []DataQualityIssue{}
	instances := BuildScenarioInstances(recordingID, events, &issues)
	if len(instances) != 1 {
		t.Fatalf("expected one strict known scenario, got %#v", instances)
	}
	instance := instances[0]
	if instance.StartedAtMS != 2000 || instance.EndedAtMS != 4000 || len(instance.EventIDs) != 3 {
		t.Fatalf("unexpected strict boundaries: %#v", instance)
	}
	groups := BuildScenarioGroups(instances, events)
	if len(groups) != 1 || !strings.HasPrefix(groups[0].Signature, "LATE_PICKUP>OPEN_ORDER>") ||
		!strings.HasSuffix(groups[0].Signature, ">RESOLVE_ISSUE") {
		t.Fatalf("unexpected strict signature: %#v", groups)
	}
}

func TestKnownOrderScenarioDoesNotStartWithoutOpenOrder(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	events := []ActionEvent{
		scenarioAction(runID, 1000, "TAKE_ACTION", "Unassigned courier", "101", ""),
		scenarioAction(runID, 2000, "ASSIGN_DRIVER", "Unassigned courier", "101", ""),
		scenarioAction(runID, 3000, "RESOLVE_ISSUE", "Unassigned courier", "101", ""),
	}
	issues := []DataQualityIssue{}
	if instances := BuildScenarioInstances(recordingID, events, &issues); len(instances) != 0 {
		t.Fatalf("known scenario started without OPEN_ORDER: %#v", instances)
	}
}
