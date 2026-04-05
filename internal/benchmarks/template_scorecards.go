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
	MedianTaskToolCalls        int     `json:"medianTaskToolCalls"`
	MedianDownstreamToolCalls  int     `json:"medianDownstreamToolCalls"`
	MedianCentianErrors        int     `json:"medianCentianErrors"`
	MedianDownstreamToolErrors int     `json:"medianDownstreamToolErrors"`
	MedianDurationMillis       int64   `json:"medianDurationMillis"`
	FirstPassRate              float64 `json:"firstPassRate"`
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
			MedianTaskToolCalls:        medianInt(group.taskToolCalls),
			MedianDownstreamToolCalls:  medianInt(group.downstreamCalls),
			MedianCentianErrors:        medianInt(group.centianErrors),
			MedianDownstreamToolErrors: medianInt(group.downstreamErrors),
			MedianDurationMillis:       medianInt64(group.durationsMillis),
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
	return strings.TrimSpace(status) == runStatusCompleted &&
		stats.RestartCount == 0 &&
		stats.FailCount == 0 &&
		stats.TimeoutCount == 0
}

func scorecardRate(successes, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(successes) / float64(total)
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
