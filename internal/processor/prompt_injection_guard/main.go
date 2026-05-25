package promptinjectionguard

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type detection struct {
	Pattern string
	Path    string
	Length  int
}

type scanPattern struct {
	name  string
	regex *regexp.Regexp
}

const (
	modeAnnotate = "annotate"
	modeError    = "error"
	modeRedact   = "redact"
	modeRemove   = "remove"
)

var patterns = []scanPattern{
	{
		name:  "ignore_previous_instructions",
		regex: regexp.MustCompile(`(?i)\b(ignore|disregard|forget)\s+(all\s+)?(previous|prior|above|earlier)\s+(instructions|rules|directions|process)\b`),
	},
	{
		name:  "role_marker",
		regex: regexp.MustCompile(`(?i)(^|\s)(system|developer|assistant)\s*:\s*`),
	},
	{
		name:  "xml_role_tag",
		regex: regexp.MustCompile(`(?i)</?(system|developer|assistant|instruction|instructions)\b[^>]*>`),
	},
	{
		name:  "inst_marker",
		regex: regexp.MustCompile(`(?i)\[/?(INST|SYS|SYSTEM)\]`),
	},
	{
		name:  "secret_exfiltration",
		regex: regexp.MustCompile(`(?i)\b(reveal|leak|print|show|exfiltrate|send)\b.{0,80}\b(system prompt|developer prompt|hidden instructions|secret|api key|token)\b`),
	},
	{
		name:  "instruction_override",
		regex: regexp.MustCompile(`(?i)\b(new|updated)\s+(system|developer)\s+(instruction|message)\b|\byou\s+are\s+now\b`),
	},
}

// ProcessJSON processes one serialized Centian processor DataContext.
func ProcessJSON(input []byte, mode string) ([]byte, error) {
	var ctx map[string]any
	if err := json.Unmarshal(input, &ctx); err != nil {
		return nil, fmt.Errorf("decode processor input: %w", err)
	}

	mode = NormalizeMode(mode)
	detections := scanContext(ctx)
	if len(detections) > 0 {
		applyAction(ctx, detections, mode)
	}

	output, err := json.Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("encode processor output: %w", err)
	}
	return output, nil
}

// NormalizeMode returns a supported processor mode, defaulting to error.
func NormalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case modeAnnotate, modeError, modeRedact, modeRemove:
		return mode
	default:
		return modeError
	}
}

func applyAction(ctx map[string]any, detections []detection, mode string) {
	addAnnotation(ctx, detections, mode)
	switch mode {
	case modeAnnotate:
		// Observe-only mode intentionally leaves the MCP payload and event status unchanged.
	case modeRedact, modeRemove:
		mutatePayload(ctx, mode)
		markModified(ctx)
	default:
		block(ctx, detections)
	}
}

func scanContext(ctx map[string]any) []detection {
	payload, ok := childMap(ctx, "payload")
	if !ok {
		return nil
	}

	if result, ok := childMap(payload, "result"); ok {
		return scanResult(result)
	}

	if request, ok := childMap(payload, "request"); ok {
		return scanRequest(request)
	}
	return nil
}

func scanRequest(request map[string]any) []detection {
	params, ok := childMap(request, "Params")
	if !ok {
		return nil
	}
	arguments, ok := params["arguments"]
	if !ok {
		return nil
	}
	return scanValue(arguments, "payload.request.Params.arguments")
}

func scanResult(result map[string]any) []detection {
	var detections []detection
	if content, ok := result["content"].([]any); ok {
		for i, item := range content {
			entry, ok := item.(map[string]any)
			if !ok || entry["type"] != "text" {
				continue
			}
			text, ok := entry["text"].(string)
			if !ok {
				continue
			}
			path := fmt.Sprintf("payload.result.content[%d].text", i)
			detections = append(detections, scanText(text, path)...)
		}
	}
	if structured, ok := result["structuredContent"]; ok {
		detections = append(detections, scanValue(structured, "payload.result.structuredContent")...)
	}
	return detections
}

