package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const NormalizationVersion = "vision-normalizer-v10"

var orderPattern = regexp.MustCompile(`(?i)\b(?:ORD[-\s#]*)?(\d{3,})\b`)

var eventTypeByAction = map[string]string{
	"OPEN_ORDER":              "entity_open",
	"FILTER_ISSUES":           "filter",
	"INSPECT_ISSUE":           "manual_check",
	"CHECK":                   "manual_check",
	"RESOLUTION_ATTEMPT":      "resolution_attempt",
	"TAKE_ACTION":             "action_menu",
	"OPEN_DRIVER_ASSIGNMENT":  "navigation",
	"SELECT_DRIVER":           "assignment_selection",
	"SEND_TO_SELECTED_DRIVER": "assignment_submit",
	"ASSIGN_DRIVER":           "assignment",
	"MARK_PICKUP_COMPLETED":   "status_change",
	"OPEN_FIELD_EDITOR":       "field_edit_open",
	"CHANGE_FIELD_VALUE":      "field_edit",
	"EDIT_FIELD":              "field_edit",
	"SAVE":                    "save",
	"RESOLVE_ISSUE":           "resolution",
	"NAVIGATE":                "navigation",
}

type NormalizeResult struct {
	ActionEvents      []ActionEvent
	DataQualityIssues []DataQualityIssue
}

type actionPlanItem struct {
	Action            string
	TimestampOffsetMS int
	Target            string
}

func Normalize(recordingID, analysisRunID uuid.UUID, rawEvents []RawVisionEvent) NormalizeResult {
	sorted := append([]RawVisionEvent(nil), rawEvents...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].TimestampMS < sorted[j].TimestampMS
	})

	events := make([]ActionEvent, 0, len(sorted)*2)
	issues := make([]DataQualityIssue, 0)
	var previous *RawVisionEvent

	for index := range sorted {
		raw := sorted[index]
		if raw.ID == uuid.Nil {
			raw.ID = uuid.New()
		}
		entityID := inferEntityID(raw)
		issueType := inferIssueType(raw)
		plan := atomicActionPlan(raw)
		for planIndex, item := range plan {
			flags := qualityFlags(raw, item.Action, previous, entityID)
			payload := cloneMap(raw.Payload)
			payload["visibleText"] = raw.VisibleText
			payload["colorCues"] = raw.ColorCues
			payload["stateChange"] = raw.StateChange
			payload["eventTypeGuess"] = raw.EventTypeGuess
			payload["issueTypeSource"] = nullableString(issueType)
			payload["normalizationVersion"] = NormalizationVersion
			if len(plan) > 1 {
				payload["sourceCombinedAction"] = true
				payload["atomicActionIndex"] = planIndex
				payload["atomicActionCount"] = len(plan)
			}

			target := raw.Target
			if item.Target != "" {
				target = item.Target
			}
			event := ActionEvent{
				ID: uuid.New(), RecordingID: recordingID, AnalysisRunID: analysisRunID,
				TimestampMS: raw.TimestampMS + item.TimestampOffsetMS, CanonicalAction: item.Action,
				EventType: eventTypeByAction[item.Action], Screen: raw.Screen,
				EntityID: entityID, OrderID: entityID, IssueType: issueType,
				Target: target, VisibleText: raw.VisibleText, StateChange: raw.StateChange,
				Confidence: clamp(raw.Confidence, 0, 1), SourceRawEventIDs: []uuid.UUID{raw.ID},
				QualityFlags: flags, Payload: payload, Source: "gemini",
			}
			if entityID != "" {
				event.EntityType = "order"
			}
			events = append(events, event)
			if planIndex == 0 {
				for _, flag := range flags {
					if flag == "ACTION_FAILED" {
						continue
					}
					rawID := raw.ID
					issues = append(issues, DataQualityIssue{
						ID: uuid.New(), RecordingID: recordingID, AnalysisRunID: analysisRunID,
						RawVisionEventID: &rawID, Type: flag, Severity: severity(flag),
						Message: issueMessage(flag), TimestampMS: raw.TimestampMS,
						Payload: map[string]any{"confidence": raw.Confidence, "target": raw.Target},
					})
				}
			}
		}
		previous = &raw
	}

	propagateIssueTypes(events)
	return NormalizeResult{ActionEvents: events, DataQualityIssues: issues}
}

func atomicActionPlan(raw RawVisionEvent) []actionPlanItem {
	action := canonicalize(raw)
	source := actionSource(raw)
	if combinedDriverSelectionSubmitSignal(raw, source) {
		return []actionPlanItem{
			{Action: "SELECT_DRIVER", Target: inferDriverTarget(raw)},
			{Action: "SEND_TO_SELECTED_DRIVER", TimestampOffsetMS: 1},
		}
	}
	if combinedFieldOpenChangeSignal(raw, source) {
		return []actionPlanItem{
			{Action: "OPEN_FIELD_EDITOR"},
			{Action: "CHANGE_FIELD_VALUE", TimestampOffsetMS: 1},
		}
	}
	return []actionPlanItem{{Action: action}}
}

