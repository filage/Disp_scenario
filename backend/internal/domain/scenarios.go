package domain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const BoundaryRuleVersion = "boundary-rules-v3"

type KnownScenario struct {
	Code            string
	Name            string
	IssueType       string
	EntityType      string
	RequiredActions []string
	EndActions      []string
	TimeoutMS       int
}

type BoundaryRule struct {
	ID                       string
	Priority                 int
	Type                     string
	Actions                  []string
	RequiresIssueType        bool
	RequiresPriorActions     []string
	EntityChange             bool
	RequireDifferentEntityID bool
	InactivityMS             int
	Version                  string
}

type ScenarioConfig struct {
	KnownScenarios      []KnownScenario
	BoundaryRules       []BoundaryRule
	SplitOnEntityChange bool
	BoundaryRuleVersion string
}

var KnownScenarios = []KnownScenario{
	{
		Code: "LATE_PICKUP", Name: "Опоздание на забор", IssueType: "Late pickup",
		EntityType: "order", RequiredActions: []string{"CHECK"},
		EndActions: []string{"RESOLVE_ISSUE"}, TimeoutMS: 20 * 60 * 1000,
	},
	{
		Code: "UNASSIGNED_COURIER", Name: "Курьер не назначен", IssueType: "Unassigned courier",
		EntityType: "order", RequiredActions: []string{"SEND_TO_SELECTED_DRIVER"},
		EndActions: []string{"RESOLVE_ISSUE"}, TimeoutMS: 20 * 60 * 1000,
	},
	{
		Code: "CHANGE_DELIVERY_DESTINATION", Name: "Смена точки окончания доставки",
		IssueType: "Delivery destination change", EntityType: "order",
		RequiredActions: []string{"CHANGE_FIELD_VALUE"}, EndActions: []string{"SAVE"},
		TimeoutMS: 20 * 60 * 1000,
	},
	{
		Code: "UPDATE_RECIPIENT_CONTACT", Name: "Обновление контакта получателя",
		IssueType: "Recipient contact update", EntityType: "order",
		RequiredActions: []string{"CHANGE_FIELD_VALUE"}, EndActions: []string{"SAVE"},
		TimeoutMS: 20 * 60 * 1000,
	},
	{
		Code: "ADD_DELIVERY_NOTE", Name: "Добавление комментария к доставке",
		IssueType: "Delivery note update", EntityType: "order",
		RequiredActions: []string{"CHANGE_FIELD_VALUE"}, EndActions: []string{"SAVE"},
		TimeoutMS: 20 * 60 * 1000,
	},
}

func BuildScenarioInstances(
	recordingID uuid.UUID,
	events []ActionEvent,
	issues *[]DataQualityIssue,
) []ScenarioInstance {
	return BuildScenarioInstancesWithConfig(recordingID, events, issues, ScenarioConfig{
		KnownScenarios: KnownScenarios, SplitOnEntityChange: true,
		BoundaryRuleVersion: BoundaryRuleVersion,
	})
}

