package piiredactor

import (
	"fmt"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor/builtinutil"
)

var piiRules = []builtinutil.RedactionRule{
	{
		Name:        "email",
		Pattern:     `[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`,
		Replacement: "[REDACTED_EMAIL]",
	},
	{
		Name:        "iban",
		Pattern:     `\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b`,
		Replacement: "[REDACTED_IBAN]",
	},
	{
		Name:        "credit_card_like",
		Pattern:     `\b(?:\d[ -]*?){13,19}\b`,
		Replacement: "[REDACTED_CARD_LIKE_NUMBER]",
	},
	{
		Name:        "phone",
		Pattern:     `(?m)(?:\+\d{1,3}[\s.-]+)?(?:\(?\d{3}\)?[\s.-]+)\d{3}[\s.-]+\d{4}\b`,
		Replacement: "[REDACTED_PHONE]",
	},
}

// ProcessJSON processes one serialized Centian processor DataContext.
func ProcessJSON(input []byte, settings *config.BuiltinProcessorSettings) ([]byte, error) {
	ctx, err := builtinutil.DecodeContext(input)
	if err != nil {
		return nil, fmt.Errorf("decode processor input: %w", err)
	}

	_, err = builtinutil.ApplyPatternRedactions(ctx, builtinutil.RedactionOptions{
		Processor: config.BuiltinPIIRedactor,
		Mode:      settings.Mode,
		Scope:     settings.Scope,
		Category:  "policy",
		Severity:  "medium",
		Message:   "Potential PII was redacted from tool response content.",
		Rules:     piiRules,
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