func canonicalize(raw RawVisionEvent) string {
	target := strings.ToLower(raw.Target)
	source := canonicalActionSource(raw)

	switch {
	case issueFilterSignal(source):
		return "FILTER_ISSUES"
	case orderDetailsOpenSignal(raw):
		return "OPEN_ORDER"
	case failedActionSignal(source) && resolutionAttemptSignal(source):
		return "RESOLUTION_ATTEMPT"
	case resolutionClickSignal(source):
		return "RESOLVE_ISSUE"
	case explicitSuccessfulResolution(target, source):
		return "RESOLVE_ISSUE"
	case navigationSignal(target) || navigationSignal(source):
		return "NAVIGATE"
	case issueInspectionSignal(source):
		return "INSPECT_ISSUE"
	case failedActionSignal(source):
		return "CHECK"
	case containsAny(target, "take action", "action dropdown", "action options"):
		return "TAKE_ACTION"
	case containsAny(source, "send to selected"):
		return "SEND_TO_SELECTED_DRIVER"
	case containsAny(source,
		"driver checkbox", "checkbox next to", "driver selected",
		"driver is selected", "selected driver"):
		return "SELECT_DRIVER"
	case strings.Contains(target, "send order") ||
		strings.Contains(source, "send order to drivers") && strings.Contains(source, "modal") ||
		containsAny(source, "add driver", "driver helper"):
		return "OPEN_DRIVER_ASSIGNMENT"
	case containsAny(source, "driver assigned", "assigns order"):
		return "SEND_TO_SELECTED_DRIVER"
	case successfulResolutionSignal(source):
		return "RESOLVE_ISSUE"
	case containsAny(target, "resolve late pickup", "mark issue resolved"):
		return "RESOLVE_ISSUE"
	case containsAny(source, "save", "saved", "successfully updated", "schedule updated"):
		return "SAVE"
	case opensFieldEditorSignal(target, source):
		return "OPEN_FIELD_EDITOR"
	case changesFieldValueSignal(source):
		return "CHANGE_FIELD_VALUE"
	case containsOrder(source) && containsAny(source, "open", "opens", "navigated", "details"):
		return "OPEN_ORDER"
	case containsAny(source, "take action", "action dropdown", "action options"):
		return "TAKE_ACTION"
	case containsOrder(source) && containsAny(source, "open", "opens", "order id", "row ord"):
		return "OPEN_ORDER"
	case resolutionAttemptSignal(source):
		return "RESOLVE_ISSUE"
	case opensFieldEditorSignal(target, source):
		return "OPEN_FIELD_EDITOR"
	case changesFieldValueSignal(source):
		return "CHANGE_FIELD_VALUE"
	case containsAny(source, "edit", "field", "window", "input"):
		return "CHANGE_FIELD_VALUE"
	case containsAny(source, "save", "confirm"):
		return "SAVE"
	case containsAny(source, "navigate", "tab", "menu", "list", "route"):
		return "NAVIGATE"
	default:
		return "CHECK"
	}
}

func canonicalActionSource(raw RawVisionEvent) string {
	return strings.ToLower(strings.Join([]string{
		raw.EventTypeGuess,
		raw.Target,
		raw.VisibleText,
		raw.StateChange,
		raw.Screen,
	}, " "))
}

func actionSource(raw RawVisionEvent) string {
	return strings.ToLower(strings.Join([]string{
		raw.EventTypeGuess,
		raw.Target,
		raw.VisibleText,
		raw.StateChange,
		raw.Screen,
		payloadString(raw.Payload, "notes"),
		payloadString(raw.Payload, "driverName"),
		payloadString(raw.Payload, "fieldName"),
		payloadString(raw.Payload, "previousValue"),
		payloadString(raw.Payload, "newValue"),
	}, " "))
}

func combinedDriverSelectionSubmitSignal(raw RawVisionEvent, source string) bool {
	if !containsAny(source, "send to selected") {
		return false
	}
	notes := strings.ToLower(payloadString(raw.Payload, "notes"))
	return containsAny(notes,
		"selected driver", "selects driver", "selected the driver", "driver is selected",
		"driver was selected", "checked driver", "checkbox next to") ||
		containsAny(source, "driver checkbox", "checkbox next to", "driver is selected", "driver was selected")
}

