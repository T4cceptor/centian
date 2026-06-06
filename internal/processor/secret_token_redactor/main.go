package secrettokenredactor

import (
	"fmt"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor/builtinutil"
)

var secretTokenRules = []builtinutil.RedactionRule{
	{
		Name:        "openai_api_key",
		Pattern:     `sk-[A-Za-z0-9_-]{20,}`,
		Replacement: "[REDACTED_OPENAI_API_KEY]",
	},
	{
		Name:        "anthropic_api_key",
		Pattern:     `sk-ant-[A-Za-z0-9_-]{20,}`,
		Replacement: "[REDACTED_ANTHROPIC_API_KEY]",
	},
	{
		Name:        "github_token",
		Pattern:     `gh[pousr]_[A-Za-z0-9_]{20,}`,
		Replacement: "[REDACTED_GITHUB_TOKEN]",
	},
	{
		Name:        "aws_access_key",
		Pattern:     `AKIA[0-9A-Z]{16}`,
		Replacement: "[REDACTED_AWS_ACCESS_KEY]",
	},
	{
		Name:        "bearer_token",
		Pattern:     `(?i)Bearer\s+[A-Za-z0-9._~+/=-]{20,}`,
		Replacement: "[REDACTED_BEARER_TOKEN]",
	},
	{
		Name:        "private_key",
		Pattern:     `-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]+?-----END [A-Z ]*PRIVATE KEY-----`,
		Replacement: "[REDACTED_PRIVATE_KEY]",
	},
	{
		Name:        "env_secret_assignment",
		Pattern:     `(?i)(api[_-]?key|secret|token|password)\s*=\s*['"]?[^'"\s]+`,
		Replacement: "[REDACTED_SECRET_ASSIGNMENT]",
	},
}

// ProcessJSON processes one serialized Centian processor DataContext.
func ProcessJSON(input []byte, settings *config.BuiltinProcessorSettings) ([]byte, error) {
	ctx, err := builtinutil.DecodeContext(input)
	if err != nil {
		return nil, fmt.Errorf("decode processor input: %w", err)
	}

	_, err = builtinutil.ApplyPatternRedactions(ctx, builtinutil.RedactionOptions{
		Processor: config.BuiltinSecretTokenRedactor,
		Mode:      settings.Mode,
		Scope:     settings.Scope,
		Category:  "security",
		Severity:  severityForContext(ctx),
		Message:   "Potential secrets were redacted from MCP traffic.",
		Rules:     secretTokenRules,
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

func severityForContext(ctx *builtinutil.DataContext) string {
	if ctx != nil && ctx.Event != nil {
		switch ctx.Event.Direction {
		case common.DirectionClientToServer:
			return "high"
		case common.DirectionServerToClient:
			return "medium"
		}
	}
	if ctx != nil && ctx.Payload != nil && ctx.Payload.Result != nil {
		return "medium"
	}
	return "high"
}
