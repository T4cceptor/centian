package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/persistence"
)

// TemplateScorecard is the generic template-level scorecard built from all persisted task runs.
type TemplateScorecard struct {
	TemplateKey                string  `json:"templateKey"`
	TemplateID                 string  `json:"templateId"`
	TemplateName               string  `json:"templateName,omitempty"`
	RunCount                   int     `json:"runCount"`
	TotalTaskToolCalls         int     `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls   int     `json:"totalDownstreamToolCalls"`
	MedianTaskToolCalls        int     `json:"medianTaskToolCalls"`
	MedianDownstreamToolCalls  int     `json:"medianDownstreamToolCalls"`
	TotalCentianErrors         int     `json:"totalCentianErrors"`
	TotalDownstreamToolErrors  int     `json:"totalDownstreamToolErrors"`
	MedianCentianErrors        int     `json:"medianCentianErrors"`
	MedianDownstreamToolErrors int     `json:"medianDownstreamToolErrors"`
	MedianDurationMillis       int64   `json:"medianDurationMillis"`
	SuccessRate                float64 `json:"successRate"`
	FirstPassRate              float64 `json:"firstPassRate"`
}

// AgentScorecard is the agent-level scorecard built from persisted benchmark runs.
type AgentScorecard struct {
	Agent                      string   `json:"agent"`
	Model                      string   `json:"model,omitempty"`
	Models                     []string `json:"models,omitempty"`
	RunCount                   int      `json:"runCount"`
	TotalTaskToolCalls         int      `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls   int      `json:"totalDownstreamToolCalls"`
	MedianTaskToolCalls        int      `json:"medianTaskToolCalls"`
	MedianDownstreamToolCalls  int      `json:"medianDownstreamToolCalls"`
	TotalCentianErrors         int      `json:"totalCentianErrors"`
	TotalDownstreamToolErrors  int      `json:"totalDownstreamToolErrors"`
	MedianCentianErrors        int      `json:"medianCentianErrors"`
	MedianDownstreamToolErrors int      `json:"medianDownstreamToolErrors"`
	MedianDurationMillis       int64    `json:"medianDurationMillis"`
	SuccessRate                float64  `json:"successRate"`
	FirstPassRate              float64  `json:"firstPassRate"`
}

// ListTemplateScorecards returns template-level scorecards for all persisted benchmark runs.
func (s *QueryService) ListTemplateScorecards(ctx context.Context) ([]TemplateScorecard, error) {
	s = s.withDefaults()
	if s.store == nil {
		return nil, fmt.Errorf("benchmark query service is not initialized")
	}

	runs, err := s.store.ListBenchmarkRuns(ctx, &persistence.BenchmarkRunFilter{})
	if err != nil {
		return nil, err
	}
	scoreRows, err := s.store.ListBenchmarkRunScores(ctx)
	if err != nil {
		return nil, err
	}
	scoreByRunID := make(map[string]*persistence.BenchmarkRunScoreRecord, len(scoreRows))
	for idx := range scoreRows {
		score := scoreRows[idx]
		scoreByRunID[score.BenchmarkRunID] = &score
	}

	type aggregate struct {
		templateKey      string
		templateID       string
		templateName     string
		runCount         int
		successCount     int
		firstPassCount   int
		taskToolCalls    []int
		downstreamCalls  []int
		centianErrors    []int
		downstreamErrors []int
		durationsMillis  []int64
	}

	grouped := map[string]*aggregate{}
	for idx := range runs {
		run := runs[idx]
		templateID := strings.TrimSpace(run.TemplateID)
		if templateID == "" {
			continue
		}
		score := scoreByRunID[run.BenchmarkRunID]
		scorecard, err := scorecardFromSnapshot(score)
		if err != nil || scorecard == nil {
			continue
		}
		templateName := strings.TrimSpace(run.TemplateName)
		templateKey := templateIdentityKey(templateID, templateName)
		group := grouped[templateKey]
		if group == nil {
			group = &aggregate{
				templateKey:      templateKey,
				templateID:       templateID,
				taskToolCalls:    []int{},
				downstreamCalls:  []int{},
				centianErrors:    []int{},
				downstreamErrors: []int{},
				durationsMillis:  []int64{},
			}
			grouped[templateKey] = group
		}
		group.templateName = firstNonEmpty(group.templateName, templateName)
		group.runCount++
		group.taskToolCalls = append(group.taskToolCalls, scorecard.Process.TotalTaskToolCalls)
		group.downstreamCalls = append(group.downstreamCalls, scorecard.Process.TotalDownstreamToolCalls)
		group.centianErrors = append(group.centianErrors, scorecardCentianErrorCount(scorecard))
		group.downstreamErrors = append(group.downstreamErrors, scorecard.Process.FailedDownstreamToolCalls)
		group.durationsMillis = append(group.durationsMillis, int64(scorecard.Efficiency.WallClockSeconds*1000))
		if score.CompletedSuccessfully {
			group.successCount++
		}
		if score.FirstPassSuccess {
			group.firstPassCount++
		}
	}

	result := make([]TemplateScorecard, 0, len(grouped))
	for _, group := range grouped {
		result = append(result, TemplateScorecard{
			TemplateKey:                group.templateKey,
			TemplateID:                 group.templateID,
			TemplateName:               group.templateName,
			RunCount:                   group.runCount,
			TotalTaskToolCalls:         sumInt(group.taskToolCalls),
			TotalDownstreamToolCalls:   sumInt(group.downstreamCalls),
			MedianTaskToolCalls:        medianInt(group.taskToolCalls),
			MedianDownstreamToolCalls:  medianInt(group.downstreamCalls),
			TotalCentianErrors:         sumInt(group.centianErrors),
			TotalDownstreamToolErrors:  sumInt(group.downstreamErrors),
			MedianCentianErrors:        medianInt(group.centianErrors),
			MedianDownstreamToolErrors: medianInt(group.downstreamErrors),
			MedianDurationMillis:       medianInt64(group.durationsMillis),
			SuccessRate:                scorecardRate(group.successCount, group.runCount),
			FirstPassRate:              scorecardRate(group.firstPassCount, group.runCount),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].RunCount != result[j].RunCount {
			return result[i].RunCount > result[j].RunCount
		}
		left := firstNonEmpty(result[i].TemplateName, result[i].TemplateID)
		right := firstNonEmpty(result[j].TemplateName, result[j].TemplateID)
		return left < right
	})
	return result, nil
}