func BuildScenarioInstancesWithConfig(
	recordingID uuid.UUID,
	events []ActionEvent,
	issues *[]DataQualityIssue,
	config ScenarioConfig,
) []ScenarioInstance {
	if config.KnownScenarios == nil {
		config.KnownScenarios = KnownScenarios
	}
	if config.BoundaryRuleVersion == "" {
		config.BoundaryRuleVersion = BoundaryRuleVersion
	}
	sorted := append([]ActionEvent(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TimestampMS < sorted[j].TimestampMS })

	instances := make([]ScenarioInstance, 0)
	pendingInspections := map[string]ActionEvent{}
	pendingOpenOrders := map[string]ActionEvent{}
	var active *activeScenario

	for _, event := range sorted {
		if event.CanonicalAction == "FILTER_ISSUES" {
			continue
		}
		if event.CanonicalAction == "INSPECT_ISSUE" {
			if event.EntityID != "" {
				pendingInspections[event.EntityID] = event
			}
			if active != nil && event.EntityID != "" && active.entityID == event.EntityID {
				active.events = append(active.events, event)
			}
			continue
		}

		known := findKnownScenario(event, config.KnownScenarios)
		if active == nil && known == nil && event.CanonicalAction == "OPEN_ORDER" && event.EntityID != "" {
			pendingOpenOrders[event.EntityID] = event
		}
		var previous *ActionEvent
		if active != nil && len(active.events) > 0 {
			previous = &active.events[len(active.events)-1]
		}
		split := config.SplitOnEntityChange && shouldSplit(previous, &event)
		if len(config.BoundaryRules) > 0 {
			split = shouldSplitWithRules(previous, &event, config.BoundaryRules)
		}
		if active != nil && split {
			instances = append(instances, closeScenario(recordingID, *active, *previous, "interrupted", "ambiguous"))
			active = nil
		}

		start := isStartEventWithConfig(event, config)
		if known != nil {
			// Problem scenarios start at the order opening. Routine scenarios are
			// classified by the first specific edit/assignment action and include a
			// preceding order opening when it was observed.
			start = !isStrictOrderScenario(known.Code) || event.CanonicalAction == "OPEN_ORDER"
		}
		if active == nil && !start {
			continue
		}
		if active == nil {
			active = &activeScenario{
				id: uuid.New(), known: known, issueType: event.IssueType,
				entityID: event.EntityID, startedAtMS: event.TimestampMS,
				boundaryRuleVersion: config.BoundaryRuleVersion,
			}
			if known != nil && active.issueType == "" {
				active.issueType = known.IssueType
			}
			if known != nil && !isStrictOrderScenario(known.Code) {
				if opened, ok := pendingOpenOrders[event.EntityID]; ok && opened.ID != event.ID {
					active.events = append(active.events, opened)
					active.startedAtMS = opened.TimestampMS
					delete(pendingOpenOrders, event.EntityID)
				}
			}
			if inspection, ok := pendingInspections[event.EntityID]; ok && event.EntityID != "" {
				if known == nil {
					active.events = append(active.events, inspection)
					active.startedAtMS = inspection.TimestampMS
				}
				delete(pendingInspections, event.EntityID)
			}
		}
		active.events = append(active.events, event)

		timeout := 15 * 60 * 1000
		if active.known != nil {
			timeout = active.known.TimeoutMS
		}
		elapsed := event.TimestampMS - active.startedAtMS
		knownEnd := active.known != nil && shouldEndKnown(*active.known, event, active.events)
		genericEnd := active.known == nil &&
			shouldEndGeneric(active.events, event, config.BoundaryRules) &&
			successfulScenarioEnd(event)
		if knownEnd || genericEnd || elapsed >= timeout {
			outcome, status := inferOutcome(active.events), "confirmed"
			if knownEnd && active.known != nil && isStrictOrderScenario(active.known.Code) {
				outcome = "resolved"
			} else if knownEnd && active.known != nil {
				outcome = "completed"
			}
			if elapsed >= timeout && !knownEnd && !genericEnd {
				outcome, status = "unresolved", "ambiguous"
			}
			instances = append(instances, closeScenario(recordingID, *active, event, outcome, status))
			active = nil
		}
	}

	if active != nil && len(active.events) > 0 {
		last := active.events[len(active.events)-1]
		instance := closeScenario(recordingID, *active, last, "interrupted", "ambiguous")
		instances = append(instances, instance)
		eventID := last.ID
		*issues = append(*issues, DataQualityIssue{
			ID: uuid.New(), RecordingID: recordingID, AnalysisRunID: last.AnalysisRunID,
			ActionEventID: &eventID, Type: "MISSING_SCENARIO_END", Severity: "warning",
			Message: "Не найдено действие завершения сценария", TimestampMS: last.TimestampMS,
			Payload: map[string]any{
				"boundaryRuleVersion": BoundaryRuleVersion,
				"knownScenarioCode":   instance.KnownScenarioCode,
			},
		})
	}

	return instances
}

type activeScenario struct {
	id                  uuid.UUID
	known               *KnownScenario
	issueType           string
	entityID            string
	events              []ActionEvent
	startedAtMS         int
	boundaryRuleVersion string
}

