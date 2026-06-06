package persistence

import (
	"context"
	"fmt"
	"strings"
)

// ActivityFilter bounds the activity aggregation to a time window (inclusive).
type ActivityFilter struct {
	Start int64 // unix milli, inclusive
	End   int64 // unix milli, inclusive
}

// ActivitySummary is the aggregated payload backing the UI "Activity" view.
// JSON tags intentionally match the frontend ActivitySummary type
// (web/src/api/activity.ts) so it can be consumed without transformation.
type ActivitySummary struct {
	RangeStartUnixMilli int64                  `json:"rangeStartUnixMilli"`
	RangeEndUnixMilli   int64                  `json:"rangeEndUnixMilli"`
	Stats               ActivityStats          `json:"stats"`
	CategoryCounts      ActivityCategoryCounts `json:"categoryCounts"`
	Volume              []ActivityVolumePoint  `json:"volume"`
	Interventions       []ActivityIntervention `json:"interventions"`
}

// ActivityStats holds the headline counters shown above the skyline chart.
type ActivityStats struct {
	Interventions      int `json:"interventions"`
	ThreatsNeutralized int `json:"threatsNeutralized"`
	PIIRedacted        int `json:"piiRedacted"`
	RiskyActionsHeld   int `json:"riskyActionsHeld"`
	RequestsInspected  int `json:"requestsInspected"`
}

// ActivityCategoryCounts is a fixed-key map so every category is always present.
type ActivityCategoryCounts struct {
	Security   int `json:"security"`
	Policy     int `json:"policy"`
	Risk       int `json:"risk"`
	Quality    int `json:"quality"`
	Compliance int `json:"compliance"`
}

// ActivityVolumePoint is one time-bucket of inspected request volume.
type ActivityVolumePoint struct {
	TimeUnixMilli int64   `json:"timeUnixMilli"`
	Volume        float64 `json:"volume"`
}

// ActivityIntervention is a single moment where a processor stepped in.
type ActivityIntervention struct {
	ID                 string  `json:"id"`
	Category           string  `json:"category"`
	TimestampUnixMilli int64   `json:"timestampUnixMilli"`
	Severity           float64 `json:"severity"`
	Title              string  `json:"title"`
	Summary            string  `json:"summary"`
	RuleID             string  `json:"ruleId"`
	RuleExplanation    string  `json:"ruleExplanation"`
	ToolName           string  `json:"toolName"`
	Gateway            string  `json:"gateway"`
	ServerName         string  `json:"serverName"`
	Label              string  `json:"label,omitempty"`

	action string // raw action verb, retained for stat classification (not serialized)
}

const activityVolumeBuckets = 64

// activityInterventionRow is one governance annotation row joined to its action event.
type activityInterventionRow struct {
	AnnotationID       string `bun:"annotation_id"`
	ActionEventID      string `bun:"action_event_id"`
	CreatedAtUnixMilli int64  `bun:"created_at_unix_milli"`
	Category           string `bun:"category"`
	Severity           string `bun:"severity"`
	Action             string `bun:"action"`
	Processor          string `bun:"processor"`
	Message            string `bun:"message"`
	Rule               string `bun:"rule"`
	ToolName           string `bun:"tool_name"`
	Gateway            string `bun:"gateway"`
	ServerName         string `bun:"server_name"`
}

type activityVolumeRow struct {
	CreatedAtUnixMilli int64  `bun:"created_at_unix_milli"`
	MessageType        string `bun:"message_type"`
}

// ActivitySummary aggregates interventions, request volume, and headline stats for
// the given window. It powers GET /api/{projectSlug}/activity.
func (s *Store) ActivitySummary(ctx context.Context, filter *ActivityFilter) (*ActivitySummary, error) {
	if filter == nil {
		filter = &ActivityFilter{}
	}
	start, end := filter.Start, filter.End
	if end <= start {
		return nil, fmt.Errorf("activity window end must be after start")
	}

	interventions, err := s.activityInterventions(ctx, start, end)
	if err != nil {
		return nil, err
	}
	volume, requestsInspected, err := s.activityVolume(ctx, start, end)
	if err != nil {
		return nil, err
	}

	summary := &ActivitySummary{
		RangeStartUnixMilli: start,
		RangeEndUnixMilli:   end,
		Interventions:       interventions,
		Volume:              volume,
	}
	summary.Stats.Interventions = len(interventions)
	summary.Stats.RequestsInspected = requestsInspected
	for idx := range interventions {
		item := &interventions[idx]
		switch item.Category {
		case "security":
			summary.CategoryCounts.Security++
		case "policy":
			summary.CategoryCounts.Policy++
		case "risk":
			summary.CategoryCounts.Risk++
		case "quality":
			summary.CategoryCounts.Quality++
		case "compliance":
			summary.CategoryCounts.Compliance++
		}
		action := strings.ToLower(item.action)
		if item.Category == "security" {
			summary.Stats.ThreatsNeutralized++
		}
		if strings.Contains(action, "redact") {
			summary.Stats.PIIRedacted++
		}
		if strings.Contains(action, "hold") || strings.Contains(action, "held") {
			summary.Stats.RiskyActionsHeld++
		}
	}
	return summary, nil
}

