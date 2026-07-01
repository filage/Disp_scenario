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

func TestRoutineKnownScenariosCompleteWithoutIssueType(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		actions []string
		targets []string
	}{
		{
			name: "delivery destination", code: "CHANGE_DELIVERY_DESTINATION",
			actions: []string{"OPEN_ORDER", "OPEN_FIELD_EDITOR", "CHANGE_FIELD_VALUE", "SAVE"},
			targets: []string{"Order 101", "Edit delivery destination", "New delivery address", "Save delivery destination"},
		},
		{
			name: "recipient contact", code: "UPDATE_RECIPIENT_CONTACT",
			actions: []string{"OPEN_ORDER", "OPEN_FIELD_EDITOR", "CHANGE_FIELD_VALUE", "SAVE"},
			targets: []string{"Order 101", "Edit recipient contact", "New recipient phone", "Save recipient contact"},
		},
		{
			name: "delivery note", code: "ADD_DELIVERY_NOTE",
			actions: []string{"OPEN_ORDER", "OPEN_FIELD_EDITOR", "CHANGE_FIELD_VALUE", "SAVE"},
			targets: []string{"Order 101", "Add delivery note", "New delivery instructions", "Save delivery note"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recordingID, runID := uuid.New(), uuid.New()
			events := make([]ActionEvent, 0, len(test.actions))
			for index, action := range test.actions {
				event := scenarioAction(runID, (index+1)*1000, action, "", "101", "")
				event.Target = test.targets[index]
				events = append(events, event)
			}
			issues := []DataQualityIssue{}
			instances := BuildScenarioInstances(recordingID, events, &issues)
			if len(instances) != 1 {
				t.Fatalf("expected one routine scenario, got %#v", instances)
			}
			instance := instances[0]
			if instance.KnownScenarioCode != test.code || instance.Outcome != "completed" ||
				instance.StartedAtMS != 1000 || instance.EndedAtMS != len(test.actions)*1000 ||
				len(instance.EventIDs) != len(test.actions) {
				t.Fatalf("unexpected routine scenario: %#v", instance)
			}
			if len(issues) != 0 {
				t.Fatalf("unexpected quality issues: %#v", issues)
			}
		})
	}
}