func findKnownScenario(event ActionEvent, knownScenarios []KnownScenario) *KnownScenario {
	source := strings.ToLower(event.Target + " " + event.Screen + " " + event.StateChange + " " + event.VisibleText)
	for index := range knownScenarios {
		scenario := &knownScenarios[index]
		if scenario.IssueType != "" && event.IssueType == scenario.IssueType {
			return scenario
		}
		if scenario.Code == "LATE_PICKUP" && latePickupSignal(source) {
			return scenario
		}
		if scenario.Code == "UNASSIGNED_COURIER" &&
			containsAny(source, "unassigned", "no courier", "no driver", "to drivers", "send order") {
			return scenario
		}
		if scenario.Code == "CHANGE_DELIVERY_DESTINATION" && destinationChangeSignal(source) {
			return scenario
		}
		if scenario.Code == "UPDATE_RECIPIENT_CONTACT" && recipientContactUpdateSignal(source) {
			return scenario
		}
		if scenario.Code == "ADD_DELIVERY_NOTE" && deliveryNoteUpdateSignal(source) {
			return scenario
		}
	}
	return nil
}

func isStartEvent(event ActionEvent) bool {
	return event.IssueType != "" &&
		(event.CanonicalAction == "OPEN_ORDER" || event.CanonicalAction == "TAKE_ACTION" || event.CanonicalAction == "CHECK")
}

func isStartEventWithConfig(event ActionEvent, config ScenarioConfig) bool {
	if len(config.BoundaryRules) == 0 {
		return isStartEvent(event)
	}
	for _, rule := range config.BoundaryRules {
		if rule.Type != "start" {
			continue
		}
		if len(rule.Actions) > 0 && !includes(rule.Actions, event.CanonicalAction) {
			continue
		}
		if rule.RequiresIssueType && event.IssueType == "" {
			continue
		}
		return true
	}
	return false
}

func shouldSplit(previous, current *ActionEvent) bool {
	return previous != nil && current != nil &&
		previous.EntityID != "" && current.EntityID != "" && previous.EntityID != current.EntityID
}

func shouldSplitWithRules(
	previous, current *ActionEvent,
	rules []BoundaryRule,
) bool {
	if previous == nil || current == nil {
		return false
	}
	for _, rule := range rules {
		if rule.Type != "split" && rule.Type != "entity_change" {
			continue
		}
		if rule.EntityChange || rule.RequireDifferentEntityID {
			return shouldSplit(previous, current)
		}
	}
	return false
}

func shouldEndGeneric(
	events []ActionEvent,
	current ActionEvent,
	rules []BoundaryRule,
) bool {
	for _, rule := range rules {
		if rule.Type != "end" {
			continue
		}
		if len(rule.Actions) > 0 && !includes(rule.Actions, current.CanonicalAction) {
			continue
		}
		if len(rule.RequiresPriorActions) > 0 {
			for _, required := range rule.RequiresPriorActions {
				for _, event := range events {
					if event.CanonicalAction == required {
						return true
					}
				}
			}
			continue
		}
		if rule.InactivityMS > 0 {
			if len(events) < 2 {
				continue
			}
			previous := events[len(events)-2]
			if current.TimestampMS-previous.TimestampMS > rule.InactivityMS {
				return true
			}
			continue
		}
		return true
	}
	return false
}

func shouldEndKnown(scenario KnownScenario, current ActionEvent, events []ActionEvent) bool {
	if isStrictOrderScenario(scenario.Code) {
		return current.CanonicalAction == "RESOLVE_ISSUE" && !failedScenarioEnd(current)
	}
	if !includes(scenario.EndActions, current.CanonicalAction) || failedScenarioEnd(current) {
		return false
	}
	completed := map[string]bool{}
	for _, event := range events {
		if !includes(event.QualityFlags, "ACTION_FAILED") {
			completed[event.CanonicalAction] = true
		}
	}
	for _, required := range scenario.RequiredActions {
		done := completed[required] || required == "CHECK" && completed["INSPECT_ISSUE"]
		if !done {
			if required != "CHECK" {
				return false
			}
		}
	}
	return true
}

