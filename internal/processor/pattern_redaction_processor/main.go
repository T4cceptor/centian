package patternredactionprocessor

import (
	"fmt"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor/builtinutil"
)

// ProcessJSON processes one serialized Centian processor DataContext.
func ProcessJSON(input []byte, settings *config.BuiltinProcessorSettings) ([]byte, error) {
	ctx, err := builtinutil.DecodeContext(input)
	if err != nil {
		return nil, fmt.Errorf("decode processor input: %w", err)
	}

	_, err = builtinutil.ApplyPatternRedactions(ctx, &builtinutil.RedactionOptions{
		Processor: config.BuiltinPatternRedactionProcessor,
		Mode:      settings.Mode,
		Scope:     settings.Scope,
		Category:  "policy",
		Severity:  "low",
		Message:   "Configured redaction patterns matched MCP traffic.",
		Rules:     configRulesToBuiltinRules(settings.Rules),
	})
	if err != nil {
		return nil, err
	}

	output, err := builtinutil.EncodeContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("encode processor output: %w", err)
	}
	return output, nil
}

func configRulesToBuiltinRules(rules []config.BuiltinRedactionRule) []builtinutil.RedactionRule {
	out := make([]builtinutil.RedactionRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, builtinutil.RedactionRule{
			Name:        rule.Name,
			Pattern:     rule.Pattern,
			Replacement: rule.Replacement,
		})
	}
	return out
}
