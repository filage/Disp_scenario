package domain

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestOnlyImplementedScenariosAreExposed(t *testing.T) {
	codes := make([]string, 0, len(KnownScenarios))
	for _, scenario := range KnownScenarios {
		codes = append(codes, scenario.Code)
	}
	expected := []string{
		"LATE_PICKUP", "UNASSIGNED_COURIER", "CHANGE_DELIVERY_DESTINATION",
		"UPDATE_RECIPIENT_CONTACT", "ADD_DELIVERY_NOTE",
	}
	if !reflect.DeepEqual(codes, expected) {
		t.Fatalf("unexpected known scenarios: %#v", codes)
	}
}

func TestCourierConfirmationClickResolvesIssue(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 24000, Screen: "Order Details",
		Target:      "Confirm courier assigned menu item",
		VisibleText: "Confirm courier assigned", EventTypeGuess: "click",
		StateChange: "Take Action menu closes", Confidence: .95,
		Payload: map[string]any{"orderId": "1011", "possibleIssueType": "Unassigned courier"},
	}})
	if got := result.ActionEvents[0].CanonicalAction; got != "RESOLVE_ISSUE" {
		t.Fatalf("expected RESOLVE_ISSUE, got %s", got)
	}
}

func TestFailedCourierConfirmationRemainsAttempt(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 24000, Screen: "Order Details",
		Target:      "Confirm courier assigned menu item",
		VisibleText: "Confirm courier assigned", EventTypeGuess: "click",
		StateChange: "Error: driver assignment is required first", Confidence: .95,
		Payload: map[string]any{"orderId": "1011", "possibleIssueType": "Unassigned courier"},
	}})
	if got := result.ActionEvents[0].CanonicalAction; got != "RESOLUTION_ATTEMPT" {
		t.Fatalf("expected RESOLUTION_ATTEMPT, got %s", got)
	}
}

func TestOrderRowNavigation(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 5300, Screen: "Deliveries", Target: "Row ORD-1022",
		VisibleText: "ORD-1022", EventTypeGuess: "navigation",
		StateChange: "Navigates to Order Details for ORD-1022", Confidence: .95,
		Payload: map[string]any{"orderId": "ORD-1022"},
	}})
	event := result.ActionEvents[0]
	if event.CanonicalAction != "OPEN_ORDER" || event.OrderID != "1022" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestEditViewButtonOpeningDetailsIsOpenOrder(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 10000, Screen: "Deliveries",
		Target:      "Edit/View button for ORD-1022",
		VisibleText: "ORD-1022", EventTypeGuess: "click",
		StateChange: "Opened Order details modal for ORD-1022", Confidence: .95,
		Payload: map[string]any{
			"orderId":           "ORD-1022",
			"possibleIssueType": "Unassigned courier",
			"notes":             "Clicked to open order details for ORD-1022",
		},
	}})
	event := result.ActionEvents[0]
	if event.CanonicalAction != "OPEN_ORDER" || event.OrderID != "1022" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestEditDetailsNavigationToOrderDetailsIsOpenOrder(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 10000, Screen: "Deliveries",
		Target:      "Edit/Details button for ORD-1022",
		VisibleText: "ORD-1022", EventTypeGuess: "click",
		StateChange: "Navigating to Order Details for ORD-1022", Confidence: .95,
		Payload: map[string]any{
			"orderId":           "ORD-1022",
			"possibleIssueType": "Unassigned courier",
			"notes":             "Dispatcher clicks to open the details of order ORD-1022.",
		},
	}})
	event := result.ActionEvents[0]
	if event.CanonicalAction != "OPEN_ORDER" || event.OrderID != "1022" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestSendToSelectedIsDriverAssignmentSubmit(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 15500, Screen: "Send Order to Drivers modal",
		Target:      "Send to Selected (1) button",
		VisibleText: "Send to Selected (1)", EventTypeGuess: "click",
		StateChange: "Sending order to selected driver", Confidence: .95,
		Payload: map[string]any{
			"orderId":           "ORD-1022",
			"possibleIssueType": "Unassigned courier",
		},
	}})
	if got := result.ActionEvents[0].CanonicalAction; got != "SEND_TO_SELECTED_DRIVER" {
		t.Fatalf("expected SEND_TO_SELECTED_DRIVER, got %s", got)
	}
}

