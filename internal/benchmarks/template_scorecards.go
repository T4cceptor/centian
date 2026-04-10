package benchmarks

import (
	"context"
	"encoding/json"
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

// ListTemplateScorecards returns template-level scorecards for all persisted task runs.
func (s *QueryService) ListTemplateScorecards(ctx context.Context) ([]TemplateScorecard, error) {
	s = s.withDefaults()
	if s.store == nil {
		return nil, fmt.Errorf("benchmark query service is not initialized")
	}

	statsRows, err := s.store.ListTaskRunStats(ctx)
	if err != nil {
		return nil, err
	}
	statsByRunID := make(map[string]*persistence.TaskRunStatsRecord, len(statsRows))
	for idx := range statsRows {
		stats := statsRows[idx]
		statsByRunID[stats.RunID] = &stats
	}

	snapshots, err := s.store.ListTaskRunSnapshots(ctx)
	if err != nil {
		return nil, err
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
	for idx := range snapshots {
		snapshot := snapshots[idx]
		templateID := strings.TrimSpace(snapshot.TemplateID)
		if templateID == "" {
			continue
		}
		templateName := strings.TrimSpace(snapshot.TemplateName)
		templateKey := templateIdentityKey(templateID, templateName)
		stats := statsByRunID[snapshot.RunID]
		if stats == nil {
			continue
		}
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
		group.taskToolCalls = append(group.taskToolCalls, stats.TaskToolCallCount)
		group.downstreamCalls = append(group.downstreamCalls, stats.DownstreamToolCallCount)
		group.centianErrors = append(group.centianErrors, stats.TaskToolErrorCount+stats.RestartCount+stats.FailCount+stats.TimeoutCount)
		group.downstreamErrors = append(group.downstreamErrors, stats.DownstreamToolErrorCount)
		if stats.DurationMillis != nil {
			group.durationsMillis = append(group.durationsMillis, *stats.DurationMillis)
		}
		if isSuccessful(snapshot.Status) {
			group.successCount++
		}
		if isFirstPass(snapshot.Status, stats) {
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
		centianErrors := score.FailedTaskToolCalls
		if score.RestartOccurred {
			centianErrors++
		}
		if score.FailOccurred {
			centianErrors++
		}
		if score.TimeoutOccurred {
			centianErrors++
		}
		group.centianErrors = append(group.centianErrors, centianErrors)
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

func selectedModelForAgentScorecardRun(run persistence.BenchmarkRunRecord) (string, error) {
	model := strings.TrimSpace(run.SelectedModel)
	if len(run.AgentMetadataJSON) == 0 {
		return model, nil
	}

	var metadata AgentMetadata
	if err := json.Unmarshal(run.AgentMetadataJSON, &metadata); err != nil {
		return "", fmt.Errorf("unmarshal benchmark agent metadata for %q: %w", run.BenchmarkRunID, err)
	}
	return firstNonEmpty(strings.TrimSpace(metadata.SelectedModel), agentMetadataModelUsageLabel(metadata.ModelUsage), model), nil
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

func agentMetadataModelUsageLabel(modelUsage map[string]AgentModelUsage) string {
	if len(modelUsage) == 0 {
		return ""
	}
	models := make([]string, 0, len(modelUsage))
	for model := range modelUsage {
		if model = strings.TrimSpace(model); model != "" {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return strings.Join(models, ", ")
}

func templateIdentityKey(templateID, templateName string) string {
	templateID = strings.TrimSpace(templateID)
	templateName = strings.TrimSpace(templateName)
	if templateName == "" {
		return templateID
	}
	return templateID + "::" + templateName
}

func isFirstPass(status string, stats *persistence.TaskRunStatsRecord) bool {
	if stats == nil {
		return false
	}
	return isSuccessful(status) &&
		stats.RestartCount == 0 &&
		stats.FailCount == 0 &&
		stats.TimeoutCount == 0
}

func isSuccessful(status string) bool {
	return strings.TrimSpace(status) == runStatusCompleted
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
