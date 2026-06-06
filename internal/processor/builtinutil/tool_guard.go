package builtinutil

import (
	"fmt"
	"path"
	"regexp"

	"github.com/T4cceptor/centian/internal/common"
)

const (
	// ToolGuardModeBlock blocks matching tool calls.
	ToolGuardModeBlock = "block"
	// ToolGuardModeAnnotate records matching tool calls without blocking.
	ToolGuardModeAnnotate = "annotate"
)

// ToolGuardArgumentRule matches request argument paths, scalar values, or both.
type ToolGuardArgumentRule struct {
	Path    string
	Pattern string
	Regex   *regexp.Regexp
}

// ToolGuardRule describes one deny rule for tool calls.
type ToolGuardRule struct {
	Name          string
	Severity      string
	Message       string
	ToolPatterns  []string
	ArgumentRules []ToolGuardArgumentRule
}

// ToolGuardOptions controls generic tool-call guard behavior.
type ToolGuardOptions struct {
	Processor string
	Mode      string
	Rules     []ToolGuardRule
}

// ToolGuardResult summarizes one guard pass.
type ToolGuardResult struct {
	Matched     bool
	Blocked     bool
	RuleName    string
	ToolName    string
	Findings    []common.EventAnnotationFinding
	Annotations []common.EventAnnotation
}

type toolGuardMatch struct {
	rule     ToolGuardRule
	toolName string
	toolPath string
	findings []common.EventAnnotationFinding
}

type toolNameCandidate struct {
	Path  string
	Value string
}

// ApplyToolGuard evaluates deny rules against request-phase tool calls.
func ApplyToolGuard(ctx *DataContext, options ToolGuardOptions) (*ToolGuardResult, error) {
	options = normalizeToolGuardOptions(options)
	result := &ToolGuardResult{}
	if !isRequestPhaseToolCall(ctx) {
		return result, nil
	}

	compiled, err := CompileToolGuardRules(options.Rules)
	if err != nil {
		return nil, err
	}
	options.Rules = compiled

	match, ok, err := firstToolGuardMatch(ctx, options.Rules)
	if err != nil || !ok {
		return result, err
	}

	action := "blocked"
	if options.Mode == ToolGuardModeAnnotate {
		action = "annotated"
	}
	message := match.rule.Message
	if message == "" {
		message = fmt.Sprintf("Tool call matched policy rule %q.", match.rule.Name)
	}

	report := common.EventAnnotation{
		Type:      "governance_events",
		Processor: options.Processor,
		Action:    action,
		Category:  "policy",
		Severity:  ruleSeverity(match.rule),
		Message:   message,
		Findings:  match.findings,
		Details: map[string]any{
			"mode":                options.Mode,
			"matched_rule":        match.rule.Name,
			"tool_name":           match.toolName,
			"tool_name_path":      match.toolPath,
			"matched_paths":       findingPaths(match.findings),
			"matched_count":       len(match.findings),
			"tool_patterns":       match.rule.ToolPatterns,
			"argument_rule_count": len(match.rule.ArgumentRules),
		},
	}
	AppendReport(ctx, report)

	result.Matched = true
	result.Blocked = options.Mode == ToolGuardModeBlock
	result.RuleName = match.rule.Name
	result.ToolName = match.toolName
	result.Findings = match.findings
	result.Annotations = append(result.Annotations, report)

	if result.Blocked {
		BlockWithTextResult(ctx, BlockResultOptions{
			Processor: options.Processor,
			Message:   message,
			Status:    403,
			StructuredContent: map[string]any{
				"blocked":        true,
				"category":       "policy",
				"rule":           match.rule.Name,
				"severity":       ruleSeverity(match.rule),
				"tool_name":      match.toolName,
				"tool_name_path": match.toolPath,
				"findings":       match.findings,
				"matched_paths":  findingPaths(match.findings),
			},
		})
	}

	return result, nil
}

// CompileToolGuardRules compiles regexes in guard rules.
func CompileToolGuardRules(rules []ToolGuardRule) ([]ToolGuardRule, error) {
	compiled := make([]ToolGuardRule, 0, len(rules))
	for _, rule := range rules {
		next := rule
		next.ArgumentRules = make([]ToolGuardArgumentRule, 0, len(rule.ArgumentRules))
		for _, argumentRule := range rule.ArgumentRules {
			if argumentRule.Pattern != "" && argumentRule.Regex == nil {
				re, err := regexp.Compile(argumentRule.Pattern)
				if err != nil {
					return nil, fmt.Errorf("tool guard rule %q has invalid argument pattern: %w", rule.Name, err)
				}
				argumentRule.Regex = re
			}
			next.ArgumentRules = append(next.ArgumentRules, argumentRule)
		}
		compiled = append(compiled, next)
	}
	return compiled, nil
}