func TestCombinedDriverSelectAndSendSplitsAtomicActions(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 16000, Screen: "Send Order to Drivers",
		Target:      "Send to Selected (1) button",
		VisibleText: "Send to Selected (1)", EventTypeGuess: "click",
		StateChange: "Modal closes, returning to Order details with a success toast",
		Confidence:  .95,
		Payload: map[string]any{
			"orderId":           "ORD-1022",
			"possibleIssueType": "Unassigned courier",
			"notes":             "Selected driver Alice Mower and clicked Send to Selected.",
		},
	}})
	if len(result.ActionEvents) != 2 {
		t.Fatalf("expected two action events, got %d: %#v", len(result.ActionEvents), result.ActionEvents)
	}
	if got := result.ActionEvents[0].CanonicalAction; got != "SELECT_DRIVER" {
		t.Fatalf("expected SELECT_DRIVER first, got %s", got)
	}
	if got := result.ActionEvents[1].CanonicalAction; got != "SEND_TO_SELECTED_DRIVER" {
		t.Fatalf("expected SEND_TO_SELECTED_DRIVER second, got %s", got)
	}
	if result.ActionEvents[0].TimestampMS >= result.ActionEvents[1].TimestampMS {
		t.Fatalf("split actions are not ordered: %#v", result.ActionEvents)
	}
	if result.ActionEvents[0].Target != "Alice Mower driver option" {
		t.Fatalf("unexpected driver target: %q", result.ActionEvents[0].Target)
	}
}

func TestCombinedEditAndChangeSplitsAtomicActions(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 23000, Screen: "Order details",
		Target:      "Schedule Edit button",
		VisibleText: "Pickup Window End: 12:30", EventTypeGuess: "click",
		StateChange: "Schedule section switches to inline edit mode and pickup window end changed to 12:30",
		Confidence:  .95,
		Payload: map[string]any{
			"orderId": "ORD-1014",
			"notes":   "Clicked Edit and changed pickup window end time to 12:30.",
		},
	}})
	if len(result.ActionEvents) != 2 {
		t.Fatalf("expected two action events, got %d: %#v", len(result.ActionEvents), result.ActionEvents)
	}
	if got := result.ActionEvents[0].CanonicalAction; got != "OPEN_FIELD_EDITOR" {
		t.Fatalf("expected OPEN_FIELD_EDITOR first, got %s", got)
	}
	if got := result.ActionEvents[1].CanonicalAction; got != "CHANGE_FIELD_VALUE" {
		t.Fatalf("expected CHANGE_FIELD_VALUE second, got %s", got)
	}
}

func TestTakeActionWithResolutionDropdownStaysTakeAction(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 18000, Screen: "Order details",
		Target:      "Take Action button",
		VisibleText: "Take Action", EventTypeGuess: "click",
		StateChange: "Take Action dropdown menu opens",
		Confidence:  .95,
		Payload: map[string]any{
			"orderId": "ORD-1022",
			"notes":   "Clicked the Take Action button to open the resolution dropdown.",
		},
	}})
	if got := result.ActionEvents[0].CanonicalAction; got != "TAKE_ACTION" {
		t.Fatalf("expected TAKE_ACTION, got %s", got)
	}
}

func TestUnknownLowConfidenceInspection(t *testing.T) {
	result := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{{
		TimestampMS: 1000, Screen: "Deliveries", Target: "Red issue badge",
		VisibleText: "Late pickup", EventTypeGuess: "hover",
		ColorCues: []string{"red badge"}, StateChange: "Tooltip 'Late pickup' is displayed",
		Confidence: .55, Payload: map[string]any{"notes": "The row order number is not readable."},
	}})
	event := result.ActionEvents[0]
	if event.CanonicalAction != "INSPECT_ISSUE" {
		t.Fatalf("expected INSPECT_ISSUE, got %s", event.CanonicalAction)
	}
	if !includes(event.QualityFlags, "LOW_CONFIDENCE") || !includes(event.QualityFlags, "UNKNOWN_TARGET") {
		t.Fatalf("missing quality flags: %#v", event.QualityFlags)
	}
}