func combinedFieldOpenChangeSignal(raw RawVisionEvent, source string) bool {
	notes := strings.ToLower(payloadString(raw.Payload, "notes"))
	explicitCombined := containsAny(notes,
		"clicked edit and changed", "clicks edit and changes",
		"opened editor and changed", "opens editor and changes",
		"edit button and changed", "edit button and entered")
	return explicitCombined ||
		opensFieldEditorSignal(strings.ToLower(raw.Target), source) &&
			changesFieldValueSignal(source) &&
			containsAny(source, "clicked edit", "clicks edit", "edit button", "opens edit", "opened editor")
}

func inferDriverTarget(raw RawVisionEvent) string {
	if driverName := payloadString(raw.Payload, "driverName"); driverName != "" {
		return driverName + " driver option"
	}
	notes := payloadString(raw.Payload, "notes")
	lowerNotes := strings.ToLower(notes)
	marker := "selected driver "
	if index := strings.Index(lowerNotes, marker); index >= 0 {
		start := index + len(marker)
		end := start
		for end < len(notes) {
			char := notes[end]
			if char == ',' || char == '.' || char == ';' {
				break
			}
			if strings.HasPrefix(strings.ToLower(notes[end:]), " and ") {
				break
			}
			end++
		}
		if name := strings.TrimSpace(notes[start:end]); name != "" {
			return name + " driver option"
		}
	}
	return "Selected driver option"
}

func opensFieldEditorSignal(target, source string) bool {
	return containsAny(target, "edit") ||
		containsAny(source, "edit button", "schedule edit", "switches to inline edit mode",
			"editable mode", "edit mode", "opens edit", "field editor")
}

func changesFieldValueSignal(source string) bool {
	return containsAny(source, "input", "changed", "change", "entered", "typed",
		"pickup window end", "pickup window start", "dropoff window",
		"new time", "time value", "field value")
}

func inferIssueType(raw RawVisionEvent) string {
	for _, key := range []string{"possibleIssueType", "issueType"} {
		if value, ok := raw.Payload[key].(string); ok {
			if normalized := normalizeIssueType(value); normalized != "" {
				return normalized
			}
		}
	}

	source := strings.ToLower(strings.Join([]string{raw.Target, raw.VisibleText, raw.StateChange}, " "))
	if navigationSignal(source) {
		return ""
	}
	if payloadString(raw.Payload, "orderId") == "" &&
		containsAny(source, "attention required tab", "filters updated", "filtered to attention") {
		return ""
	}
	if containsAny(source, "late pickup", "overdue") || latePickupSignal(source) {
		return "Late pickup"
	}
	if containsAny(source, "unassigned", "courier", "driver", "to drivers", "send order") {
		return "Unassigned courier"
	}
	return ""
}

