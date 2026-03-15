package realworld

import (
	"context"
	"testing"
)

var sequentialThinkingManifest = serverManifest{
	Name:           "sequentialthinking",
	GatewayID:      "sequentialthinking",
	ServerID:       "sequentialthinking",
	CommandEnvVar:  "CENTIAN_SEQ_THINKING_SERVER_CMD",
	ArgsEnvVar:     "CENTIAN_SEQ_THINKING_SERVER_ARGS",
	DefaultCommand: "npx",
	DefaultArgs:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
	ExpectedTools:  []string{"sequentialthinking"},
	BuildFixture: func(t *testing.T) *fixtureBundle {
		t.Helper()
		return &fixtureBundle{}
	},
}

func TestSequentialThinkingToolCatalogParity(t *testing.T) {
	runServerComparison(t, sequentialThinkingManifest, "tool_catalog", func(ctx context.Context, t *testing.T, pair *connectionPair, _ *fixtureBundle) {
		assertToolCatalogParity(ctx, t, sequentialThinkingManifest, pair)
	})
}

func TestSequentialThinkingToolParity(t *testing.T) {
	testCases := []struct {
		name string
		args map[string]any
	}{
		{
			name: "linear",
			args: map[string]any{
				"thought":           "Break the work into two implementation steps.",
				"thoughtNumber":     1,
				"totalThoughts":     2,
				"nextThoughtNeeded": true,
			},
		},
		{
			name: "revision",
			args: map[string]any{
				"thought":           "Revising the earlier plan to remove a risky migration.",
				"thoughtNumber":     2,
				"totalThoughts":     3,
				"nextThoughtNeeded": true,
				"isRevision":        true,
				"revisesThought":    1,
			},
		},
		{
			name: "branch",
			args: map[string]any{
				"thought":           "Explore a browser-based alternative in a branch.",
				"thoughtNumber":     3,
				"totalThoughts":     3,
				"nextThoughtNeeded": true,
				"branchFromThought": 2,
				"branchId":          "browser-path",
			},
		},
		{
			name: "complete",
			args: map[string]any{
				"thought":           "The solution is good enough to finalize.",
				"thoughtNumber":     4,
				"totalThoughts":     4,
				"nextThoughtNeeded": false,
			},
		},
	}

	for _, tc := range testCases {
		runServerComparison(t, sequentialThinkingManifest, tc.name, func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
			assertToolCallParity(ctx, t, sequentialThinkingManifest, fixture, pair, "sequentialthinking", tc.args, tc.args)
		})
	}
}