func TestFailedCourierFlowStaysOpenUntilConfirmed(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	raw := []RawVisionEvent{
		event(3500, "Deliveries", "Red issue-count badge for ORD-1022", "Unassigned courier", "Tooltip appears after hovering the issue badge", "hover", map[string]any{"orderId": "1022", "possibleIssueType": "Unassigned courier"}),
		event(5000, "Deliveries", "Row ORD-1022", "ORD-1022, Attention Required", "Opens Order Details screen for Order #1022", "click", map[string]any{"orderId": "1022", "possibleIssueType": "Unassigned courier"}),
		event(9500, "Order Details", "Confirm courier assigned", "Confirm courier assigned", "Error: driver assignment is required first", "click", map[string]any{"orderId": "1022", "possibleIssueType": "Unassigned courier"}),
		event(15000, "Send Order To Drivers", "Checkbox next to Sam Harper", "Sam Harper, Available", "Driver Sam Harper is selected", "click", map[string]any{"orderId": "1022"}),
		event(19000, "Order Details", "Confirm courier assigned", "Confirm courier assigned", "Order state successfully updated, confirmation appears", "click", map[string]any{"orderId": "1022", "possibleIssueType": "Unassigned courier"}),
	}
	normalized := Normalize(recordingID, runID, raw)
	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	instances := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(instances) != 1 {
		t.Fatalf("expected one instance, got %d", len(instances))
	}
	if instances[0].Outcome != "resolved" || instances[0].EndedAtMS != 19000 {
		t.Fatalf("unexpected instance: %#v", instances[0])
	}
}

func TestMultipleBadgeInspectionsAttachOnlySelectedOrder(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	raw := []RawVisionEvent{
		hoverEvent(1000, "1021", "Late pickup"),
		hoverEvent(2000, "1022", "Unassigned courier"),
		hoverEvent(3000, "1023", "Address mismatch"),
		event(4000, "Deliveries", "Row ORD-1022", "ORD-1022", "Opens Order Details screen for Order #1022", "click", map[string]any{
			"orderId": "1022", "possibleIssueType": "Unassigned courier",
		}),
		event(5000, "Send Order To Drivers", "Checkbox next to Sam Harper", "Sam Harper, Available", "Driver Sam Harper is selected", "click", map[string]any{
			"orderId": "1022", "driverName": "Sam Harper",
		}),
		event(6000, "Order Details", "Confirm courier assigned", "Confirm courier assigned", "Order state successfully updated, toast confirmation appears", "click", map[string]any{
			"orderId": "1022", "possibleIssueType": "Unassigned courier",
		}),
	}
	normalized := Normalize(recordingID, runID, raw)
	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	instances := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(instances) != 1 {
		t.Fatalf("expected one instance, got %d", len(instances))
	}
	instance := instances[0]
	if instance.OrderID != "1022" || instance.StartedAtMS != 4000 || instance.EndedAtMS != 6000 {
		t.Fatalf("unexpected instance: %#v", instance)
	}
	if includesUUID(instance.EventIDs, normalized.ActionEvents[0].ID) ||
		includesUUID(instance.EventIDs, normalized.ActionEvents[1].ID) ||
		includesUUID(instance.EventIDs, normalized.ActionEvents[2].ID) {
		t.Fatalf("wrong inspection attachment: %#v", instance.EventIDs)
	}
}

