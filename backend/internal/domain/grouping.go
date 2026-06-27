package domain

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const automationThreshold = .3

var automationScoreWeights = map[string]float64{
	"frequency": .35, "repeatability": .25, "duration": .2,
	"manualCheck": .1, "errorReduction": .1,
}

type ScenarioTemplate struct {
	ID                   uuid.UUID             `json:"id"`
	Code                 string                `json:"code"`
	Name                 string                `json:"name"`
	IssueType            string                `json:"issueType"`
	Signature            string                `json:"signature"`
	Frequency            int                   `json:"frequency"`
	InstanceIDs          []uuid.UUID           `json:"instanceIds"`
	ActionSequence       []string              `json:"actionSequence"`
	Variants             []ScenarioVariant     `json:"variants,omitempty"`
	AverageDurationMS    int                   `json:"averageDurationMs"`
	MedianDurationMS     int                   `json:"medianDurationMs"`
	P95DurationMS        int                   `json:"p95DurationMs"`
	ManualCheckCount     int                   `json:"manualCheckCount"`
	ManualActionCount    int                   `json:"manualActionCount"`
	RepeatedActionCount  int                   `json:"repeatedActionCount"`
	ConfidenceAverage    float64               `json:"confidenceAverage"`
	AmbiguousCount       int                   `json:"ambiguousCount"`
	AutomationScore      float64               `json:"automationScore"`
	Metrics              ScenarioMetrics       `json:"metrics"`
	Status               string                `json:"status"`
	AutomationCandidates []AutomationCandidate `json:"automationCandidates"`
}

type ScenarioVariant struct {
	Sequence  []string `json:"sequence"`
	Frequency int      `json:"frequency"`
}

type AutomationCandidate struct {
	ID            uuid.UUID      `json:"id"`
	TemplateID    uuid.UUID      `json:"templateId"`
	Title         string         `json:"title"`
	Type          string         `json:"type"`
	Rationale     string         `json:"rationale"`
	AffectedSteps []string       `json:"affectedSteps"`
	Impact        string         `json:"impact"`
	Confidence    float64        `json:"confidence"`
	Score         float64        `json:"score"`
	Status        string         `json:"status"`
	Breakdown     map[string]any `json:"breakdown"`
}

type automationResult struct {
	score   float64
	factors map[string]float64
}

type candidateConfig struct {
	level, candidateType, title, rationale string
	affectedSteps                          []string
	occurrences, durationMS                int
	confidence                             *float64
	automation                             automationResult
}

func BuildScenarioGroups(instances []ScenarioInstance, events []ActionEvent) []ScenarioTemplate {
	byEvent := make(map[uuid.UUID]ActionEvent, len(events))
	for _, event := range events {
		byEvent[event.ID] = event
	}
	type accumulator struct {
		template  ScenarioTemplate
		instances []ScenarioInstance
		events    []ActionEvent
	}
	groups := map[string]*accumulator{}

	for _, instance := range instances {
		instanceEvents := eventsForInstance(instance, byEvent)
		sequence := compactActionSequence(instanceEvents)
		signature := strings.Join(append([]string{scenarioCode(instance)}, sequence...), ">")
		group := groups[signature]
		if group == nil {
			id := deterministicUUID("scenario-template:" + signature)
			group = &accumulator{template: ScenarioTemplate{
				ID: id, Code: scenarioCode(instance), Name: scenarioName(instance, sequence),
				IssueType: defaultString(instance.IssueType, "Unknown"), Signature: signature,
				ActionSequence: sequence, Status: "candidate",
			}}
			groups[signature] = group
		}
		group.template.Frequency++
		group.template.InstanceIDs = append(group.template.InstanceIDs, instance.ID)
		group.instances = append(group.instances, instance)
		group.events = append(group.events, instanceEvents...)
	}

	result := make([]ScenarioTemplate, 0, len(groups))
	for _, group := range groups {
		metrics := CalculateMetrics(group.instances, group.events)
		automation := scoreAutomation(group.template.Frequency, metrics, nil)
		group.template.AverageDurationMS = metrics.AverageDurationMS
		group.template.MedianDurationMS = metrics.MedianDurationMS
		group.template.P95DurationMS = metrics.P95DurationMS
		group.template.ManualCheckCount = metrics.ManualCheckCount
		group.template.ManualActionCount = metrics.ManualActionCount
		group.template.RepeatedActionCount = metrics.RepeatedActionCount
		group.template.ConfidenceAverage = metrics.ConfidenceAverage
		group.template.AmbiguousCount = metrics.AmbiguousCount
		group.template.AutomationScore = automation.score
		group.template.Metrics = metrics
		group.template.AutomationCandidates = buildAutomationCandidates(group.template, metrics, automation)
		result = append(result, group.template)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Frequency == result[j].Frequency {
			return result[i].AutomationScore > result[j].AutomationScore
		}
		return result[i].Frequency > result[j].Frequency
	})
	return result
}

