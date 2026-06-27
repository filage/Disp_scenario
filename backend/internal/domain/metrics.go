package domain

import (
	"sort"
	"strings"

	"github.com/google/uuid"
)

var manualCheckActions = map[string]bool{"CHECK": true, "INSPECT_ISSUE": true}
var manualWorkActions = map[string]bool{
	"CHECK": true, "INSPECT_ISSUE": true, "RESOLUTION_ATTEMPT": true,
	"SELECT_DRIVER": true, "SEND_TO_SELECTED_DRIVER": true,
	"OPEN_FIELD_EDITOR": true, "CHANGE_FIELD_VALUE": true,
	"EDIT_FIELD": true, "SAVE": true, "TAKE_ACTION": true,
}

func CalculateMetrics(instances []ScenarioInstance, events []ActionEvent) ScenarioMetrics {
	durations := make([]int, 0, len(instances))
	frequency := map[string]int{}
	repeated := map[string]int{}
	manualChecks, manualActions, interrupted, ambiguous := 0, 0, 0, 0
	confidence := 0.0

	byID := map[uuid.UUID]ActionEvent{}
	for _, event := range events {
		byID[event.ID] = event
		frequency[event.CanonicalAction]++
		confidence += event.Confidence
		if manualCheckActions[event.CanonicalAction] {
			manualChecks++
		}
		if manualWorkActions[event.CanonicalAction] {
			manualActions++
		}
	}
	for _, instance := range instances {
		durations = append(durations, instance.DurationMS)
		if instance.Outcome == "interrupted" {
			interrupted++
		}
		if instance.Status == "ambiguous" || instance.Outcome == "interrupted" {
			ambiguous++
		}
		instanceEvents := eventsForInstance(instance, byID)
		for index := 1; index < len(instanceEvents); index++ {
			if instanceEvents[index].CanonicalAction == instanceEvents[index-1].CanonicalAction &&
				targetKey(instanceEvents[index]) == targetKey(instanceEvents[index-1]) {
				repeated[instanceEvents[index].CanonicalAction]++
			}
		}
	}
	repeatedCount := 0
	for _, count := range repeated {
		repeatedCount += count
	}
	actionStats := buildActionStats(instances, byID, repeated)
	pairStats := buildPairStats(instances, byID)

	return ScenarioMetrics{
		AverageDurationMS: averageInt(durations),
		MedianDurationMS:  percentile(durations, .5),
		P95DurationMS:     percentile(durations, .95),
		ActionFrequency:   frequency, ActionStats: actionStats, PairStats: pairStats,
		RepeatedActions:     repeated,
		RepeatedActionCount: repeatedCount, ManualCheckCount: manualChecks,
		ManualActionCount:               manualActions,
		AverageManualChecksPerInstance:  round(float64(manualChecks)/float64(max(1, len(instances))), 2),
		AverageManualActionsPerInstance: round(float64(manualActions)/float64(max(1, len(instances))), 2),
		ConfidenceAverage:               round(confidence/float64(max(1, len(events))), 2),
		AmbiguousCount:                  ambiguous, InterruptedCount: interrupted,
	}
}

func buildActionStats(
	instances []ScenarioInstance,
	byID map[uuid.UUID]ActionEvent,
	repeated map[string]int,
) []ActionStat {
	type state struct {
		action      string
		count       int
		instanceIDs map[uuid.UUID]bool
		durations   []int
		confidences []float64
		examples    []string
	}
	states := map[string]*state{}
	for _, instance := range instances {
		instanceEvents := eventsForInstance(instance, byID)
		for index, event := range instanceEvents {
			action := event.CanonicalAction
			if action == "" {
				action = event.EventType
			}
			if action == "" {
				action = "UNKNOWN"
			}
			current := states[action]
			if current == nil {
				current = &state{action: action, instanceIDs: map[uuid.UUID]bool{}}
				states[action] = current
			}
			current.count++
			current.instanceIDs[instance.ID] = true
			current.confidences = append(current.confidences, event.Confidence)
			if index+1 < len(instanceEvents) {
				duration := instanceEvents[index+1].TimestampMS - event.TimestampMS
				current.durations = append(current.durations, min(max(0, duration), 180000))
			}
			example := event.Target
			if example == "" {
				example = event.VisibleText
			}
			if example == "" {
				example = action
			}
			if len(current.examples) < 3 {
				current.examples = append(current.examples, example)
			}
		}
	}
	result := make([]ActionStat, 0, len(states))
	for _, current := range states {
		result = append(result, ActionStat{
			Action: current.action, Count: current.count, InstanceCount: len(current.instanceIDs),
			RepeatedCount: repeated[current.action], AverageDurationMS: averageInt(current.durations),
			ConfidenceAverage: round(averageFloat(current.confidences), 2),
			IsManualCheck:     manualCheckActions[current.action], IsManualWork: manualWorkActions[current.action],
			Examples: current.examples,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].AverageDurationMS > result[j].AverageDurationMS
		}
		return result[i].Count > result[j].Count
	})
	return result
}

