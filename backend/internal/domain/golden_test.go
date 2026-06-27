package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"
)

type goldenRegressionFixture struct {
	Version int                    `json:"version"`
	Source  string                 `json:"source"`
	Cases   []goldenRegressionCase `json:"cases"`
}

type goldenRegressionCase struct {
	Name            string           `json:"name"`
	RawEvents       []RawVisionEvent `json:"rawEvents"`
	ExpectedActions []string         `json:"expectedActions"`
	ExpectedOrderID string           `json:"expectedOrderId"`
	ExpectedFlags   []string         `json:"expectedFlags"`
}

type fullPipelineFixture struct {
	Version   int              `json:"version"`
	Source    string           `json:"source"`
	RawEvents []RawVisionEvent `json:"rawEvents"`
	Expected  struct {
		Actions           []string                `json:"actions"`
		QualityIssueTypes []string                `json:"qualityIssueTypes"`
		ScenarioCount     int                     `json:"scenarioCount"`
		Scenarios         []expectedScenario      `json:"scenarios"`
		GroupSignatures   []string                `json:"groupSignatures"`
		ProjectGroupCodes []string                `json:"projectGroupCodes"`
		Metrics           expectedPipelineMetrics `json:"metrics"`
		Graph             expectedPipelineGraph   `json:"graph"`
		Report            expectedPipelineReport  `json:"report"`
	} `json:"expected"`
}

type expectedScenario struct {
	Code       string `json:"code"`
	OrderID    string `json:"orderId"`
	IssueType  string `json:"issueType"`
	Outcome    string `json:"outcome"`
	Status     string `json:"status"`
	DurationMS int    `json:"durationMs"`
	EventCount int    `json:"eventCount"`
}

type expectedPipelineMetrics struct {
	AverageDurationMS int     `json:"averageDurationMs"`
	MedianDurationMS  int     `json:"medianDurationMs"`
	P95DurationMS     int     `json:"p95DurationMs"`
	ManualCheckCount  int     `json:"manualCheckCount"`
	ManualActionCount int     `json:"manualActionCount"`
	ConfidenceAverage float64 `json:"confidenceAverage"`
}

type expectedPipelineGraph struct {
	Nodes              int `json:"nodes"`
	Edges              int `json:"edges"`
	OpenOrderFrequency int `json:"openOrderFrequency"`
}

type expectedPipelineReport struct {
	Summary              string `json:"summary"`
	NormalizationVersion string `json:"normalizationVersion"`
	BoundaryRuleVersion  string `json:"boundaryRuleVersion"`
	GroupingVersion      string `json:"groupingVersion"`
}

func TestGoldenRegressionFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/regression.json")
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}

	var fixture goldenRegressionFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse golden fixture: %v", err)
	}
	if fixture.Version != 1 {
		t.Fatalf("unexpected fixture version: %d", fixture.Version)
	}
	if fixture.Source != "../analyst-app/test/scenario-regression.test.js" {
		t.Fatalf("unexpected fixture source: %s", fixture.Source)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("golden fixture has no cases")
	}

	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			result := Normalize(uuid.New(), uuid.New(), testCase.RawEvents)
			if len(result.ActionEvents) != len(testCase.ExpectedActions) {
				t.Fatalf("expected %d action events, got %d: %#v", len(testCase.ExpectedActions), len(result.ActionEvents), result.ActionEvents)
			}

			gotActions := make([]string, 0, len(result.ActionEvents))
			for _, event := range result.ActionEvents {
				gotActions = append(gotActions, event.CanonicalAction)
			}
			if !reflect.DeepEqual(gotActions, testCase.ExpectedActions) {
				t.Fatalf("unexpected canonical actions: got %#v want %#v", gotActions, testCase.ExpectedActions)
			}

			if testCase.ExpectedOrderID != "" {
				if result.ActionEvents[0].OrderID != testCase.ExpectedOrderID {
					t.Fatalf("unexpected order id: got %q want %q", result.ActionEvents[0].OrderID, testCase.ExpectedOrderID)
				}
			}

			for _, expectedFlag := range testCase.ExpectedFlags {
				if !includes(result.ActionEvents[0].QualityFlags, expectedFlag) {
					t.Fatalf("missing expected quality flag %q in %#v", expectedFlag, result.ActionEvents[0].QualityFlags)
				}
				if !hasIssueType(result.DataQualityIssues, expectedFlag) {
					t.Fatalf("missing expected data quality issue %q in %#v", expectedFlag, result.DataQualityIssues)
				}
			}
		})
	}
}

func TestFullPipelineGoldenFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/full_pipeline_regression.json")
	if err != nil {
		t.Fatalf("read full pipeline golden fixture: %v", err)
	}

	var fixture fullPipelineFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse full pipeline golden fixture: %v", err)
	}
	if fixture.Version != 1 {
		t.Fatalf("unexpected fixture version: %d", fixture.Version)
	}
	if fixture.Source == "" {
		t.Fatal("fixture source must be documented")
	}

	recordingID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	analysisRunID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	normalized := Normalize(recordingID, analysisRunID, fixture.RawEvents)

	if got := actionSequence(normalized.ActionEvents); !reflect.DeepEqual(got, fixture.Expected.Actions) {
		t.Fatalf("unexpected action sequence:\ngot  %#v\nwant %#v", got, fixture.Expected.Actions)
	}
	if got := sortedIssueTypes(normalized.DataQualityIssues); !reflect.DeepEqual(got, fixture.Expected.QualityIssueTypes) {
		t.Fatalf("unexpected quality issue types: got %#v want %#v", got, fixture.Expected.QualityIssueTypes)
	}

	issues := append([]DataQualityIssue(nil), normalized.DataQualityIssues...)
	scenarios := BuildScenarioInstances(recordingID, normalized.ActionEvents, &issues)
	if len(scenarios) != fixture.Expected.ScenarioCount {
		t.Fatalf("unexpected scenario count: got %d want %d", len(scenarios), fixture.Expected.ScenarioCount)
	}
	if got := scenarioSummaries(scenarios); !reflect.DeepEqual(got, fixture.Expected.Scenarios) {
		t.Fatalf("unexpected scenarios:\ngot  %#v\nwant %#v", got, fixture.Expected.Scenarios)
	}

	groups := BuildScenarioGroups(scenarios, normalized.ActionEvents)
	if got := sortedGroupSignatures(groups); !reflect.DeepEqual(got, fixture.Expected.GroupSignatures) {
		t.Fatalf("unexpected group signatures:\ngot  %#v\nwant %#v", got, fixture.Expected.GroupSignatures)
	}
	projectGroups := BuildProjectScenarioGroups(scenarios, normalized.ActionEvents)
	if got := sortedProjectGroupCodes(projectGroups); !reflect.DeepEqual(got, fixture.Expected.ProjectGroupCodes) {
		t.Fatalf("unexpected project group codes:\ngot  %#v\nwant %#v", got, fixture.Expected.ProjectGroupCodes)
	}

	metrics := CalculateMetrics(scenarios, normalized.ActionEvents)
	expectedMetrics := fixture.Expected.Metrics
	if metrics.AverageDurationMS != expectedMetrics.AverageDurationMS ||
		metrics.MedianDurationMS != expectedMetrics.MedianDurationMS ||
		metrics.P95DurationMS != expectedMetrics.P95DurationMS ||
		metrics.ManualCheckCount != expectedMetrics.ManualCheckCount ||
		metrics.ManualActionCount != expectedMetrics.ManualActionCount ||
		metrics.ConfidenceAverage != expectedMetrics.ConfidenceAverage {
		t.Fatalf("unexpected metrics: got %#v want %#v", metrics, expectedMetrics)
	}

	graph := BuildGraph(scenarios, normalized.ActionEvents)
	if len(graph.Nodes) != fixture.Expected.Graph.Nodes || len(graph.Edges) != fixture.Expected.Graph.Edges {
		t.Fatalf(
			"unexpected graph size: got nodes=%d edges=%d want nodes=%d edges=%d",
			len(graph.Nodes), len(graph.Edges), fixture.Expected.Graph.Nodes, fixture.Expected.Graph.Edges,
		)
	}
	if frequency := graphNodeFrequency(graph, "OPEN_ORDER"); frequency != fixture.Expected.Graph.OpenOrderFrequency {
		t.Fatalf("unexpected OPEN_ORDER graph frequency: got %d want %d", frequency, fixture.Expected.Graph.OpenOrderFrequency)
	}

	report := buildGoldenReportSummary(len(scenarios), len(groups), fixture.Expected.Report)
	if report != fixture.Expected.Report.Summary {
		t.Fatalf("unexpected report summary: got %q want %q", report, fixture.Expected.Report.Summary)
	}
	if fixture.Expected.Report.NormalizationVersion != NormalizationVersion {
		t.Fatalf("unexpected normalization version: got %q want %q", NormalizationVersion, fixture.Expected.Report.NormalizationVersion)
	}
	if fixture.Expected.Report.BoundaryRuleVersion != BoundaryRuleVersion {
		t.Fatalf("unexpected boundary rule version: got %q want %q", BoundaryRuleVersion, fixture.Expected.Report.BoundaryRuleVersion)
	}
	if fixture.Expected.Report.GroupingVersion != "scenario-grouping-v6" {
		t.Fatalf("unexpected grouping version: %q", fixture.Expected.Report.GroupingVersion)
	}
}

func actionSequence(events []ActionEvent) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, event.CanonicalAction)
	}
	return result
}

func sortedIssueTypes(issues []DataQualityIssue) []string {
	result := make([]string, 0, len(issues))
	for _, issue := range issues {
		result = append(result, issue.Type)
	}
	sort.Strings(result)
	return result
}

func scenarioSummaries(scenarios []ScenarioInstance) []expectedScenario {
	result := make([]expectedScenario, 0, len(scenarios))
	for _, scenario := range scenarios {
		result = append(result, expectedScenario{
			Code: scenario.KnownScenarioCode, OrderID: scenario.OrderID,
			IssueType: scenario.IssueType, Outcome: scenario.Outcome, Status: scenario.Status,
			DurationMS: scenario.DurationMS, EventCount: len(scenario.EventIDs),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result
}

func sortedGroupSignatures(groups []ScenarioTemplate) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.Signature)
	}
	sort.Strings(result)
	return result
}

func sortedProjectGroupCodes(groups []ScenarioTemplate) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.Code)
	}
	sort.Strings(result)
	return result
}

func graphNodeFrequency(graph Graph, nodeID string) int {
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			return node.Frequency
		}
	}
	return 0
}

func buildGoldenReportSummary(scenarioCount, groupCount int, expected expectedPipelineReport) string {
	if expected.NormalizationVersion == "" || expected.BoundaryRuleVersion == "" || expected.GroupingVersion == "" {
		return ""
	}
	return fmt.Sprintf(
		"Построено %d экземпляров и %d групп сценариев. Нормализация, границы и метрики рассчитаны детерминированным Go-кодом.",
		scenarioCount, groupCount,
	)
}