func inferEntityID(raw RawVisionEvent) string {
	source := strings.Join([]string{
		payloadString(raw.Payload, "orderId"),
		payloadString(raw.Payload, "entityId"),
		raw.Target, raw.VisibleText, raw.Screen,
	}, " ")
	match := orderPattern.FindStringSubmatch(source)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func qualityFlags(raw RawVisionEvent, action string, previous *RawVisionEvent, entityID string) []string {
	flags := make([]string, 0)
	source := strings.ToLower(raw.StateChange + " " + raw.VisibleText + " " + payloadString(raw.Payload, "notes"))
	if raw.Confidence < 0.65 {
		flags = append(flags, "LOW_CONFIDENCE")
	}
	if raw.Target == "" && raw.VisibleText == "" || action == "INSPECT_ISSUE" && entityID == "" {
		flags = append(flags, "UNKNOWN_TARGET")
	}
	if previous != nil && raw.TimestampMS < previous.TimestampMS {
		flags = append(flags, "OUT_OF_ORDER_TIMESTAMP")
	}
	if previous != nil && abs(raw.TimestampMS-previous.TimestampMS) < 650 && action == canonicalize(*previous) {
		flags = append(flags, "DUPLICATE_ACTION")
	}
	if containsAny(strings.ToLower(raw.StateChange+" "+raw.VisibleText),
		"ambiguous", "uncertain", "unclear", "partially") {
		flags = append(flags, "AMBIGUOUS_BOUNDARY")
	}
	if failedActionSignal(source) {
		flags = append(flags, "ACTION_FAILED")
	}
	return flags
}

func propagateIssueTypes(events []ActionEvent) {
	explicit := map[string][]int{}
	for index := range events {
		if events[index].EntityID != "" && events[index].IssueType != "" {
			explicit[events[index].EntityID] = append(explicit[events[index].EntityID], index)
		}
	}
	for index := range events {
		event := &events[index]
		if event.EntityID == "" || event.IssueType != "" ||
			event.CanonicalAction == "NAVIGATE" || event.CanonicalAction == "FILTER_ISSUES" {
			continue
		}
		bestDistance := 60001
		bestIssue := ""
		for _, candidateIndex := range explicit[event.EntityID] {
			candidate := events[candidateIndex]
			distance := abs(candidate.TimestampMS - event.TimestampMS)
			if distance <= 60000 && distance < bestDistance {
				bestDistance = distance
				bestIssue = candidate.IssueType
			}
		}
		event.IssueType = bestIssue
	}
}

func orderDetailsOpenSignal(raw RawVisionEvent) bool {
	source := strings.ToLower(strings.Join([]string{
		raw.Target, raw.VisibleText, raw.StateChange,
		payloadString(raw.Payload, "notes"),
		payloadString(raw.Payload, "orderIdEvidence"),
	}, " "))
	state := strings.ToLower(raw.StateChange)
	hasOrder := containsOrder(source) || payloadString(raw.Payload, "orderId") != ""
	opensDetails := containsAny(state, "opens order details", "opens the order details",
		"opened order details", "opened the order details",
		"navigates to order details", "navigating to order details",
		"details screen opens", "details panel opens",
		"order details panel opens", "order details screen", "order details modal",
		"opened order details modal") ||
		containsAny(source, "clicks to open the details", "clicked to open the details",
			"click to open order details", "open the details of order")
	return hasOrder && opensDetails
}

func issueFilterSignal(source string) bool {
	hasFilter := containsAny(source, "attention required", "issues filter", "problem orders")
	hasResult := containsAny(source, "filter", "filtered", "selected", "active state", "show only")
	opensOrder := containsAny(source, "opens order", "order details", "row ord", "order row")
	return hasFilter && hasResult && !opensOrder
}

func issueInspectionSignal(source string) bool {
	return containsAny(source, "hover", "mouse over", "tooltip", "inspect", "inspection") &&
		containsAny(source, "badge", "attention", "warning", "issue", "problem", "unassigned courier", "late pickup")
}

func navigationSignal(source string) bool {
	return containsAny(source, "navigation", "breadcrumb", "back arrow", "back to deliveries",
		"returns to", "returned to", "deliveries menu", "deliveries list", "route")
}

func resolutionAttemptSignal(source string) bool {
	return containsAny(source, "confirm courier assigned", "resolve late pickup",
		"mark pickup completed", "resolve", "resolution")
}

func resolutionClickSignal(source string) bool {
	return resolutionAttemptSignal(source) && !failedActionSignal(source)
}

func explicitSuccessfulResolution(target, source string) bool {
	explicit := strings.HasPrefix(target, "resolve") || strings.Contains(target, "confirm courier assigned")
	return explicit && !failedActionSignal(source) &&
		containsAny(source, "success", "successfully", "updated", "resolved", "issue disappears",
			"warning disappears", "panel closes", "returns to deliveries list", "resolving the issue")
}

func successfulResolutionSignal(source string) bool {
	explicit := containsAny(source, "resolve", "resolved", "confirm courier assigned", "issue resolved")
	success := containsAny(source, "success", "successfully", "updated", "confirmation appears",
		"confirmed", "badge disappears", "attention disappears", "green status", "resolved state")
	return explicit && success && !failedActionSignal(source)
}

func failedActionSignal(source string) bool {
	return containsAny(source, "error", "failed", "failure", "required first", "requires driver",
		"cannot", "can't", "unable", "blocked", "validation")
}

func latePickupSignal(source string) bool {
	return containsAny(source, "pickup", "window") &&
		containsAny(source, "late pickup", "overdue", "late", "attention required", "warning", "red", "orange", "amber")
}

func normalizeIssueType(value string) string {
	value = strings.ToLower(value)
	if containsAny(value, "late pickup", "overdue", "late") {
		return "Late pickup"
	}
	if containsAny(value, "unassigned", "courier", "driver") {
		return "Unassigned courier"
	}
	return ""
}

func containsOrder(source string) bool {
	return orderPattern.MatchString(source) ||
		containsAny(source, "order id", "row ord", "order row")
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch current := value.(type) {
	case string:
		return current
	case float64:
		return strconv.FormatInt(int64(current), 10)
	default:
		return fmt.Sprint(current)
	}
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input)+6)
	for key, value := range input {
		output[key] = value
	}
	return output
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func containsAny(source string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(source, value) {
			return true
		}
	}
	return false
}

func severity(flag string) string {
	if flag == "LOW_CONFIDENCE" || flag == "UNKNOWN_TARGET" {
		return "warning"
	}
	return "info"
}

func issueMessage(flag string) string {
	return map[string]string{
		"LOW_CONFIDENCE":         "Низкая уверенность raw-наблюдения",
		"UNKNOWN_TARGET":         "Неизвестная цель действия",
		"OUT_OF_ORDER_TIMESTAMP": "Событие пришло не по порядку timestamp",
		"DUPLICATE_ACTION":       "Возможный повтор действия",
		"AMBIGUOUS_BOUNDARY":     "Неоднозначная граница действия или сценария",
	}[flag]
}

func clamp(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