func scanValue(value any, path string) []detection {
	switch typed := value.(type) {
	case string:
		return scanText(typed, path)
	case []any:
		var detections []detection
		for i, item := range typed {
			detections = append(detections, scanValue(item, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return detections
	case map[string]any:
		var detections []detection
		for key, item := range typed {
			detections = append(detections, scanValue(item, path+"."+key)...)
		}
		return detections
	default:
		return nil
	}
}

func scanText(text, path string) []detection {
	candidates := []string{text}
	if decoded, ok := urlDecode(text); ok {
		candidates = append(candidates, decoded)
	}
	candidates = append(candidates, base64Candidates(text)...)

	seen := map[string]struct{}{}
	var detections []detection
	for _, candidate := range candidates {
		for _, pattern := range patterns {
			if !pattern.regex.MatchString(candidate) {
				continue
			}
			key := path + "\x00" + pattern.name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			detections = append(detections, detection{Pattern: pattern.name, Path: path, Length: len(text)})
		}
	}
	return detections
}

func mutatePayload(ctx map[string]any, mode string) {
	payload, ok := childMap(ctx, "payload")
	if !ok {
		return
	}

	if result, ok := childMap(payload, "result"); ok {
		sanitizeResult(result, mode)
		return
	}
	if request, ok := childMap(payload, "request"); ok {
		sanitizeRequest(request, mode)
	}
}

func sanitizeRequest(request map[string]any, mode string) {
	params, ok := childMap(request, "Params")
	if !ok {
		return
	}
	arguments, ok := params["arguments"]
	if !ok {
		return
	}
	sanitized, keep := sanitizeValue(arguments, mode)
	if keep {
		params["arguments"] = sanitized
		return
	}
	delete(params, "arguments")
}

func sanitizeResult(result map[string]any, mode string) {
	if content, ok := result["content"].([]any); ok {
		sanitized := make([]any, 0, len(content))
		for _, item := range content {
			entry, ok := item.(map[string]any)
			if !ok || entry["type"] != "text" {
				sanitized = append(sanitized, item)
				continue
			}
			text, ok := entry["text"].(string)
			if !ok || len(scanText(text, "")) == 0 {
				sanitized = append(sanitized, item)
				continue
			}
			if mode == modeRedact {
				entry["text"] = "[PROMPT_INJECTION_REDACTED]"
				sanitized = append(sanitized, entry)
			}
		}
		result["content"] = sanitized
	}

	if structured, ok := result["structuredContent"]; ok {
		sanitized, keep := sanitizeValue(structured, mode)
		if keep {
			result["structuredContent"] = sanitized
			return
		}
		delete(result, "structuredContent")
	}
}

func sanitizeValue(value any, mode string) (any, bool) {
	switch typed := value.(type) {
	case string:
		if len(scanText(typed, "")) == 0 {
			return typed, true
		}
		if mode == modeRedact {
			return "[PROMPT_INJECTION_REDACTED]", true
		}
		return nil, false
	case []any:
		sanitized := make([]any, 0, len(typed))
		for _, item := range typed {
			next, keep := sanitizeValue(item, mode)
			if keep {
				sanitized = append(sanitized, next)
			}
		}
		return sanitized, true
	case map[string]any:
		sanitized := make(map[string]any, len(typed))
		for key, item := range typed {
			next, keep := sanitizeValue(item, mode)
			if keep {
				sanitized[key] = next
			}
		}
		return sanitized, true
	default:
		return typed, true
	}
}

func urlDecode(text string) (string, bool) {
	if !strings.Contains(text, "%") && !strings.Contains(text, "+") {
		return "", false
	}
	decoded, err := url.QueryUnescape(text)
	if err != nil || decoded == text {
		return "", false
	}
	return decoded, true
}

func base64Candidates(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !isBase64TokenRune(r)
	})

	var candidates []string
	for _, field := range fields {
		if len(field) < 16 || len(field)%4 != 0 {
			continue
		}
		if decoded, ok := decodeBase64(field); ok {
			candidates = append(candidates, decoded)
		}
	}
	return candidates
}

func isBase64TokenRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		r == '+' || r == '/' || r == '=' || r == '-' || r == '_'
}

