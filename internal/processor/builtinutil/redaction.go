package builtinutil

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/T4cceptor/centian/internal/common"
)

const (
	// RedactionModeRedact replaces matching text and records an annotation.
	RedactionModeRedact = "redact"
	// RedactionModeAnnotate records findings without mutating payloads.
	RedactionModeAnnotate = "annotate"

	// RedactionScopeRequest scans request arguments.
	RedactionScopeRequest = "request"
	// RedactionScopeResponse scans response text and structured content.
	RedactionScopeResponse = "response"
	// RedactionScopeBoth scans request and response payloads.
	RedactionScopeBoth = "both"
)

// RedactionRule describes one compiled literal-replacement regex rule.
type RedactionRule struct {
	Name        string
	Pattern     string
	Replacement string
	Regex       *regexp.Regexp
}

// RedactionOptions controls common regex redaction behavior.
type RedactionOptions struct {
	Processor string
	Mode      string
	Scope     string
	Category  string
	Severity  string
	Message   string
	Rules     []RedactionRule
}

// RedactionResult summarizes one redaction pass.
type RedactionResult struct {
	Matched     bool
	Modified    bool
	MatchCount  int
	Findings    []common.EventAnnotationFinding
	RuleNames   []string
	Paths       []string
	Annotations []common.EventAnnotation
}

// CompileRedactionRules compiles rule patterns for use by ApplyPatternRedactions.
func CompileRedactionRules(rules []RedactionRule) ([]RedactionRule, error) {
	compiled := make([]RedactionRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Regex != nil {
			compiled = append(compiled, rule)
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("redaction rule %q has invalid pattern: %w", rule.Name, err)
		}
		rule.Regex = re
		compiled = append(compiled, rule)
	}
	return compiled, nil
}

// ApplyPatternRedactions applies regex redaction rules and appends one annotation
// report when at least one match is found.
func ApplyPatternRedactions(ctx *DataContext, options RedactionOptions) (*RedactionResult, error) {
	options = normalizeRedactionOptions(options)
	compiledRules, err := CompileRedactionRules(options.Rules)
	if err != nil {
		return nil, err
	}
	options.Rules = compiledRules

	result := &RedactionResult{}
	if scansRequest(options.Scope) {
		modified, err := applyRequestRedactions(ctx, options, result)
		if err != nil {
			return nil, err
		}
		result.Modified = result.Modified || modified
	}
	if scansResponse(options.Scope) {
		result.Modified = applyResponseRedactions(ctx, options, result) || result.Modified
	}
	if !result.Matched {
		return result, nil
	}

	result.RuleNames = sortedStringSet(ruleNameSet(result.Findings))
	result.Paths = sortedStringSet(pathSet(result.Findings))
	report := common.EventAnnotation{
		Type:      "governance_events",
		Processor: options.Processor,
		Action:    redactionAction(options.Mode),
		Category:  options.Category,
		Severity:  options.Severity,
		Message:   redactionMessage(options, result),
		Findings:  result.Findings,
		Details: map[string]any{
			"mode":                options.Mode,
			"scope":               options.Scope,
			"match_count":         result.MatchCount,
			"unique_rule_count":   len(result.RuleNames),
			"rules":               result.RuleNames,
			"affected_path_count": len(result.Paths),
			"affected_paths":      result.Paths,
		},
	}
	AppendReport(ctx, report)
	result.Annotations = append(result.Annotations, report)
	if result.Modified {
		MarkModified(ctx)
	}
	return result, nil
}

func applyRequestRedactions(ctx *DataContext, options RedactionOptions, result *RedactionResult) (bool, error) {
	if options.Mode == RedactionModeAnnotate {
		err := WalkRequestArguments(ctx, func(node TextNode) {
			recordRedactionMatches(node, options.Rules, result)
		})
		return false, err
	}
	return ReplaceRequestArgumentStrings(ctx, func(node TextNode) TextReplacement {
		return redactNode(node, options.Rules, result)
	})
}

func applyResponseRedactions(ctx *DataContext, options RedactionOptions, result *RedactionResult) bool {
	if options.Mode == RedactionModeAnnotate {
		WalkResultText(ctx, func(node TextNode) {
			recordRedactionMatches(node, options.Rules, result)
		})
		WalkStructuredContent(ctx, func(node TextNode) {
			recordRedactionMatches(node, options.Rules, result)
		})
		return false
	}
	modified := ReplaceResultText(ctx, func(node TextNode) TextReplacement {
		return redactNode(node, options.Rules, result)
	})
	modified = ReplaceStructuredContentStrings(ctx, func(node TextNode) TextReplacement {
		return redactNode(node, options.Rules, result)
	}) || modified
	return modified
}

func redactNode(node TextNode, rules []RedactionRule, result *RedactionResult) TextReplacement {
	text := node.Text
	for _, rule := range rules {
		matches := rule.Regex.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			continue
		}
		recordRuleMatch(node.Path, rule.Name, len(matches), result)
		text = rule.Regex.ReplaceAllLiteralString(text, rule.Replacement)
	}
	return KeepText(text)
}

func recordRedactionMatches(node TextNode, rules []RedactionRule, result *RedactionResult) {
	for _, rule := range rules {
		matches := rule.Regex.FindAllStringIndex(node.Text, -1)
		if len(matches) == 0 {
			continue
		}
		recordRuleMatch(node.Path, rule.Name, len(matches), result)
	}
}

func recordRuleMatch(path, ruleName string, count int, result *RedactionResult) {
	result.Matched = true
	result.MatchCount += count
	result.Findings = append(result.Findings, common.EventAnnotationFinding{
		Rule: ruleName,
		Path: path,
	})
}

func normalizeRedactionOptions(options RedactionOptions) RedactionOptions {
	if options.Mode == "" {
		options.Mode = RedactionModeRedact
	}
	if options.Scope == "" {
		options.Scope = RedactionScopeBoth
	}
	if options.Category == "" {
		options.Category = "security"
	}
	if options.Severity == "" {
		options.Severity = "medium"
	}
	return options
}

func redactionAction(mode string) string {
	if mode == RedactionModeAnnotate {
		return "annotated"
	}
	return "redacted"
}

func redactionMessage(options RedactionOptions, result *RedactionResult) string {
	if options.Message != "" {
		return options.Message
	}
	return fmt.Sprintf(
		"%s matched %d text segment(s) across %d path(s).",
		options.Processor,
		result.MatchCount,
		len(result.Paths),
	)
}

func scansRequest(scope string) bool {
	return scope == RedactionScopeRequest || scope == RedactionScopeBoth
}

func scansResponse(scope string) bool {
	return scope == RedactionScopeResponse || scope == RedactionScopeBoth
}

func ruleNameSet(findings []common.EventAnnotationFinding) map[string]struct{} {
	values := map[string]struct{}{}
	for _, finding := range findings {
		values[finding.Rule] = struct{}{}
	}
	return values
}

func pathSet(findings []common.EventAnnotationFinding) map[string]struct{} {
	values := map[string]struct{}{}
	for _, finding := range findings {
		values[finding.Path] = struct{}{}
	}
	return values
}

func sortedStringSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