func normalizeToolGuardOptions(options ToolGuardOptions) ToolGuardOptions {
	if options.Mode == "" {
		options.Mode = ToolGuardModeBlock
	}
	return options
}

func isRequestPhaseToolCall(ctx *DataContext) bool {
	return ctx != nil &&
		ctx.Payload != nil &&
		ctx.Payload.Request != nil &&
		ctx.Payload.Request.Params != nil &&
		ctx.Payload.Result == nil
}

func firstToolGuardMatch(ctx *DataContext, rules []ToolGuardRule) (toolGuardMatch, bool, error) {
	candidates := toolNameCandidates(ctx)
	for _, rule := range rules {
		toolName, ok := matchingToolName(rule, candidates)
		if !ok {
			continue
		}
		findings, ok, err := matchingArgumentFindings(ctx, rule)
		if err != nil || !ok {
			return toolGuardMatch{}, false, err
		}
		if len(findings) == 0 {
			findings = []common.EventAnnotationFinding{{Rule: rule.Name, Path: toolName.pathOrDefault()}}
		}
		return toolGuardMatch{rule: rule, toolName: toolName.Value, toolPath: toolName.pathOrDefault(), findings: findings}, true, nil
	}
	return toolGuardMatch{}, false, nil
}

func toolNameCandidates(ctx *DataContext) []toolNameCandidate {
	values := []toolNameCandidate{}
	if ctx.Routing != nil {
		values = appendNonEmptyToolName(values, "routing.tool_name", ctx.Routing.ToolName)
		values = appendNonEmptyToolName(values, "routing.original_tool_name", ctx.Routing.OriginalToolname)
	}
	if ctx.Payload != nil && ctx.Payload.Request != nil && ctx.Payload.Request.Params != nil {
		values = appendNonEmptyToolName(values, "payload.request.Params.name", ctx.Payload.Request.Params.Name)
	}
	return values
}

func appendNonEmptyToolName(values []toolNameCandidate, candidatePath string, value string) []toolNameCandidate {
	if value == "" {
		return values
	}
	return append(values, toolNameCandidate{Path: candidatePath, Value: value})
}

func matchingToolName(rule ToolGuardRule, candidates []toolNameCandidate) (toolNameCandidate, bool) {
	if len(rule.ToolPatterns) == 0 {
		if len(candidates) == 0 {
			return toolNameCandidate{}, true
		}
		return candidates[0], true
	}
	for _, candidate := range candidates {
		for _, pattern := range rule.ToolPatterns {
			if matchesGlob(pattern, candidate.Value) {
				return candidate, true
			}
		}
	}
	return toolNameCandidate{}, false
}

func (candidate toolNameCandidate) pathOrDefault() string {
	if candidate.Path == "" {
		return "routing.tool_name"
	}
	return candidate.Path
}

func matchingArgumentFindings(ctx *DataContext, rule ToolGuardRule) ([]common.EventAnnotationFinding, bool, error) {
	if len(rule.ArgumentRules) == 0 {
		return nil, true, nil
	}

	var nodes []ArgumentNode
	if err := WalkRequestArgumentValues(ctx, func(node ArgumentNode) {
		nodes = append(nodes, node)
	}); err != nil {
		return nil, false, err
	}

	findings := []common.EventAnnotationFinding{}
	for _, argumentRule := range rule.ArgumentRules {
		for _, node := range nodes {
			if !argumentRuleMatchesNode(argumentRule, node) {
				continue
			}
			findings = append(findings, common.EventAnnotationFinding{
				Rule: rule.Name,
				Path: requestArgumentPath(node.Path),
			})
		}
	}
	return findings, len(findings) > 0, nil
}

func argumentRuleMatchesNode(rule ToolGuardArgumentRule, node ArgumentNode) bool {
	if rule.Path != "" && !matchesGlob(rule.Path, node.Path) {
		return false
	}
	if rule.Pattern == "" {
		return true
	}
	return node.Scalar && rule.Regex.MatchString(node.Text)
}

func matchesGlob(pattern, value string) bool {
	matched, err := path.Match(pattern, value)
	return err == nil && matched
}

func requestArgumentPath(path string) string {
	if path == "" {
		return "payload.request.Params.arguments"
	}
	return "payload.request.Params.arguments." + path
}

func ruleSeverity(rule ToolGuardRule) string {
	if rule.Severity == "" {
		return "medium"
	}
	return rule.Severity
}

func findingPaths(findings []common.EventAnnotationFinding) []string {
	paths := make([]string, 0, len(findings))
	seen := map[string]struct{}{}
	for _, finding := range findings {
		if _, exists := seen[finding.Path]; exists {
			continue
		}
		seen[finding.Path] = struct{}{}
		paths = append(paths, finding.Path)
	}
	return paths
}