func BuildProjectScenarioGroups(instances []ScenarioInstance, events []ActionEvent) []ScenarioTemplate {
	byEvent := make(map[uuid.UUID]ActionEvent, len(events))
	for _, event := range events {
		byEvent[event.ID] = event
	}
	type accumulator struct {
		template   ScenarioTemplate
		instances  []ScenarioInstance
		events     []ActionEvent
		variants   map[string]ScenarioVariant
		instanceID map[uuid.UUID]bool
	}
	groups := map[string]*accumulator{}

	for _, instance := range instances {
		key := scenarioCode(instance)
		instanceEvents := eventsForInstance(instance, byEvent)
		sequence := compactActionSequence(instanceEvents)
		group := groups[key]
		if group == nil {
			group = &accumulator{
				template: ScenarioTemplate{
					ID:   deterministicUUID("project-scenario-template:" + key),
					Code: key, Name: scenarioName(instance, sequence),
					IssueType: defaultString(instance.IssueType, "Unknown"),
					Signature: "PROJECT>" + key, Status: "candidate",
				},
				variants:   map[string]ScenarioVariant{},
				instanceID: map[uuid.UUID]bool{},
			}
			groups[key] = group
		}
		if !group.instanceID[instance.ID] {
			group.template.Frequency++
			group.template.InstanceIDs = append(group.template.InstanceIDs, instance.ID)
			group.instances = append(group.instances, instance)
			group.instanceID[instance.ID] = true
		}
		group.events = append(group.events, instanceEvents...)
		variantKey := strings.Join(sequence, ">")
		variant := group.variants[variantKey]
		variant.Sequence = sequence
		variant.Frequency++
		group.variants[variantKey] = variant
	}

	result := make([]ScenarioTemplate, 0, len(groups))
	for _, group := range groups {
		metrics := CalculateMetrics(group.instances, group.events)
		variants := make([]ScenarioVariant, 0, len(group.variants))
		for _, variant := range group.variants {
			variants = append(variants, variant)
		}
		sort.Slice(variants, func(i, j int) bool { return variants[i].Frequency > variants[j].Frequency })
		group.template.Variants = variants
		if len(variants) > 0 {
			group.template.ActionSequence = variants[0].Sequence
		}
		repeatability := 0.0
		if len(variants) > 0 {
			repeatability = float64(variants[0].Frequency) / float64(max(1, group.template.Frequency))
		}
		automation := scoreAutomation(
			group.template.Frequency,
			metrics,
			map[string]float64{"repeatability": repeatability},
		)
		group.template.AverageDurationMS = metrics.AverageDurationMS
		group.template.MedianDurationMS = metrics.MedianDurationMS
		group.template.P95DurationMS = metrics.P95DurationMS
		group.template.ManualCheckCount = metrics.ManualCheckCount
		group.template.ManualActionCount = metrics.ManualActionCount
		group.template.RepeatedActionCount = metrics.RepeatedActionCount
		group.template.ConfidenceAverage = metrics.ConfidenceAverage
		group.template.AmbiguousCount = metrics.AmbiguousCount
		group.template.AutomationScore = automation.score
		group.template.Metrics = metrics
		group.template.AutomationCandidates = buildAutomationCandidates(group.template, metrics, automation)
		result = append(result, group.template)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Frequency == result[j].Frequency {
			return result[i].AutomationScore > result[j].AutomationScore
		}
		return result[i].Frequency > result[j].Frequency
	})
	return result
}