func TestAttentionRequiredFilterDoesNotStartScenario(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	raw := []RawVisionEvent{
		event(1000, "Deliveries", "Attention Required", "Attention Required", "List filtered to show only orders requiring attention", "click", map[string]any{}),
		hoverEvent(2000, "1022", "Unassigned courier"),
		event(3000, "Deliveries", "Row ORD-1022", "ORD-1022", "Opens Order Details screen for Order #1022", "click", map[string]any{
			"orderId": "1022", "possibleIssueType": "Unassigned courier",
		}),
		event(4000, "Send Order To Drivers", "Checkbox next to Sam Harper", "Sam Harper, Available", "Driver Sam Harper is selected", "click", map[string]any{"orderId": "1022"}),
		event(5000, "Order Details", "Confirm courier assigned", "Confirm courier assigned", "Order state successfully updated, toast confirmation appears", "click", map[string]any{
			"orderId": "1022", "possibleIssueType": "Unassigned courier",
		}),
	}
	normalized := Normalize(recordingID, runID, raw)
	if normalized.ActionEvents[0].CanonicalAction != "FILTER_ISSUES" ||
		normalized.ActionEvents[0].OrderID != "" {
		t.Fatalf("unexpected filter event: %#v", normalized.ActionEvents[0])
	}
	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	instances := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(instances) != 1 || instances[0].StartedAtMS != 3000 ||
		includesUUID(instances[0].EventIDs, normalized.ActionEvents[0].ID) {
		t.Fatalf("filter polluted scenario: %#v", instances)
	}
}

func TestNavigationAfterResolutionDoesNotStartAnotherScenario(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	raw := []RawVisionEvent{
		hoverEvent(1000, "1007", "Late pickup"),
		event(2000, "Deliveries", "Row ORD-1007", "ORD-1007", "Opens Order Details screen for Order #1007", "click", map[string]any{
			"orderId": "1007", "possibleIssueType": "Late pickup",
		}),
		event(3000, "Order Details", "Resolve late pickup", "Resolve late pickup", "Issue successfully resolved and attention badge disappears", "click", map[string]any{
			"orderId": "1007", "possibleIssueType": "Late pickup",
		}),
		event(4000, "Order Details", "Back to Deliveries", "Deliveries", "Returns to Deliveries list", "click", map[string]any{"orderId": "1007"}),
	}
	normalized := Normalize(recordingID, runID, raw)
	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	instances := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(instances) != 1 || instances[0].StartedAtMS != 2000 ||
		instances[0].EndedAtMS != 3000 || instances[0].Outcome != "resolved" {
		t.Fatalf("unexpected instances: %#v", instances)
	}
	if hasIssueType(issues, "MISSING_SCENARIO_END") {
		t.Fatalf("unexpected missing end issue: %#v", issues)
	}
}

func TestKnownScenarioClosesAtResolveEvenWithoutRequiredActions(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	raw := []RawVisionEvent{
		event(1000, "Deliveries", "Row ORD-1022", "ORD-1022, Attention Required", "Opens Order Details screen for Order #1022", "click", map[string]any{
			"orderId": "1022", "possibleIssueType": "Unassigned courier",
		}),
		event(2000, "Order Details", "Confirm courier assigned", "Confirm courier assigned", "Selected Confirm courier assigned option", "click", map[string]any{
			"orderId": "1022", "possibleIssueType": "Unassigned courier",
		}),
	}
	normalized := Normalize(recordingID, runID, raw)
	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	instances := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(instances) != 1 || instances[0].Outcome != "resolved" ||
		instances[0].Status != "confirmed" || hasIssueType(issues, "MISSING_SCENARIO_END") {
		t.Fatalf("scenario did not close at RESOLVE_ISSUE: %#v / %#v", instances, issues)
	}
}