func (s *Store) activityInterventions(ctx context.Context, start, end int64) ([]ActivityIntervention, error) {
	rows := make([]activityInterventionRow, 0)
	query := `
SELECT
	a.id AS annotation_id,
	a.action_event_id,
	ae.created_at_unix_milli,
	a.category,
	a.severity,
	a.action,
	a.processor,
	a.message,
	a.rule,
	ae.tool_name,
	ae.gateway,
	ae.server_name
FROM event_annotations a
JOIN action_events ae ON ae.id = a.action_event_id
WHERE ae.created_at_unix_milli >= ? AND ae.created_at_unix_milli <= ? AND a.type = 'governance_events'
ORDER BY ae.created_at_unix_milli ASC, a.id ASC
`
	if err := s.db.NewRaw(query, start, end).Scan(ctx, &rows); err != nil {
		return nil, err
	}

	// Action-event rows are stored one-per-finding, so collapse them back into a
	// single intervention per governed action event, keeping the highest severity.
	order := make([]string, 0)
	byEvent := make(map[string]*activityInterventionRow)
	for idx := range rows {
		row := &rows[idx]
		existing, ok := byEvent[row.ActionEventID]
		if !ok {
			byEvent[row.ActionEventID] = row
			order = append(order, row.ActionEventID)
			continue
		}
		if severityToScore(row.Severity) > severityToScore(existing.Severity) {
			byEvent[row.ActionEventID] = row
		}
	}

	interventions := make([]ActivityIntervention, 0, len(order))
	for _, eventID := range order {
		row := byEvent[eventID]
		score := severityToScore(row.Severity)
		label := ""
		if score >= 0.8 {
			label = actionLabel(row.Action)
		}
		interventions = append(interventions, ActivityIntervention{
			ID:                 eventID,
			Category:           normalizeCategory(row.Category),
			TimestampUnixMilli: row.CreatedAtUnixMilli,
			Severity:           score,
			Title:              interventionTitle(row),
			Summary:            interventionSummary(row),
			RuleID:             firstNonEmpty(row.Rule, row.Processor),
			RuleExplanation:    row.Message,
			ToolName:           row.ToolName,
			Gateway:            row.Gateway,
			ServerName:         row.ServerName,
			Label:              label,
			action:             row.Action,
		})
	}
	return interventions, nil
}

// activityVolume buckets inspected request volume across the window and returns the
// total number of inspected requests. Falls back to all action events when no rows
// are explicitly tagged as requests.
func (s *Store) activityVolume(ctx context.Context, start, end int64) ([]ActivityVolumePoint, int, error) {
	rows := make([]activityVolumeRow, 0)
	query := `SELECT created_at_unix_milli, message_type FROM action_events WHERE created_at_unix_milli >= ? AND created_at_unix_milli <= ?`
	if err := s.db.NewRaw(query, start, end).Scan(ctx, &rows); err != nil {
		return nil, 0, err
	}

	requestCount := 0
	for idx := range rows {
		if rows[idx].MessageType == "request" {
			requestCount++
		}
	}
	useRequestsOnly := requestCount > 0

	bucketMillis := (end - start) / activityVolumeBuckets
	if bucketMillis <= 0 {
		bucketMillis = 1
	}
	counts := make([]float64, activityVolumeBuckets)
	inspected := 0
	for idx := range rows {
		if useRequestsOnly && rows[idx].MessageType != "request" {
			continue
		}
		inspected++
		bucket := int((rows[idx].CreatedAtUnixMilli - start) / bucketMillis)
		if bucket < 0 {
			bucket = 0
		}
		if bucket >= activityVolumeBuckets {
			bucket = activityVolumeBuckets - 1
		}
		counts[bucket]++
	}

	volume := make([]ActivityVolumePoint, activityVolumeBuckets)
	for i := 0; i < activityVolumeBuckets; i++ {
		volume[i] = ActivityVolumePoint{
			TimeUnixMilli: start + int64(i)*bucketMillis,
			Volume:        counts[i],
		}
	}
	return volume, inspected, nil
}

// severityToScore maps stored severity strings to the 0..1 marker height the UI uses.
func severityToScore(severity string) float64 {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 1.0
	case "high":
		return 0.8
	case "medium", "moderate":
		return 0.55
	case "low":
		return 0.3
	case "info", "informational":
		return 0.2
	default:
		return 0.5
	}
}

// normalizeCategory maps a stored category onto the five UI categories. Unknown
// categories fall back to "policy"; this mapping is the tuning point as richer
// processors emit categories.
func normalizeCategory(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "security":
		return "security"
	case "risk":
		return "risk"
	case "quality":
		return "quality"
	case "compliance":
		return "compliance"
	case "policy":
		return "policy"
	default:
		return "policy"
	}
}

// actionLabel returns a short chart label for an action verb.
func actionLabel(action string) string {
	a := strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.Contains(a, "block"):
		return "Blocked"
	case strings.Contains(a, "redact"):
		return "Redacted"
	case strings.Contains(a, "hold") || strings.Contains(a, "held"):
		return "Held"
	case strings.Contains(a, "flag"):
		return "Flagged"
	case a == "":
		return ""
	default:
		return strings.ToUpper(a[:1]) + a[1:]
	}
}

func interventionTitle(row *activityInterventionRow) string {
	if title := humanizeToken(row.Processor); title != "" {
		return title
	}
	return humanizeToken(row.Category + " " + row.Action)
}

func interventionSummary(row *activityInterventionRow) string {
	verb := actionLabel(row.Action)
	category := normalizeCategory(row.Category)
	if verb == "" {
		return fmt.Sprintf("Intervention on %s traffic.", category)
	}
	return fmt.Sprintf("%s by the %s processor (%s).", verb, category, category)
}

func humanizeToken(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "_", " "), ".", " "))
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	for idx, part := range parts {
		parts[idx] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
