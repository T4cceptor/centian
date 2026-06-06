package builtinutil

import (
	"fmt"
	"path"
	"regexp"
	"strings"

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
	PathBoundary  *PathBoundaryOptions
}

// PathBoundaryOptions controls lexical filesystem path guard behavior.
type PathBoundaryOptions struct {
	AllowedRoots     []string
	RelativeBaseRoot string
	ArgumentPaths    []string
	DeniedPaths      []string
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
	details  map[string]any
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
	for key, value := range match.details {
		report.Details[key] = value
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
		if structured, ok := ctx.Payload.Result.StructuredContent.(map[string]any); ok {
			for key, value := range match.details {
				structured[key] = value
			}
		}
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
		findings, details, ok, err := matchingArgumentFindings(ctx, rule)
		if err != nil || !ok {
			return toolGuardMatch{}, false, err
		}
		if len(findings) == 0 {
			findings = []common.EventAnnotationFinding{{Rule: rule.Name, Path: toolName.pathOrDefault()}}
		}
		return toolGuardMatch{rule: rule, toolName: toolName.Value, toolPath: toolName.pathOrDefault(), findings: findings, details: details}, true, nil
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

func matchingArgumentFindings(ctx *DataContext, rule ToolGuardRule) ([]common.EventAnnotationFinding, map[string]any, bool, error) {
	if rule.PathBoundary != nil {
		return matchingPathBoundaryFindings(ctx, rule)
	}
	if len(rule.ArgumentRules) == 0 {
		return nil, nil, true, nil
	}

	var nodes []ArgumentNode
	if err := WalkRequestArgumentValues(ctx, func(node ArgumentNode) {
		nodes = append(nodes, node)
	}); err != nil {
		return nil, nil, false, err
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
	return findings, nil, len(findings) > 0, nil
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

func matchingPathBoundaryFindings(ctx *DataContext, rule ToolGuardRule) ([]common.EventAnnotationFinding, map[string]any, bool, error) {
	var nodes []ArgumentNode
	if err := WalkRequestArgumentValues(ctx, func(node ArgumentNode) {
		if node.Scalar {
			nodes = append(nodes, node)
		}
	}); err != nil {
		return nil, nil, false, err
	}

	options := rule.PathBoundary
	for _, node := range nodes {
		if !matchesPathBoundaryArgumentPath(options.ArgumentPaths, node.Path) {
			continue
		}
		check := checkPathBoundary(node.Text, options)
		if check.Rule == "" {
			continue
		}
		findingPath := requestArgumentPath(node.Path)
		return []common.EventAnnotationFinding{
				{Rule: check.Rule, Path: findingPath},
			}, map[string]any{
				"path_boundary_reason":          check.Reason,
				"path_boundary_original_path":   check.OriginalPath,
				"path_boundary_normalized_path": check.NormalizedPath,
				"path_boundary_resolved_path":   check.ResolvedPath,
				"path_boundary_matched_path":    findingPath,
				"path_boundary_allowed_roots":   options.AllowedRoots,
				"path_boundary_relative_base":   options.RelativeBaseRoot,
			}, true, nil
	}
	return nil, nil, false, nil
}

type pathBoundaryCheck struct {
	Rule           string
	Reason         string
	OriginalPath   string
	NormalizedPath string
	ResolvedPath   string
}

func checkPathBoundary(rawPath string, options *PathBoundaryOptions) pathBoundaryCheck {
	check := pathBoundaryCheck{OriginalPath: rawPath}
	normalizedInput := normalizeBoundaryPathInput(rawPath)
	check.NormalizedPath = normalizedInput
	if normalizedInput == "" {
		return check
	}
	if containsPathTraversal(normalizedInput) {
		check.Rule = "path_boundary_traversal"
		check.Reason = "path_traversal"
		return check
	}
	normalized := path.Clean(normalizedInput)
	check.NormalizedPath = normalized
	for _, deniedPath := range options.DeniedPaths {
		if boundaryPathContains(normalized, deniedPath) {
			check.Rule = "path_boundary_denied_path"
			check.Reason = "denied_path"
			return check
		}
	}
	if len(options.AllowedRoots) == 0 {
		return check
	}
	resolved := resolveBoundaryPath(normalized, options.RelativeBaseRoot)
	check.ResolvedPath = resolved
	if !isUnderAllowedRoot(resolved, options.AllowedRoots) {
		check.Rule = "path_boundary_outside_allowed_roots"
		check.Reason = "outside_allowed_roots"
	}
	return check
}

func normalizeBoundaryPathInput(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "/")
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	return value
}

func containsPathTraversal(value string) bool {
	if value == ".." || strings.HasPrefix(value, "../") {
		return true
	}
	return strings.Contains(value, "/../") || strings.HasSuffix(value, "/..")
}

func boundaryPathContains(value string, deniedPath string) bool {
	deniedPath = path.Clean(normalizeBoundaryPathInput(deniedPath))
	if deniedPath == "" {
		return false
	}
	value = strings.ToLower(value)
	deniedPath = strings.ToLower(deniedPath)
	return strings.Contains(value, deniedPath)
}

func resolveBoundaryPath(value string, relativeBaseRoot string) string {
	if strings.HasPrefix(value, "/") {
		return path.Clean(value)
	}
	if relativeBaseRoot == "" {
		return path.Clean(value)
	}
	return path.Clean(path.Join(relativeBaseRoot, value))
}

func isUnderAllowedRoot(value string, allowedRoots []string) bool {
	for _, root := range allowedRoots {
		root = path.Clean(root)
		if value == root || strings.HasPrefix(value, root+"/") {
			return true
		}
	}
	return false
}

func matchesPathBoundaryArgumentPath(patterns []string, argumentPath string) bool {
	if len(patterns) == 0 {
		return isPathLikeArgumentPath(argumentPath)
	}
	for _, pattern := range patterns {
		if matchesGlob(pattern, argumentPath) {
			return true
		}
	}
	return false
}

func isPathLikeArgumentPath(argumentPath string) bool {
	argumentPath = strings.ToLower(argumentPath)
	lastSegment := argumentPath
	if index := strings.LastIndex(lastSegment, "."); index >= 0 {
		lastSegment = lastSegment[index+1:]
	}
	return strings.Contains(lastSegment, "path") ||
		strings.Contains(lastSegment, "file") ||
		strings.Contains(lastSegment, "dir")
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
