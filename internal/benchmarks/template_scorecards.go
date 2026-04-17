package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
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
		templateKey := common.JoinTrimmedIfRight(templateID, templateName, "::")
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
		group.templateName = common.FirstNonEmpty(group.templateName, templateName)
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
			TotalTaskToolCalls:         common.SumInts(group.taskToolCalls),
			TotalDownstreamToolCalls:   common.SumInts(group.downstreamCalls),
			MedianTaskToolCalls:        common.MedianInt(group.taskToolCalls),
			MedianDownstreamToolCalls:  common.MedianInt(group.downstreamCalls),
			TotalCentianErrors:         common.SumInts(group.centianErrors),
			TotalDownstreamToolErrors:  common.SumInts(group.downstreamErrors),
			MedianCentianErrors:        common.MedianInt(group.centianErrors),
			MedianDownstreamToolErrors: common.MedianInt(group.downstreamErrors),
			MedianDurationMillis:       common.MedianInt64(group.durationsMillis),
			SuccessRate:                common.Ratio(group.successCount, group.runCount),
			FirstPassRate:              common.Ratio(group.firstPassCount, group.runCount),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].RunCount != result[j].RunCount {
			return result[i].RunCount > result[j].RunCount
		}
		left := common.FirstNonEmpty(result[i].TemplateName, result[i].TemplateID)
		right := common.FirstNonEmpty(result[j].TemplateName, result[j].TemplateID)
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
		model := common.FirstNonEmpty(strings.TrimSpace(score.SelectedModel), scorecard.SelectedModel, run.SelectedModel)
		groupKey := common.JoinTrimmed(agent, model, "\x00")
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
			Models:                     common.NonEmptyStrings(group.model),
			RunCount:                   group.runCount,
			TotalTaskToolCalls:         common.SumInts(group.taskToolCalls),
			TotalDownstreamToolCalls:   common.SumInts(group.downstreamCalls),
			MedianTaskToolCalls:        common.MedianInt(group.taskToolCalls),
			MedianDownstreamToolCalls:  common.MedianInt(group.downstreamCalls),
			TotalCentianErrors:         common.SumInts(group.centianErrors),
			TotalDownstreamToolErrors:  common.SumInts(group.downstreamErrors),
			MedianCentianErrors:        common.MedianInt(group.centianErrors),
			MedianDownstreamToolErrors: common.MedianInt(group.downstreamErrors),
			MedianDurationMillis:       common.MedianInt64(group.durationsMillis),
			SuccessRate:                common.Ratio(group.successCount, group.runCount),
			FirstPassRate:              common.Ratio(group.firstPassCount, group.runCount),
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

// scorecardCentianErrorCount returns task-tool failures plus restart/fail/timeout events.
func scorecardCentianErrorCount(sc *RunScorecard) int {
	if sc == nil {
		return 0
	}
	return sc.Process.FailedTaskToolCalls + sc.Process.RestartCount + sc.Process.FailCount + sc.Process.TimeoutCount
}
