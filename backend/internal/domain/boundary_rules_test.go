package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestConfiguredGenericStartAndResolutionEndRules(t *testing.T) {
	recordingID := uuid.New()
	runID := uuid.New()
	events := []ActionEvent{
		scenarioAction(runID, 1000, "CHECK", "Custom issue", "101", ""),
		scenarioAction(runID, 3000, "RESOLVE_ISSUE", "Custom issue", "101", "Resolved successfully"),
	}
	config := ScenarioConfig{
		KnownScenarios: []KnownScenario{},
		BoundaryRules: []BoundaryRule{
			{
				ID: "start", Type: "start",
				Actions: []string{"CHECK"}, RequiresIssueType: true,
			},
			{
				ID: "end-resolution", Type: "end",
				Actions: []string{"RESOLVE_ISSUE"},
			},
		},
		BoundaryRuleVersion: "test-rules",
	}
	issues := []DataQualityIssue{}
	instances := BuildScenarioInstancesWithConfig(recordingID, events, &issues, config)
	if len(instances) != 1 {
		t.Fatalf("expected one generic scenario, got %d", len(instances))
	}
	if instances[0].Status != "confirmed" || instances[0].Outcome != "resolved" {
		t.Fatalf("unexpected generic close: %#v", instances[0])
	}
}

func TestConfiguredInactivityRuleKeepsLegacySuccessfulEndGuard(t *testing.T) {
	recordingID := uuid.New()
	runID := uuid.New()
	events := []ActionEvent{
		scenarioAction(runID, 1000, "CHECK", "Custom issue", "101", ""),
		scenarioAction(runID, 602000, "NAVIGATE", "Custom issue", "101", ""),
	}
	config := ScenarioConfig{
		KnownScenarios: []KnownScenario{},
		BoundaryRules: []BoundaryRule{
			{
				ID: "start", Type: "start",
				Actions: []string{"CHECK"}, RequiresIssueType: true,
			},
			{ID: "inactivity", Type: "end", InactivityMS: 600000},
		},
	}
	issues := []DataQualityIssue{}
	instances := BuildScenarioInstancesWithConfig(recordingID, events, &issues, config)
	if len(instances) != 1 || instances[0].Status != "ambiguous" {
		t.Fatalf("legacy success guard must keep inactivity-only end ambiguous: %#v", instances)
	}
}

func TestConfiguredRulesCanDisableEntitySplit(t *testing.T) {
	recordingID := uuid.New()
	runID := uuid.New()
	events := []ActionEvent{
		scenarioAction(runID, 1000, "CHECK", "Custom issue", "101", ""),
		scenarioAction(runID, 2000, "CHECK", "Custom issue", "202", ""),
		scenarioAction(runID, 3000, "RESOLVE_ISSUE", "Custom issue", "202", "Resolved successfully"),
	}
	config := ScenarioConfig{
		KnownScenarios: []KnownScenario{},
		BoundaryRules: []BoundaryRule{
			{
				ID: "start", Type: "start",
				Actions: []string{"CHECK"}, RequiresIssueType: true,
			},
			{
				ID: "end", Type: "end",
				Actions: []string{"RESOLVE_ISSUE"},
			},
		},
	}
	issues := []DataQualityIssue{}
	instances := BuildScenarioInstancesWithConfig(recordingID, events, &issues, config)
	if len(instances) != 1 {
		t.Fatalf("disabled split rule should keep one scenario, got %d", len(instances))
	}
}

func scenarioAction(
	runID uuid.UUID,
	timestampMS int,
	action, issueType, entityID, stateChange string,
) ActionEvent {
	return ActionEvent{
		ID: uuid.New(), AnalysisRunID: runID, TimestampMS: timestampMS,
		CanonicalAction: action, IssueType: issueType,
		EntityID: entityID, OrderID: entityID, StateChange: stateChange,
		Confidence: 0.9,
	}
}