func destinationChangeSignal(source string) bool {
	return containsAny(source, "delivery destination", "delivery address", "dropoff address", "drop-off address", "destination point") &&
		containsAny(source, "edit", "change", "changed", "new", "save", "saved")
}

func recipientContactUpdateSignal(source string) bool {
	return containsAny(source, "recipient contact", "recipient phone", "customer contact", "customer phone", "phone number") &&
		containsAny(source, "edit", "change", "changed", "new", "save", "saved")
}

func deliveryNoteUpdateSignal(source string) bool {
	return containsAny(source, "delivery note", "delivery comment", "delivery instructions", "courier note", "order comment") &&
		containsAny(source, "add", "edit", "change", "changed", "new", "save", "saved")
}

func isStrictOrderScenario(code string) bool {
	return code == "LATE_PICKUP" || code == "UNASSIGNED_COURIER"
}

func failedScenarioEnd(event ActionEvent) bool {
	source := strings.ToLower(event.Target + " " + event.StateChange + " " + event.VisibleText)
	return includes(event.QualityFlags, "ACTION_FAILED") || failedActionSignal(source)
}

func successfulScenarioEnd(event ActionEvent) bool {
	if event.CanonicalAction != "RESOLVE_ISSUE" && event.CanonicalAction != "MARK_PICKUP_COMPLETED" {
		return false
	}
	source := strings.ToLower(event.Target + " " + event.StateChange + " " + event.VisibleText + " " + strings.Join(stringSlice(event.Payload["colorCues"]), " "))
	if includes(event.QualityFlags, "ACTION_FAILED") || failedActionSignal(source) {
		return false
	}
	if event.CanonicalAction == "MARK_PICKUP_COMPLETED" {
		return true
	}
	return containsAny(source, "success", "successfully", "updated", "confirmation appears",
		"confirmed", "badge disappears", "warning disappears", "green status", "resolved state", "resolving the issue")
}

func closeScenario(
	recordingID uuid.UUID,
	active activeScenario,
	last ActionEvent,
	outcome, status string,
) ScenarioInstance {
	started := active.startedAtMS
	if len(active.events) > 0 {
		started = active.events[0].TimestampMS
	}
	eventIDs := make([]uuid.UUID, 0, len(active.events))
	flagsSet := map[string]bool{}
	confidence := 0.0
	for _, event := range active.events {
		eventIDs = append(eventIDs, event.ID)
		confidence += event.Confidence
		for _, flag := range event.QualityFlags {
			flagsSet[flag] = true
		}
	}
	if len(active.events) > 0 {
		confidence /= float64(len(active.events))
	}
	flags := mapKeys(flagsSet)
	code, entityType := "", ""
	if active.known != nil {
		code, entityType = active.known.Code, active.known.EntityType
	}
	return ScenarioInstance{
		ID: active.id, RecordingID: recordingID, AnalysisRunID: last.AnalysisRunID,
		KnownScenarioCode: code, OrderID: active.entityID, EntityID: active.entityID,
		EntityType: entityType, IssueType: defaultString(active.issueType, "Unknown"),
		StartedAtMS: started, EndedAtMS: last.TimestampMS,
		DurationMS: max(0, last.TimestampMS-started), EventIDs: eventIDs,
		Outcome: outcome, Status: status, Confidence: round(confidence, 2),
		BoundaryRuleVersion: active.boundaryRuleVersion, QualityFlags: flags,
	}
}

func inferOutcome(events []ActionEvent) string {
	for _, event := range events {
		if successfulScenarioEnd(event) {
			return "resolved"
		}
	}
	for _, event := range events {
		if event.CanonicalAction == "SAVE" || event.CanonicalAction == "TAKE_ACTION" {
			return "pending_confirmation"
		}
	}
	return "unresolved"
}

func stringSlice(value any) []string {
	switch current := value.(type) {
	case []string:
		return current
	case []any:
		result := make([]string, 0, len(current))
		for _, item := range current {
			result = append(result, strings.TrimSpace(toString(item)))
		}
		return result
	default:
		return nil
	}
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func includes(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