func TestLatePickupClosesWithoutVisualCheckAndIgnoresCancelText(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	first := event(11500, "Deliveries", "Order row ORD-1014", "ORD-1014", "Order Details panel opens for ORD-1014", "click", map[string]any{"orderId": "ORD-1014"})
	first.ColorCues = []string{"red badge"}
	raw := []RawVisionEvent{
		first,
		event(18500, "Order Details", "Schedule Edit button", "Schedule, Edit", "Schedule section switches to inline edit mode", "click", map[string]any{"orderId": "ORD-1014"}),
		event(23500, "Order Details", "Schedule Save button", "Save, Cancel", "Pickup Window updated, success toast shown: 'Completed successfully'", "click", map[string]any{"orderId": "ORD-1014"}),
		event(26000, "Order Details", "Take Action button", "Take Action", "Take Action dropdown menu opens", "click", map[string]any{"orderId": "ORD-1014"}),
		event(27000, "Order Details", "Resolve late pickup option", "Resolve late pickup", "Success toast shown: 'Delivery has been updated.' Warning banner disappears", "click", map[string]any{
			"orderId": "ORD-1014", "possibleIssueType": "Late pickup",
		}),
		event(31000, "Order Details", "Deliveries navigation link", "Deliveries", "Returned to Deliveries list", "click", map[string]any{}),
	}
	normalized := Normalize(recordingID, runID, raw)
	for index := 0; index < 5; index++ {
		if normalized.ActionEvents[index].IssueType != "Late pickup" {
			t.Fatalf("issue type not propagated at %d: %#v", index, normalized.ActionEvents[index])
		}
	}
	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	instances := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(instances) != 1 || instances[0].KnownScenarioCode != "LATE_PICKUP" ||
		instances[0].StartedAtMS != 11500 || instances[0].EndedAtMS != 27000 ||
		instances[0].Outcome != "resolved" || hasIssueType(issues, "MISSING_SCENARIO_END") {
		t.Fatalf("late pickup regression: %#v / %#v", instances, issues)
	}
}

func TestExplicitOrderDetailsTransitionIsOpenOrder(t *testing.T) {
	raw := event(6500, "Deliveries", "ORD-1007", "ORD-1007", "Opens Order Details screen for order #1007", "navigation", map[string]any{"orderId": "1007"})
	raw.ColorCues = []string{"blue row selection"}
	normalized := Normalize(uuid.New(), uuid.New(), []RawVisionEvent{raw})
	if normalized.ActionEvents[0].CanonicalAction != "OPEN_ORDER" ||
		normalized.ActionEvents[0].OrderID != "1007" {
		t.Fatalf("unexpected event: %#v", normalized.ActionEvents[0])
	}
}

func TestResolveMenuTypoOverridesNavigationWording(t *testing.T) {
	recordingID, runID := uuid.New(), uuid.New()
	last := event(
		21000, "Order Details", "Resolve late price menu option", "Resolve late price",
		"Closes the order details panel and returns to the deliveries list, resolving the issue",
		"status_change", map[string]any{"orderId": "1007", "possibleIssueType": "Late pickup"},
	)
	raw := []RawVisionEvent{
		hoverEvent(7000, "1007", "Late pickup"),
		event(9000, "Deliveries", "ORD-1007 row", "ORD-1007", "Opens Order details panel for #1007", "click", map[string]any{"orderId": "1007"}),
		event(20000, "Order Details", "Take Action button", "Take Action", "Opens the Take Action menu", "click", map[string]any{"orderId": "1007"}),
		last,
	}
	normalized := Normalize(recordingID, runID, raw)
	if normalized.ActionEvents[len(normalized.ActionEvents)-1].CanonicalAction != "RESOLVE_ISSUE" {
		t.Fatalf("typo was not recognized: %#v", normalized.ActionEvents)
	}
	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	instances := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(instances) != 1 || instances[0].EndedAtMS != 21000 ||
		instances[0].Outcome != "resolved" || hasIssueType(issues, "MISSING_SCENARIO_END") {
		t.Fatalf("unexpected scenario: %#v / %#v", instances, issues)
	}
}

func event(timestamp int, screen, target, visible, state, eventType string, payload map[string]any) RawVisionEvent {
	return RawVisionEvent{
		TimestampMS: timestamp, Screen: screen, Target: target, VisibleText: visible,
		StateChange: state, EventTypeGuess: eventType, Confidence: .95, Payload: payload,
	}
}

func hoverEvent(timestamp int, orderID, issueType string) RawVisionEvent {
	raw := event(
		timestamp,
		"Deliveries",
		"Red issue-count badge for ORD-"+orderID,
		issueType,
		"Tooltip appears after hovering the issue badge",
		"hover",
		map[string]any{
			"orderId": orderID, "possibleIssueType": issueType,
			"notes": "The badge contains a numeric issue count.",
		},
	)
	raw.ColorCues = []string{"red badge with numeric count"}
	return raw
}

func hasIssueType(issues []DataQualityIssue, expected string) bool {
	for _, issue := range issues {
		if issue.Type == expected {
			return true
		}
	}
	return false
}
