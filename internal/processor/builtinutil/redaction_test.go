package builtinutil

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestApplyPatternRedactionsRedactsRequestArguments(t *testing.T) {
	ctx := contextWithRequestArgs(t, map[string]any{
		"token": "it_abcdefghijklmnopqrst",
	})

	result, err := ApplyPatternRedactions(ctx, RedactionOptions{
		Processor: "pattern_redaction_processor",
		Mode:      RedactionModeRedact,
		Scope:     RedactionScopeRequest,
		Rules: []RedactionRule{
			{Name: "internal_token", Pattern: `it_[a-z]{20}`, Replacement: "[REDACTED_INTERNAL_TOKEN]"},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Matched)
	assert.Assert(t, result.Modified)
	assert.Assert(t, ctx.Event.Modified)
	args := decodeArgs(t, ctx)
	assert.Equal(t, args["token"], "[REDACTED_INTERNAL_TOKEN]")
	assert.Equal(t, ctx.Annotations.Reports[0].Action, "redacted")
	assert.Equal(t, ctx.Annotations.Reports[0].Findings[0].Rule, "internal_token")
}

func TestApplyPatternRedactionsRedactsResponseTextAndStructuredContent(t *testing.T) {
	ctx := &DataContext{
		Payload: &PayloadPart{
			Result: &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "email jane@example.com"},
				},
				StructuredContent: map[string]any{
					"owner": "jane@example.com",
				},
			},
		},
	}

	result, err := ApplyPatternRedactions(ctx, RedactionOptions{
		Processor: "pii_redactor",
		Mode:      RedactionModeRedact,
		Scope:     RedactionScopeResponse,
		Category:  "privacy",
		Rules: []RedactionRule{
			{Name: "email", Pattern: `[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`, Replacement: "[REDACTED_EMAIL]"},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Modified)
	assert.Equal(t, ctx.Payload.Result.Content[0].(*mcp.TextContent).Text, "email [REDACTED_EMAIL]")
	structured := ctx.Payload.Result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["owner"], "[REDACTED_EMAIL]")
	assert.Equal(t, ctx.Annotations.Reports[0].Category, "privacy")
	assert.Equal(t, ctx.Annotations.Reports[0].Details["affected_path_count"], 2)
}

func TestApplyPatternRedactionsAnnotateModeDoesNotMutate(t *testing.T) {
	ctx := contextWithRequestArgs(t, map[string]any{"token": "it_abcdefghijklmnopqrst"})

	result, err := ApplyPatternRedactions(ctx, RedactionOptions{
		Processor: "pattern_redaction_processor",
		Mode:      RedactionModeAnnotate,
		Scope:     RedactionScopeBoth,
		Rules: []RedactionRule{
			{Name: "internal_token", Pattern: `it_[a-z]{20}`, Replacement: "[REDACTED_INTERNAL_TOKEN]"},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Matched)
	assert.Assert(t, !result.Modified)
	assert.Assert(t, ctx.Event == nil)
	args := decodeArgs(t, ctx)
	assert.Equal(t, args["token"], "it_abcdefghijklmnopqrst")
	assert.Equal(t, ctx.Annotations.Reports[0].Action, "annotated")
}

func TestApplyPatternRedactionsScopeControlsPayloadSide(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"token": "it_abcdefghijklmnopqrst"})
	assert.NilError(t, err)
	ctx := &DataContext{
		Payload: &PayloadPart{
			Request: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      "tool",
					Arguments: raw,
				},
			},
			Result: &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "it_abcdefghijklmnopqrst"},
				},
			},
		},
	}

	result, err := ApplyPatternRedactions(ctx, RedactionOptions{
		Processor: "pattern_redaction_processor",
		Mode:      RedactionModeRedact,
		Scope:     RedactionScopeResponse,
		Rules: []RedactionRule{
			{Name: "internal_token", Pattern: `it_[a-z]{20}`, Replacement: "[REDACTED_INTERNAL_TOKEN]"},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Modified)
	args := decodeArgs(t, ctx)
	assert.Equal(t, args["token"], "it_abcdefghijklmnopqrst")
	assert.Equal(t, ctx.Payload.Result.Content[0].(*mcp.TextContent).Text, "[REDACTED_INTERNAL_TOKEN]")
}

func TestApplyPatternRedactionsInvalidRegex(t *testing.T) {
	_, err := ApplyPatternRedactions(&DataContext{}, RedactionOptions{
		Processor: "pattern_redaction_processor",
		Rules: []RedactionRule{
			{Name: "bad", Pattern: `[`, Replacement: "[REDACTED]"},
		},
	})

	assert.ErrorContains(t, err, "invalid pattern")
}
