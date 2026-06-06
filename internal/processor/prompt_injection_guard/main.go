package promptinjectionguard

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/processor/builtinutil"
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
	ctx, err := builtinutil.DecodeContext(input)
	if err != nil {
		return nil, fmt.Errorf("decode processor input: %w", err)
	}

	mode = NormalizeMode(mode)
	detections, err := scanContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(detections) > 0 {
		if err := applyAction(ctx, detections, mode); err != nil {
			return nil, err
		}
	}

	output, err := builtinutil.EncodeContext(ctx)
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

func applyAction(ctx *builtinutil.DataContext, detections []detection, mode string) error {
	addAnnotation(ctx, detections, mode)
	switch mode {
	case modeAnnotate:
		// Observe-only mode intentionally leaves the MCP payload and event status unchanged.
	case modeRedact, modeRemove:
		if err := mutatePayload(ctx, mode); err != nil {
			return err
		}
		builtinutil.MarkModified(ctx)
	default:
		block(ctx, detections)
	}
	return nil
}

func scanContext(ctx *builtinutil.DataContext) ([]detection, error) {
	if ctx == nil || ctx.Payload == nil {
		return nil, nil
	}
	if ctx.Payload.Result != nil {
		return scanResult(ctx), nil
	}
	if ctx.Payload.Request != nil {
		return scanRequest(ctx)
	}
	return nil, nil
}

func scanRequest(ctx *builtinutil.DataContext) ([]detection, error) {
	var detections []detection
	err := builtinutil.WalkRequestArguments(ctx, func(node builtinutil.TextNode) {
		detections = append(detections, scanText(node.Text, node.Path)...)
	})
	return detections, err
}

func scanResult(ctx *builtinutil.DataContext) []detection {
	var detections []detection
	builtinutil.WalkResultText(ctx, func(node builtinutil.TextNode) {
		detections = append(detections, scanText(node.Text, node.Path)...)
	})
	builtinutil.WalkStructuredContent(ctx, func(node builtinutil.TextNode) {
		detections = append(detections, scanText(node.Text, node.Path)...)
	})
	return detections
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

func mutatePayload(ctx *builtinutil.DataContext, mode string) error {
	if ctx == nil || ctx.Payload == nil {
		return nil
	}
	if ctx.Payload.Result != nil {
		sanitizeResult(ctx, mode)
		return nil
	}
	if ctx.Payload.Request != nil {
		return sanitizeRequest(ctx, mode)
	}
	return nil
}

func sanitizeRequest(ctx *builtinutil.DataContext, mode string) error {
	_, err := builtinutil.ReplaceRequestArgumentStrings(ctx, func(node builtinutil.TextNode) builtinutil.TextReplacement {
		return sanitizeText(node.Text, mode)
	})
	return err
}

func sanitizeResult(ctx *builtinutil.DataContext, mode string) {
	builtinutil.ReplaceResultText(ctx, func(node builtinutil.TextNode) builtinutil.TextReplacement {
		return sanitizeText(node.Text, mode)
	})
	builtinutil.ReplaceStructuredContentStrings(ctx, func(node builtinutil.TextNode) builtinutil.TextReplacement {
		return sanitizeText(node.Text, mode)
	})
}

func sanitizeText(text string, mode string) builtinutil.TextReplacement {
	if len(scanText(text, "")) == 0 {
		return builtinutil.KeepText(text)
	}
	if mode == modeRedact {
		return builtinutil.KeepText("[PROMPT_INJECTION_REDACTED]")
	}
	return builtinutil.DropText()
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

func block(ctx *builtinutil.DataContext, detections []detection) {
	builtinutil.BlockWithTextResult(ctx, builtinutil.BlockResultOptions{
		Processor: "prompt_injection_guard",
		Message:   "Blocked by Centian prompt injection guard: obvious prompt injection markers were detected in tool data.",
		Status:    403,
		StructuredContent: map[string]any{
			"blocked":    true,
			"processor":  "prompt_injection_guard",
			"detections": detectionSummary(detections),
		},
	})
}

func addAnnotation(ctx *builtinutil.DataContext, detections []detection, mode string) {
	details := annotationDetails(ctx, detections, mode)
	builtinutil.AppendReport(ctx, common.EventAnnotation{
		Type:      "governance_events",
		Processor: "prompt_injection_guard",
		Action:    actionName(mode),
		Category:  "security",
		Severity:  severityFor(detections, details),
		Message:   annotationMessage(detections, details),
		Findings:  findingSummary(detections),
		Details:   details,
	})
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
		return "high"
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

func findingSummary(detections []detection) []common.EventAnnotationFinding {
	summary := make([]common.EventAnnotationFinding, 0, len(detections))
	for _, detection := range detections {
		summary = append(summary, common.EventAnnotationFinding{
			Rule: detection.Pattern,
			Path: detection.Path,
		})
	}
	return summary
}

func annotationDetails(ctx *builtinutil.DataContext, detections []detection, mode string) map[string]any {
	totalBytes := builtinutil.TotalScannedTextBytes(ctx)
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