func buildAutomationCandidates(
	group ScenarioTemplate,
	metrics ScenarioMetrics,
	scenarioAutomation automationResult,
) []AutomationCandidate {
	candidates := []AutomationCandidate{}
	scenarioType := scenarioCandidateType(group, metrics)
	candidates = appendCandidate(candidates, group, metrics, candidateConfig{
		level: "scenario_group", candidateType: scenarioType,
		title: automationTitle(group, scenarioType), affectedSteps: group.ActionSequence,
		occurrences: group.Frequency, durationMS: metrics.MedianDurationMS,
		automation: scenarioAutomation,
		rationale: fmt.Sprintf(
			"Сценарий встречается %d раз; медиана %s; типовой путь повторяется в %d%% случаев.",
			group.Frequency, formatDuration(metrics.MedianDurationMS),
			int(scenarioAutomation.factors["repeatability"]*100+.5),
		),
	})

	for _, stat := range metrics.ActionStats {
		candidateType := actionCandidateType(stat.Action)
		if candidateType == "" {
			continue
		}
		manualCheck := 0.0
		if stat.IsManualCheck {
			manualCheck = float64(stat.Count) / float64(max(1, group.Frequency))
		}
		automation := scoreAutomation(group.Frequency, metrics, map[string]float64{
			"frequency":      float64(stat.Count) / 10,
			"repeatability":  float64(stat.InstanceCount) / float64(max(1, group.Frequency)),
			"duration":       float64(stat.AverageDurationMS) / 180000,
			"manualCheck":    manualCheck,
			"errorReduction": float64(stat.RepeatedCount) / float64(max(1, stat.Count)),
		})
		confidence := stat.ConfidenceAverage
		candidates = appendCandidate(candidates, group, metrics, candidateConfig{
			level: "action", candidateType: candidateType,
			title:         fmt.Sprintf("%s: %s", candidateTypeLabel(candidateType), stat.Action),
			affectedSteps: []string{stat.Action}, occurrences: stat.Count,
			durationMS: stat.AverageDurationMS, automation: automation, confidence: &confidence,
			rationale: fmt.Sprintf(
				"%s встречается %d раз в %d экземплярах; среднее время до следующего шага %s.",
				stat.Action, stat.Count, stat.InstanceCount, formatDuration(stat.AverageDurationMS),
			),
		})
	}

	pairLimit := min(5, len(metrics.PairStats))
	for _, pair := range metrics.PairStats[:pairLimit] {
		if pair.InstanceCount < 2 {
			continue
		}
		manualCheck := 0.0
		if includes(pair.Steps, "CHECK") || includes(pair.Steps, "INSPECT_ISSUE") {
			manualCheck = float64(pair.Count) / float64(max(1, group.Frequency))
		}
		automation := scoreAutomation(group.Frequency, metrics, map[string]float64{
			"frequency":      float64(pair.Count) / 10,
			"repeatability":  float64(pair.InstanceCount) / float64(max(1, group.Frequency)),
			"duration":       float64(pair.AverageDurationMS) / 180000,
			"manualCheck":    manualCheck,
			"errorReduction": 0,
		})
		confidence := pair.ConfidenceAverage
		candidates = appendCandidate(candidates, group, metrics, candidateConfig{
			level: "action_pair", candidateType: "next_best_action_hint",
			title:         "Подсказка перехода: " + strings.Join(pair.Steps, " → "),
			affectedSteps: pair.Steps, occurrences: pair.Count,
			durationMS: pair.AverageDurationMS, automation: automation, confidence: &confidence,
			rationale: fmt.Sprintf(
				"Переход встречается в %d из %d экземпляров сценария.",
				pair.InstanceCount, group.Frequency,
			),
		})
	}

	filtered := candidates[:0]
	for _, candidate := range candidates {
		if candidate.Score >= automationThreshold {
			filtered = append(filtered, candidate)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Score == filtered[j].Score {
			return breakdownInt(filtered[i], "durationImpactMs") > breakdownInt(filtered[j], "durationImpactMs")
		}
		return filtered[i].Score > filtered[j].Score
	})
	if len(filtered) > 8 {
		filtered = filtered[:8]
	}
	return filtered
}

func appendCandidate(
	candidates []AutomationCandidate,
	group ScenarioTemplate,
	metrics ScenarioMetrics,
	config candidateConfig,
) []AutomationCandidate {
	steps := make([]string, 0, len(config.affectedSteps))
	for _, step := range config.affectedSteps {
		if step != "" {
			steps = append(steps, step)
		}
	}
	confidence := metrics.ConfidenceAverage
	if config.confidence != nil {
		confidence = *config.confidence
	}
	confidence = minFloat(.99, maxFloat(0, confidence))
	durationImpact := config.durationMS * config.occurrences
	factors := copyFloatMap(config.automation.factors)
	weights := copyFloatMap(automationScoreWeights)
	idSource := fmt.Sprintf(
		"automation:%s:%s:%s:%s",
		group.ID, config.level, config.candidateType, strings.Join(steps, ">"),
	)
	return append(candidates, AutomationCandidate{
		ID: deterministicUUID(idSource), TemplateID: group.ID,
		Title: config.title, Type: config.candidateType, Rationale: config.rationale,
		AffectedSteps: steps,
		Impact:        fmt.Sprintf("Оценочный объём: %s на текущей выборке.", formatDuration(durationImpact)),
		Confidence:    confidence, Score: config.automation.score, Status: "new",
		Breakdown: map[string]any{
			"level": config.level, "frequency": config.occurrences,
			"averageDurationMs": config.durationMS, "durationImpactMs": durationImpact,
			"repeatability":         config.automation.factors["repeatability"],
			"manualCheckImpact":     config.automation.factors["manualCheck"],
			"errorReduction":        config.automation.factors["errorReduction"],
			"dataQualityConfidence": confidence, "factors": factors,
			"weights": weights, "sampleSize": group.Frequency,
		},
	})
}