func decodeBase64(value string) (string, bool) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
	}
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil && mostlyPrintable(decoded) {
			return string(decoded), true
		}
	}
	return "", false
}

func mostlyPrintable(value []byte) bool {
	if len(value) == 0 {
		return false
	}
	printable := 0
	for _, b := range value {
		if b == '\n' || b == '\r' || b == '\t' || (b >= 32 && b <= 126) {
			printable++
		}
	}
	return float64(printable)/float64(len(value)) > 0.85
}

func block(ctx map[string]any, detections []detection) {
	payload := ensureMap(ctx, "payload")
	payload["result"] = map[string]any{
		"content": []any{
			map[string]any{
				"type": "text",
				"text": "Blocked by Centian prompt injection guard: obvious prompt injection markers were detected in tool data.",
			},
		},
		"isError": true,
		"structuredContent": map[string]any{
			"blocked":    true,
			"processor":  "prompt_injection_guard",
			"detections": detectionSummary(detections),
		},
	}

	event := ensureMap(ctx, "event")
	event["status"] = float64(403)
	event["success"] = false
	event["modified"] = true
}

func markModified(ctx map[string]any) {
	event := ensureMap(ctx, "event")
	event["modified"] = true
}

func addAnnotation(ctx map[string]any, detections []detection, mode string) {
	annotations := ensureMap(ctx, "annotations")
	reports, _ := annotations["reports"].([]any)
	details := annotationDetails(ctx, detections, mode)
	reports = append(reports, map[string]any{
		"type":      "governance_events",
		"processor": "prompt_injection_guard",
		"action":    actionName(mode),
		"category":  "security",
		"severity":  severityFor(detections, details),
		"message":   annotationMessage(detections, details),
		"findings":  findingSummary(detections),
		"details":   details,
	})
	annotations["reports"] = reports
}

func actionName(mode string) string {
	switch mode {
	case modeAnnotate:
		return "annotated"
	case modeRedact:
		return "redacted"
	case modeRemove:
		return "removed"
	default:
		return "blocked"
	}
}

func severityFor(detections []detection, details map[string]any) string {
	rules := uniqueDetectionRules(detections)
	if rules["secret_exfiltration"] || (rules["role_marker"] && rules["ignore_previous_instructions"]) {
		return "critical"
	}
	if len(detections) >= 3 || intFromDetails(details, "affected_path_count") >= 2 || floatFromDetails(details, "flagged_text_ratio") >= 0.25 {
		return "high"
	}
	return "medium"
}

func annotationMessage(detections []detection, details map[string]any) string {
	return fmt.Sprintf(
		"Prompt injection markers were detected: %d evidence item(s) across %d path(s).",
		len(detections),
		intFromDetails(details, "affected_path_count"),
	)
}

func findingSummary(detections []detection) []map[string]string {
	summary := make([]map[string]string, 0, len(detections))
	for _, detection := range detections {
		summary = append(summary, map[string]string{
			"rule": detection.Pattern,
			"path": detection.Path,
		})
	}
	return summary
}

func annotationDetails(ctx map[string]any, detections []detection, mode string) map[string]any {
	totalBytes := totalScannedTextBytes(ctx)
	flaggedBytes := flaggedTextBytes(detections)
	ratio := 0.0
	if totalBytes > 0 {
		ratio = float64(flaggedBytes) / float64(totalBytes)
	}

	affectedPaths := uniqueDetectionPaths(detections)
	rules := sortedRuleNames(uniqueDetectionRules(detections))
	return map[string]any{
		"mode":                     mode,
		"evidence_count":           len(detections),
		"unique_rule_count":        len(rules),
		"rules":                    rules,
		"affected_path_count":      len(affectedPaths),
		"affected_paths":           affectedPaths,
		"flagged_text_bytes":       flaggedBytes,
		"total_scanned_text_bytes": totalBytes,
		"flagged_text_ratio":       ratio,
		"source":                   detectionSource(detections),
	}
}

