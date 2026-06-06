package builtinutil

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestApplyToolGuardBlocksToolOnlyMatch(t *testing.T) {
	ctx := contextWithToolCall(t, "crm___delete_user", map[string]any{"id": "u_123"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Mode:      ToolGuardModeBlock,
		Rules: []ToolGuardRule{
			{
				Name:         "block_delete_user_tool",
				Severity:     "high",
				Message:      "User deletion is not allowed through this gateway.",
				ToolPatterns: []string{"crm___delete_user", "delete_user"},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Matched)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, ctx.Payload.Result.IsError, true)
	assert.Equal(t, ctx.Event.Status, 403)
	assert.Equal(t, ctx.Annotations.Reports[0].Category, "policy")
	assert.Equal(t, ctx.Annotations.Reports[0].Severity, "high")
	structured := ctx.Payload.Result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["category"], "policy")
}

func TestApplyToolGuardUsesRuleCategory(t *testing.T) {
	ctx := contextWithToolCall(t, "crm___delete_user", map[string]any{"id": "u_123"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules: []ToolGuardRule{
			{
				Name:         "block_delete_user_tool",
				Category:     "security",
				Severity:     "high",
				ToolPatterns: []string{"crm___delete_user"},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, ctx.Annotations.Reports[0].Category, "security")
	structured := ctx.Payload.Result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["category"], "security")
}

func TestApplyToolGuardBlocksArgumentPathPresence(t *testing.T) {
	ctx := contextWithToolCall(t, "deploy___run", map[string]any{
		"environment": "staging",
	})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules: []ToolGuardRule{
			{
				Name:         "block_environment_arg",
				ToolPatterns: []string{"deploy___*"},
				ArgumentRules: []ToolGuardArgumentRule{
					{Path: "environment"},
				},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, result.Findings[0].Path, "payload.request.Params.arguments.environment")
}

func TestApplyToolGuardBlocksArgumentValuePattern(t *testing.T) {
	ctx := contextWithToolCall(t, "deploy___run", map[string]any{
		"target": "prod",
	})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules: []ToolGuardRule{
			{
				Name: "block_prod",
				ArgumentRules: []ToolGuardArgumentRule{
					{Pattern: `^prod$`},
				},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, result.Findings[0].Path, "payload.request.Params.arguments.target")
}

func TestApplyToolGuardBlocksCombinedToolAndArgumentRule(t *testing.T) {
	ctx := contextWithToolCall(t, "deploy___run", map[string]any{
		"environment": "prod",
	})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules: []ToolGuardRule{
			{
				Name:         "block_prod_deploy",
				ToolPatterns: []string{"deploy___*"},
				ArgumentRules: []ToolGuardArgumentRule{
					{Path: "environment", Pattern: `^prod$`},
				},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, result.RuleName, "block_prod_deploy")
}

func TestApplyToolGuardAnnotateModeDoesNotBlock(t *testing.T) {
	ctx := contextWithToolCall(t, "deploy___run", map[string]any{"environment": "prod"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Mode:      ToolGuardModeAnnotate,
		Rules: []ToolGuardRule{
			{
				Name:         "block_prod_deploy",
				ToolPatterns: []string{"deploy___*"},
				ArgumentRules: []ToolGuardArgumentRule{
					{Path: "environment", Pattern: `^prod$`},
				},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Matched)
	assert.Assert(t, !result.Blocked)
	assert.Assert(t, ctx.Payload.Result == nil)
	assert.Assert(t, ctx.Event == nil)
	assert.Equal(t, ctx.Annotations.Reports[0].Action, "annotated")
}

func TestApplyToolGuardNoopsOnResponsePhase(t *testing.T) {
	ctx := contextWithToolCall(t, "crm___delete_user", map[string]any{"id": "u_123"})
	ctx.Payload.Result = &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "ok"},
		},
	}

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules: []ToolGuardRule{
			{Name: "block_delete_user_tool", ToolPatterns: []string{"crm___delete_user"}},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, !result.Matched)
	assert.Assert(t, ctx.Annotations == nil)
}

func TestApplyToolGuardPathBoundaryBlocksTraversal(t *testing.T) {
	ctx := contextWithToolCall(t, "filesystem___read_file", map[string]any{"path": `..\secret.txt`})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules:     []ToolGuardRule{testPathBoundaryRule()},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, result.Findings[0].Rule, "path_boundary_traversal")
	assert.Equal(t, ctx.Annotations.Reports[0].Details["path_boundary_reason"], "path_traversal")
}

func TestApplyToolGuardPathBoundaryBlocksDeniedPaths(t *testing.T) {
	ctx := contextWithToolCall(t, "filesystem___read_file", map[string]any{"path": "/workspace/.git/config"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules: []ToolGuardRule{
			{
				Name:         "path_boundary",
				Severity:     "high",
				ToolPatterns: []string{"*file*", "*filesystem*"},
				PathBoundary: &PathBoundaryOptions{
					DeniedPaths: []string{".env", ".git/config", ".npmrc"},
				},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, result.Findings[0].Rule, "path_boundary_denied_path")
}

func TestApplyToolGuardPathBoundaryAllowsPathsUnderConfiguredRoots(t *testing.T) {
	ctx := contextWithToolCall(t, "filesystem___read_file", map[string]any{"path": "/workspace/src/main.go"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules:     []ToolGuardRule{testPathBoundaryRule()},
	})

	assert.NilError(t, err)
	assert.Assert(t, !result.Matched)
	assert.Assert(t, ctx.Payload.Result == nil)
}

func TestApplyToolGuardPathBoundaryBlocksPathsOutsideConfiguredRoots(t *testing.T) {
	ctx := contextWithToolCall(t, "filesystem___read_file", map[string]any{"path": "/etc/passwd"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules:     []ToolGuardRule{testPathBoundaryRule()},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Blocked)
	assert.Equal(t, result.Findings[0].Rule, "path_boundary_outside_allowed_roots")
	assert.Equal(t, ctx.Annotations.Reports[0].Details["path_boundary_resolved_path"], "/etc/passwd")
}

func TestApplyToolGuardPathBoundaryResolvesRelativePathsAgainstBaseRoot(t *testing.T) {
	ctx := contextWithToolCall(t, "filesystem___read_file", map[string]any{"path": "src/main.go"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules:     []ToolGuardRule{testPathBoundaryRule()},
	})

	assert.NilError(t, err)
	assert.Assert(t, !result.Matched)
	assert.Assert(t, ctx.Payload.Result == nil)
}

func TestApplyToolGuardPathBoundaryAnnotateModeDoesNotBlock(t *testing.T) {
	ctx := contextWithToolCall(t, "filesystem___read_file", map[string]any{"path": ".env"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Mode:      ToolGuardModeAnnotate,
		Rules: []ToolGuardRule{
			{
				Name:         "path_boundary",
				Severity:     "high",
				ToolPatterns: []string{"*file*", "*filesystem*"},
				PathBoundary: &PathBoundaryOptions{
					DeniedPaths: []string{".env"},
				},
			},
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result.Matched)
	assert.Assert(t, !result.Blocked)
	assert.Assert(t, ctx.Payload.Result == nil)
	assert.Assert(t, ctx.Event == nil)
	assert.Equal(t, ctx.Annotations.Reports[0].Action, "annotated")
}

func TestApplyToolGuardPathBoundaryIgnoresNonFilesystemTools(t *testing.T) {
	ctx := contextWithToolCall(t, "notes___create", map[string]any{"path": "../secret.txt"})

	result, err := ApplyToolGuard(ctx, ToolGuardOptions{
		Processor: "tool_call_guard",
		Rules:     []ToolGuardRule{testPathBoundaryRule()},
	})

	assert.NilError(t, err)
	assert.Assert(t, !result.Matched)
	assert.Assert(t, ctx.Annotations == nil)
}

func contextWithToolCall(t *testing.T, toolName string, args any) *DataContext {
	t.Helper()
	ctx := contextWithRequestArgs(t, args)
	ctx.Payload.Request.Params.Name = toolName
	ctx.Routing = &RoutingPart{
		ToolName:         toolName,
		OriginalToolname: toolName,
	}
	return ctx
}

func testPathBoundaryRule() ToolGuardRule {
	return ToolGuardRule{
		Name:         "path_boundary",
		Severity:     "high",
		ToolPatterns: []string{"*file*", "*filesystem*"},
		PathBoundary: &PathBoundaryOptions{
			AllowedRoots:     []string{"/workspace"},
			RelativeBaseRoot: "/workspace",
			DeniedPaths:      []string{".env", ".git/config", ".ssh", ".aws", "id_rsa", "id_ed25519"},
		},
	}
}