// ListAgentScorecards returns agent-level scorecards for all persisted benchmark runs.
func (s *QueryService) ListAgentScorecards(ctx context.Context) ([]AgentScorecard, error) {
	s = s.withDefaults()
	if s.store == nil {
		return nil, fmt.Errorf("benchmark query service is not initialized")
	}

	runs, err := s.store.ListBenchmarkRuns(ctx, &persistence.BenchmarkRunFilter{})
	if err != nil {
		return nil, err
	}
	scoreRows, err := s.store.ListBenchmarkRunScores(ctx)
	if err != nil {
		return nil, err
	}
	scoreByRunID := make(map[string]*persistence.BenchmarkRunScoreRecord, len(scoreRows))
	for idx := range scoreRows {
		score := scoreRows[idx]
		scoreByRunID[score.BenchmarkRunID] = &score
	}

	type aggregate struct {
		agent            string
		model            string
		runCount         int
		successCount     int
		firstPassCount   int
		taskToolCalls    []int
		downstreamCalls  []int
		centianErrors    []int
		downstreamErrors []int
		durationsMillis  []int64
	}

	grouped := map[string]*aggregate{}
	for idx := range runs {
		run := runs[idx]
		agent := strings.TrimSpace(run.Agent)
		if agent == "" {
			continue
		}
		score := scoreByRunID[run.BenchmarkRunID]
		scorecard, err := scorecardFromSnapshot(score)
		if err != nil || scorecard == nil {
			continue
		}
		model := firstNonEmpty(strings.TrimSpace(score.SelectedModel), scorecard.SelectedModel, run.SelectedModel)
		groupKey := agentModelScorecardKey(agent, model)
		group := grouped[groupKey]
		if group == nil {
			group = &aggregate{
				agent:            agent,
				model:            model,
				taskToolCalls:    []int{},
				downstreamCalls:  []int{},
				centianErrors:    []int{},
				downstreamErrors: []int{},
				durationsMillis:  []int64{},
			}
			grouped[groupKey] = group
		}
		group.runCount++
		group.taskToolCalls = append(group.taskToolCalls, score.TotalTaskToolCalls)
		group.downstreamCalls = append(group.downstreamCalls, score.TotalDownstreamToolCalls)
		group.centianErrors = append(group.centianErrors, scorecardCentianErrorCount(scorecard))
		group.downstreamErrors = append(group.downstreamErrors, score.FailedDownstreamToolCalls)
		group.durationsMillis = append(group.durationsMillis, int64(score.WallClockSeconds*1000))
		if score.CompletedSuccessfully {
			group.successCount++
		}
		if score.FirstPassSuccess {
			group.firstPassCount++
		}
	}

	result := make([]AgentScorecard, 0, len(grouped))
	for _, group := range grouped {
		result = append(result, AgentScorecard{
			Agent:                      group.agent,
			Model:                      group.model,
			Models:                     agentScorecardModels(group.model),
			RunCount:                   group.runCount,
			TotalTaskToolCalls:         sumInt(group.taskToolCalls),
			TotalDownstreamToolCalls:   sumInt(group.downstreamCalls),
			MedianTaskToolCalls:        medianInt(group.taskToolCalls),
			MedianDownstreamToolCalls:  medianInt(group.downstreamCalls),
			TotalCentianErrors:         sumInt(group.centianErrors),
			TotalDownstreamToolErrors:  sumInt(group.downstreamErrors),
			MedianCentianErrors:        medianInt(group.centianErrors),
			MedianDownstreamToolErrors: medianInt(group.downstreamErrors),
			MedianDurationMillis:       medianInt64(group.durationsMillis),
			SuccessRate:                scorecardRate(group.successCount, group.runCount),
			FirstPassRate:              scorecardRate(group.firstPassCount, group.runCount),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].RunCount != result[j].RunCount {
			return result[i].RunCount > result[j].RunCount
		}
		if result[i].Agent != result[j].Agent {
			return result[i].Agent < result[j].Agent
		}
		return result[i].Model < result[j].Model
	})
	return result, nil
}

func agentModelScorecardKey(agent, model string) string {
	return strings.TrimSpace(agent) + "\x00" + strings.TrimSpace(model)
}

func agentScorecardModels(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	return []string{model}
}

func templateIdentityKey(templateID, templateName string) string {
	templateID = strings.TrimSpace(templateID)
	templateName = strings.TrimSpace(templateName)
	if templateName == "" {
		return templateID
	}
	return templateID + "::" + templateName
}

// scorecardCentianErrorCount returns the total centian-side error count for one run:
// failed task tool calls plus any restart, fail, and timeout events.
func scorecardCentianErrorCount(sc *RunScorecard) int {
	if sc == nil {
		return 0
	}
	return sc.Process.FailedTaskToolCalls + sc.Process.RestartCount + sc.Process.FailCount + sc.Process.TimeoutCount
}

func scorecardRate(successes, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(successes) / float64(total)
}

func sumInt(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}
	return sorted[middle]
}

func medianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}
	return sorted[middle]
}
