package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

type detection struct {
	Pattern string
	Path    string
}

type config struct {
	Mode string
}

type scanPattern struct {
	name  string
	regex *regexp.Regexp
}

const (
	modeError  = "error"
	modeRedact = "redact"
	modeRemove = "remove"
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

func main() {
	cfg := parseConfig(os.Args[1:])
	if err := runWithConfig(os.Stdin, os.Stdout, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prompt injection guard failed: %v\n", err)
		os.Exit(1)
	}
}

func run(input io.Reader, output io.Writer) error {
	return runWithConfig(input, output, config{Mode: modeError})
}

func parseConfig(args []string) config {
	flags := flag.NewFlagSet("prompt-injection-guard", flag.ExitOnError)
	mode := flags.String("mode", modeError, "action for detections: error, redact, or remove")
	_ = flags.Parse(args)

	cfg := config{Mode: strings.ToLower(strings.TrimSpace(*mode))}
	switch cfg.Mode {
	case modeError, modeRedact, modeRemove:
	default:
		cfg.Mode = modeError
	}
	return cfg
}

func runWithConfig(input io.Reader, output io.Writer, cfg config) error {
	var ctx map[string]any
	if err := json.NewDecoder(input).Decode(&ctx); err != nil {
		return fmt.Errorf("decode processor input: %w", err)
	}

	detections := scanContext(ctx)
	if len(detections) > 0 {
		applyAction(ctx, detections, cfg.Mode)
	}

	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(ctx)
}

func applyAction(ctx map[string]any, detections []detection, mode string) {
	switch mode {
	case modeRedact, modeRemove:
		mutatePayload(ctx, mode)
		markModified(ctx)
	default:
		block(ctx, detections)
	}
	addAnnotation(ctx, detections, mode)
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
			detections = append(detections, detection{Pattern: pattern.name, Path: path})
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
		return !(r >= 'A' && r <= 'Z') &&
			!(r >= 'a' && r <= 'z') &&
			!(r >= '0' && r <= '9') &&
			r != '+' && r != '/' && r != '=' && r != '-' && r != '_'
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
	payload, _ := ensureMap(ctx, "payload")
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

	event, _ := ensureMap(ctx, "event")
	event["status"] = float64(403)
	event["success"] = false
	event["modified"] = true
}

func markModified(ctx map[string]any) {
	event, _ := ensureMap(ctx, "event")
	event["modified"] = true
}

func addAnnotation(ctx map[string]any, detections []detection, mode string) {
	annotations, _ := ensureMap(ctx, "annotations")
	reports, _ := annotations["reports"].([]any)
	reports = append(reports, map[string]any{
		"processor": "prompt_injection_guard",
		"action":    actionName(mode),
		"severity":  "high",
		"message":   "Obvious prompt injection markers were detected in tool data.",
		"findings":  findingSummary(detections),
	})
	annotations["reports"] = reports
}

func actionName(mode string) string {
	switch mode {
	case modeRedact:
		return "redacted"
	case modeRemove:
		return "removed"
	default:
		return "blocked"
	}
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

func ensureMap(parent map[string]any, key string) (map[string]any, bool) {
	if child, ok := childMap(parent, key); ok {
		return child, true
	}
	child := map[string]any{}
	parent[key] = child
	return child, false
}
