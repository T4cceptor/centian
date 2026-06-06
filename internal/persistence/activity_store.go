package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ActivityFilter bounds the activity aggregation to a time window (inclusive) and,
// optionally, to a single principal.
type ActivityFilter struct {
	Start     int64  // unix milli, inclusive
	End       int64  // unix milli, inclusive
	Principal string // when set, restrict to this principal_id
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

// activityTaskGovRow is a task event (with its related action event fields, if any)
// whose payload may carry governance annotations.
type activityTaskGovRow struct {
	TaskEventID     string          `bun:"task_event_id"`
	TaskCreatedAt   int64           `bun:"task_created_at"`
	ActionEventID   string          `bun:"action_event_id"`
	ActionCreatedAt int64           `bun:"action_created_at"`
	ToolName        string          `bun:"tool_name"`
	Gateway         string          `bun:"gateway"`
	ServerName      string          `bun:"server_name"`
	PayloadJSON     json.RawMessage `bun:"payload_json"`
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

	interventions, err := s.activityInterventions(ctx, start, end, filter.Principal)
	if err != nil {
		return nil, err
	}
	volume, requestsInspected, err := s.activityVolume(ctx, start, end, filter.Principal)
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

// rawGov is a normalized governance record from either source (action-event
// annotations or task-event payloads) before grouping into interventions.
type rawGov struct {
	key        string // grouping key: action event id when known, else task:<id>
	timestamp  int64
	category   string
	severity   string
	action     string
	processor  string
	message    string
	rule       string
	toolName   string
	gateway    string
	serverName string
}

func (s *Store) activityInterventions(ctx context.Context, start, end int64, principal string) ([]ActivityIntervention, error) {
	records := make([]rawGov, 0)

	// Source 1: governance annotations persisted against action events.
	annotationRows := make([]activityInterventionRow, 0)
	annotationQuery := `
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
WHERE ae.created_at_unix_milli >= ? AND ae.created_at_unix_milli <= ? AND a.type = 'governance_events'`
	annotationArgs := []any{start, end}
	if principal != "" {
		annotationQuery += " AND ae.principal_id = ?"
		annotationArgs = append(annotationArgs, principal)
	}
	annotationQuery += "\nORDER BY ae.created_at_unix_milli ASC, a.id ASC"
	if err := s.db.NewRaw(annotationQuery, annotationArgs...).Scan(ctx, &annotationRows); err != nil {
		return nil, err
	}
	for idx := range annotationRows {
		row := &annotationRows[idx]
		records = append(records, rawGov{
			key:        row.ActionEventID,
			timestamp:  row.CreatedAtUnixMilli,
			category:   row.Category,
			severity:   row.Severity,
			action:     row.Action,
			processor:  row.Processor,
			message:    row.Message,
			rule:       row.Rule,
			toolName:   row.ToolName,
			gateway:    row.Gateway,
			serverName: row.ServerName,
		})
	}

	// Source 2: governance annotations embedded in task-event payloads, related to
	// an action event by request id (mirrors how the Events view surfaces them).
	taskRows := make([]activityTaskGovRow, 0)
	taskQuery := `
SELECT
	te.id AS task_event_id,
	te.created_at_unix_milli AS task_created_at,
	COALESCE(ae.id, '') AS action_event_id,
	COALESCE(ae.created_at_unix_milli, 0) AS action_created_at,
	COALESCE(ae.tool_name, '') AS tool_name,
	COALESCE(ae.gateway, '') AS gateway,
	COALESCE(ae.server_name, '') AS server_name,
	te.payload_json
FROM task_events te
LEFT JOIN action_events ae ON ae.request_id = te.related_action_request_id
WHERE te.created_at_unix_milli >= ? AND te.created_at_unix_milli <= ? AND te.payload_json IS NOT NULL`
	taskArgs := []any{start, end}
	if principal != "" {
		taskQuery += " AND te.principal_id = ?"
		taskArgs = append(taskArgs, principal)
	}
	taskQuery += "\nORDER BY te.created_at_unix_milli ASC, te.id ASC"
	if err := s.db.NewRaw(taskQuery, taskArgs...).Scan(ctx, &taskRows); err != nil {
		return nil, err
	}
	for idx := range taskRows {
		row := &taskRows[idx]
		key := row.ActionEventID
		timestamp := row.ActionCreatedAt
		if key == "" {
			key = "task:" + row.TaskEventID
		}
		if timestamp == 0 {
			timestamp = row.TaskCreatedAt
		}
		for _, annotation := range governanceAnnotationsFromPayload(row.PayloadJSON) {
			rule := ""
			if len(annotation.Findings) > 0 {
				rule = annotation.Findings[0].Rule
			}
			records = append(records, rawGov{
				key:        key,
				timestamp:  timestamp,
				category:   annotation.Category,
				severity:   annotation.Severity,
				action:     annotation.Action,
				processor:  annotation.Processor,
				message:    annotation.Message,
				rule:       rule,
				toolName:   row.ToolName,
				gateway:    row.Gateway,
				serverName: row.ServerName,
			})
		}
	}

	// Collapse to a single intervention per key, keeping the highest severity.
	order := make([]string, 0)
	byKey := make(map[string]*rawGov)
	for idx := range records {
		record := &records[idx]
		existing, ok := byKey[record.key]
		if !ok {
			byKey[record.key] = record
			order = append(order, record.key)
			continue
		}
		if severityToScore(record.severity) > severityToScore(existing.severity) {
			byKey[record.key] = record
		}
	}

	interventions := make([]ActivityIntervention, 0, len(order))
	for _, key := range order {
		record := byKey[key]
		score := severityToScore(record.severity)
		label := ""
		if score >= 0.8 {
			label = actionLabel(record.action)
		}
		interventions = append(interventions, ActivityIntervention{
			ID:                 key,
			Category:           normalizeCategory(record.category),
			TimestampUnixMilli: record.timestamp,
			Severity:           score,
			Title:              interventionTitle(record),
			Summary:            interventionSummary(record),
			RuleID:             firstNonEmpty(record.rule, record.processor),
			RuleExplanation:    record.message,
			ToolName:           record.toolName,
			Gateway:            record.gateway,
			ServerName:         record.serverName,
			Label:              label,
			action:             record.action,
		})
	}
	// Order by time so the skyline plots markers left-to-right.
	sort.SliceStable(interventions, func(i, j int) bool {
		return interventions[i].TimestampUnixMilli < interventions[j].TimestampUnixMilli
	})
	return interventions, nil
}

// activityVolume buckets inspected request volume across the window and returns the
// total number of inspected requests. Falls back to all action events when no rows
// are explicitly tagged as requests.
func (s *Store) activityVolume(ctx context.Context, start, end int64, principal string) ([]ActivityVolumePoint, int, error) {
	rows := make([]activityVolumeRow, 0)
	query := `SELECT created_at_unix_milli, message_type FROM action_events WHERE created_at_unix_milli >= ? AND created_at_unix_milli <= ?`
	args := []any{start, end}
	if principal != "" {
		query += " AND principal_id = ?"
		args = append(args, principal)
	}
	if err := s.db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
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

// DistinctPrincipals returns the non-empty principal_ids that appear on action or
// task events. When both start and end are > 0 the scan is bounded to that window;
// otherwise it spans all events. Results are sorted and de-duplicated.
func (s *Store) DistinctPrincipals(ctx context.Context, start, end int64) ([]string, error) {
	windowed := start > 0 && end > 0
	timeClause := func(column string) string {
		if !windowed {
			return ""
		}
		return fmt.Sprintf(" AND %s >= ? AND %s <= ?", column, column)
	}
	query := `SELECT DISTINCT principal_id FROM action_events WHERE principal_id != ''` +
		timeClause("created_at_unix_milli") +
		` UNION SELECT DISTINCT principal_id FROM task_events WHERE principal_id != ''` +
		timeClause("created_at_unix_milli") +
		` ORDER BY principal_id ASC`

	args := []any{}
	if windowed {
		args = append(args, start, end, start, end)
	}

	principals := make([]string, 0)
	if err := s.db.NewRaw(query, args...).Scan(ctx, &principals); err != nil {
		return nil, err
	}
	return principals, nil
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

func interventionTitle(record *rawGov) string {
	if title := humanizeToken(record.processor); title != "" {
		return title
	}
	return humanizeToken(record.category + " " + record.action)
}

func interventionSummary(record *rawGov) string {
	verb := actionLabel(record.action)
	category := normalizeCategory(record.category)
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