func totalScannedTextBytes(ctx map[string]any) int {
	payload, ok := childMap(ctx, "payload")
	if !ok {
		return 0
	}
	if result, ok := childMap(payload, "result"); ok {
		return resultTextBytes(result)
	}
	if request, ok := childMap(payload, "request"); ok {
		params, ok := childMap(request, "Params")
		if !ok {
			return 0
		}
		return valueTextBytes(params["arguments"])
	}
	return 0
}

func resultTextBytes(result map[string]any) int {
	total := 0
	if content, ok := result["content"].([]any); ok {
		for _, item := range content {
			entry, ok := item.(map[string]any)
			if !ok || entry["type"] != "text" {
				continue
			}
			if text, ok := entry["text"].(string); ok {
				total += len(text)
			}
		}
	}
	total += valueTextBytes(result["structuredContent"])
	return total
}

func valueTextBytes(value any) int {
	switch typed := value.(type) {
	case string:
		return len(typed)
	case []any:
		total := 0
		for _, item := range typed {
			total += valueTextBytes(item)
		}
		return total
	case map[string]any:
		total := 0
		for _, item := range typed {
			total += valueTextBytes(item)
		}
		return total
	default:
		return 0
	}
}

func flaggedTextBytes(detections []detection) int {
	byPath := map[string]int{}
	for _, detection := range detections {
		if detection.Length > byPath[detection.Path] {
			byPath[detection.Path] = detection.Length
		}
	}
	total := 0
	for _, length := range byPath {
		total += length
	}
	return total
}

func uniqueDetectionPaths(detections []detection) []string {
	seen := map[string]struct{}{}
	for _, detection := range detections {
		seen[detection.Path] = struct{}{}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func uniqueDetectionRules(detections []detection) map[string]bool {
	rules := map[string]bool{}
	for _, detection := range detections {
		rules[detection.Pattern] = true
	}
	return rules
}

func sortedRuleNames(rules map[string]bool) []string {
	names := make([]string, 0, len(rules))
	for rule := range rules {
		names = append(names, rule)
	}
	sort.Strings(names)
	return names
}

func detectionSource(detections []detection) string {
	source := ""
	for _, detection := range detections {
		next := "unknown"
		switch {
		case strings.HasPrefix(detection.Path, "payload.request."):
			next = "request"
		case strings.HasPrefix(detection.Path, "payload.result."):
			next = "result"
		}
		if source == "" {
			source = next
			continue
		}
		if source != next {
			return "mixed"
		}
	}
	if source == "" {
		return "unknown"
	}
	return source
}

func intFromDetails(details map[string]any, key string) int {
	value, _ := details[key].(int)
	return value
}

func floatFromDetails(details map[string]any, key string) float64 {
	value, _ := details[key].(float64)
	return value
}

func detectionSummary(detections []detection) []map[string]string {
	sort.SliceStable(detections, func(i, j int) bool {
		if detections[i].Path == detections[j].Path {
			return detections[i].Pattern < detections[j].Pattern
		}
		return detections[i].Path < detections[j].Path
	})

	summary := make([]map[string]string, 0, len(detections))
	for _, detection := range detections {
		summary = append(summary, map[string]string{
			"pattern": detection.Pattern,
			"path":    detection.Path,
		})
	}
	return summary
}

func childMap(parent map[string]any, key string) (map[string]any, bool) {
	value, ok := parent[key]
	if !ok {
		return nil, false
	}
	child, ok := value.(map[string]any)
	return child, ok
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if child, ok := childMap(parent, key); ok {
		return child
	}
	child := map[string]any{}
	parent[key] = child
	return child
}