func buildPairStats(instances []ScenarioInstance, byID map[uuid.UUID]ActionEvent) []PairStat {
	type state struct {
		key         string
		steps       []string
		count       int
		instanceIDs map[uuid.UUID]bool
		durations   []int
		confidences []float64
	}
	states := map[string]*state{}
	for _, instance := range instances {
		instanceEvents := eventsForInstance(instance, byID)
		for index := 1; index < len(instanceEvents); index++ {
			previous, current := instanceEvents[index-1], instanceEvents[index]
			if previous.CanonicalAction == "" || current.CanonicalAction == "" {
				continue
			}
			key := previous.CanonicalAction + ">" + current.CanonicalAction
			pair := states[key]
			if pair == nil {
				pair = &state{
					key: key, steps: []string{previous.CanonicalAction, current.CanonicalAction},
					instanceIDs: map[uuid.UUID]bool{},
				}
				states[key] = pair
			}
			pair.count++
			pair.instanceIDs[instance.ID] = true
			pair.durations = append(
				pair.durations,
				min(max(0, current.TimestampMS-previous.TimestampMS), 180000),
			)
			pair.confidences = append(pair.confidences, previous.Confidence, current.Confidence)
		}
	}
	result := make([]PairStat, 0, len(states))
	for _, pair := range states {
		result = append(result, PairStat{
			Key: pair.key, Steps: pair.steps, Count: pair.count, InstanceCount: len(pair.instanceIDs),
			AverageDurationMS: averageInt(pair.durations),
			ConfidenceAverage: round(averageFloat(pair.confidences), 2),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].InstanceCount == result[j].InstanceCount {
			return result[i].Count > result[j].Count
		}
		return result[i].InstanceCount > result[j].InstanceCount
	})
	return result
}

func BuildGraph(instances []ScenarioInstance, events []ActionEvent) Graph {
	byID := map[uuid.UUID]ActionEvent{}
	for _, event := range events {
		byID[event.ID] = event
	}
	nodes := map[string]*GraphNode{}
	confidenceTotals := map[string]float64{}
	type edgeState struct {
		GraphEdge
		gaps []int
	}
	edges := map[string]*edgeState{}

	for _, instance := range instances {
		instanceEvents := eventsForInstance(instance, byID)
		for _, event := range instanceEvents {
			node := nodes[event.CanonicalAction]
			if node == nil {
				node = &GraphNode{
					ID: event.CanonicalAction, Label: event.CanonicalAction,
					Type: nodeType(event.CanonicalAction), Severity: nodeSeverity(event.CanonicalAction),
					IssueTypes: []string{}, Examples: []GraphExample{}, RelatedScenarioIDs: []uuid.UUID{},
				}
				nodes[event.CanonicalAction] = node
			}
			node.Frequency++
			confidenceTotals[node.ID] += event.Confidence
			node.Confidence = round(confidenceTotals[node.ID]/float64(node.Frequency), 2)
			if event.IssueType != "" && !includes(node.IssueTypes, event.IssueType) {
				node.IssueTypes = append(node.IssueTypes, event.IssueType)
			}
			if len(node.Examples) < 5 {
				node.Examples = append(node.Examples, GraphExample{
					ScenarioInstanceID: instance.ID, TimestampMS: event.TimestampMS,
					Screen: event.Screen, Target: event.Target, IssueType: event.IssueType,
				})
			}
			if !includesUUID(node.RelatedScenarioIDs, instance.ID) {
				node.RelatedScenarioIDs = append(node.RelatedScenarioIDs, instance.ID)
			}
		}
		for index := 1; index < len(instanceEvents); index++ {
			from, to := instanceEvents[index-1], instanceEvents[index]
			key := from.CanonicalAction + "->" + to.CanonicalAction
			edge := edges[key]
			if edge == nil {
				edge = &edgeState{GraphEdge: GraphEdge{ID: key, From: from.CanonicalAction, To: to.CanonicalAction}}
				edges[key] = edge
			}
			edge.Frequency++
			edge.Weight = edge.Frequency
			edge.gaps = append(edge.gaps, max(0, to.TimestampMS-from.TimestampMS))
			edge.AverageTimeToNextMS = averageInt(edge.gaps)
		}
	}

	graph := Graph{Nodes: make([]GraphNode, 0, len(nodes)), Edges: make([]GraphEdge, 0, len(edges))}
	for _, node := range nodes {
		graph.Nodes = append(graph.Nodes, *node)
	}
	for _, edge := range edges {
		graph.Edges = append(graph.Edges, edge.GraphEdge)
	}
	sort.Slice(graph.Nodes, func(i, j int) bool { return graph.Nodes[i].ID < graph.Nodes[j].ID })
	sort.Slice(graph.Edges, func(i, j int) bool { return graph.Edges[i].ID < graph.Edges[j].ID })
	return graph
}

func eventsForInstance(instance ScenarioInstance, byID map[uuid.UUID]ActionEvent) []ActionEvent {
	result := make([]ActionEvent, 0, len(instance.EventIDs))
	for _, id := range instance.EventIDs {
		if event, ok := byID[id]; ok {
			result = append(result, event)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TimestampMS < result[j].TimestampMS })
	return result
}

func percentile(values []int, fraction float64) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	index := int(float64(len(sorted))*fraction+.999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func averageInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sum := 0
	for _, value := range values {
		sum += value
	}
	return int(float64(sum)/float64(len(values)) + .5)
}

func averageFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func targetKey(event ActionEvent) string {
	value := event.Target
	if value == "" {
		value = event.VisibleText
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func nodeType(action string) string {
	if includes([]string{"NAVIGATE", "FILTER_ISSUES", "OPEN_ORDER", "OPEN_DRIVER_ASSIGNMENT"}, action) {
		return "navigation"
	}
	if manualWorkActions[action] {
		return "manual"
	}
	if action == "RESOLVE_ISSUE" || action == "MARK_PICKUP_COMPLETED" {
		return "resolved"
	}
	return "action"
}

func nodeSeverity(action string) string {
	if action == "RESOLVE_ISSUE" || action == "MARK_PICKUP_COMPLETED" {
		return "success"
	}
	return "normal"
}

func includesUUID(values []uuid.UUID, expected uuid.UUID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func round(value float64, digits int) float64 {
	scale := 1.0
	for range digits {
		scale *= 10
	}
	return float64(int(value*scale+.5)) / scale
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