func scoreAutomation(
	frequency int,
	metrics ScenarioMetrics,
	overrides map[string]float64,
) automationResult {
	factors := map[string]float64{
		"frequency": clamp01(float64(frequency) / 10),
		"repeatability": func() float64 {
			if frequency > 1 {
				return 1
			}
			return .5
		}(),
		"duration":       clamp01(float64(metrics.MedianDurationMS) / 180000),
		"manualCheck":    clamp01(float64(metrics.ManualCheckCount) / float64(max(1, frequency))),
		"errorReduction": clamp01(float64(metrics.AmbiguousCount+metrics.InterruptedCount) / float64(max(1, frequency*2))),
	}
	for key, value := range overrides {
		factors[key] = clamp01(value)
	}
	score := 0.0
	for key, weight := range automationScoreWeights {
		score += factors[key] * weight
	}
	return automationResult{score: round(score, 2), factors: factors}
}

func scenarioCandidateType(group ScenarioTemplate, metrics ScenarioMetrics) string {
	if metrics.ManualCheckCount >= group.Frequency {
		return "manual_check_assistant"
	}
	if includes(group.ActionSequence, "CHANGE_FIELD_VALUE") ||
		includes(group.ActionSequence, "EDIT_FIELD") {
		return "auto_fill"
	}
	if len(group.Variants) <= 2 && group.Frequency >= 3 {
		return "rule_based_decision"
	}
	return "next_best_action_hint"
}

func actionCandidateType(action string) string {
	switch action {
	case "CHECK", "INSPECT_ISSUE":
		return "manual_check_assistant"
	case "RESOLUTION_ATTEMPT", "SAVE":
		return "validation_assistant"
	case "CHANGE_FIELD_VALUE", "EDIT_FIELD":
		return "auto_fill"
	case "MARK_PICKUP_COMPLETED":
		return "auto_status_change"
	default:
		return ""
	}
}

func automationTitle(group ScenarioTemplate, candidateType string) string {
	switch candidateType {
	case "manual_check_assistant":
		return "Помощник ручной проверки: " + group.Name
	case "next_best_action_hint":
		return "Подсказка следующего шага: " + group.Name
	case "auto_fill":
		return "Автозаполнение: " + group.Name
	default:
		return "Правило обработки: " + group.Name
	}
}

func candidateTypeLabel(candidateType string) string {
	labels := map[string]string{
		"auto_fill": "Автозаполнение", "validation_assistant": "Помощник валидации",
		"manual_check_assistant": "Помощник ручной проверки",
		"auto_status_change":     "Автоматическая смена статуса",
	}
	if label := labels[candidateType]; label != "" {
		return label
	}
	return "Автоматизация"
}

func scenarioCode(instance ScenarioInstance) string {
	if instance.KnownScenarioCode != "" {
		return instance.KnownScenarioCode
	}
	if instance.IssueType != "" {
		return strings.ToUpper(strings.ReplaceAll(instance.IssueType, " ", "_"))
	}
	return "UNKNOWN_SCENARIO"
}

func scenarioName(instance ScenarioInstance, sequence []string) string {
	for _, known := range KnownScenarios {
		if known.Code == instance.KnownScenarioCode || known.IssueType == instance.IssueType {
			return known.Name
		}
	}
	if includes(sequence, "SEND_TO_SELECTED_DRIVER") ||
		includes(sequence, "SELECT_DRIVER") ||
		includes(sequence, "ASSIGN_DRIVER") {
		return "Назначение курьера для проблемного заказа"
	}
	if includes(sequence, "CHANGE_FIELD_VALUE") ||
		includes(sequence, "OPEN_FIELD_EDITOR") ||
		includes(sequence, "EDIT_FIELD") {
		return instance.IssueType + ": ручное изменение поля"
	}
	return defaultString(instance.IssueType, "Неизвестный сценарий") + ": типовой путь"
}

func compactActionSequence(events []ActionEvent) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		if event.CanonicalAction == "" {
			continue
		}
		if len(result) == 0 || result[len(result)-1] != event.CanonicalAction {
			result = append(result, event.CanonicalAction)
		}
	}
	return result
}

func formatDuration(milliseconds int) string {
	seconds := max(0, int(float64(milliseconds)/1000+.5))
	if seconds < 60 {
		return fmt.Sprintf("%d сек", seconds)
	}
	return fmt.Sprintf("%d мин %d сек", seconds/60, seconds%60)
}

func breakdownInt(candidate AutomationCandidate, key string) int {
	value, _ := candidate.Breakdown[key].(int)
	return value
}

func copyFloatMap(source map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func clamp01(value float64) float64 {
	return maxFloat(0, minFloat(1, value))
}

func deterministicUUID(value string) uuid.UUID {
	sum := sha1.Sum([]byte(value))
	id, _ := uuid.FromBytes(sum[:16])
	return id
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
