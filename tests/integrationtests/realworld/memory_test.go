package realworld

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var memoryManifest = serverManifest{
	Name:           "memory",
	GatewayID:      "memory",
	ServerID:       "memory",
	CommandEnvVar:  "CENTIAN_MEMORY_SERVER_CMD",
	ArgsEnvVar:     "CENTIAN_MEMORY_SERVER_ARGS",
	DefaultCommand: "npx",
	DefaultArgs:    []string{"-y", "@modelcontextprotocol/server-memory"},
	ExpectedTools: []string{
		"add_observations",
		"create_entities",
		"create_relations",
		"delete_entities",
		"delete_observations",
		"delete_relations",
		"open_nodes",
		"read_graph",
		"search_nodes",
	},
	BuildFixture: buildMemoryFixture,
	Normalize:    normalizeMemoryResult,
}

func TestMemoryToolCatalogParity(t *testing.T) {
	runServerComparison(t, memoryManifest, "tool_catalog", func(ctx context.Context, t *testing.T, pair *connectionPair, _ *fixtureBundle) {
		assertToolCatalogParity(ctx, t, memoryManifest, pair)
	})
}

func TestMemoryScenarioParity(t *testing.T) {
	runServerComparison(t, memoryManifest, "scenario", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		steps := []struct {
			tool string
			args map[string]any
		}{
			{
				tool: "create_entities",
				args: map[string]any{
					"entities": []map[string]any{
						{
							"name":         "Alice",
							"entityType":   "person",
							"observations": []string{"Likes Go", "Maintains Centian"},
						},
						{
							"name":         "Centian",
							"entityType":   "project",
							"observations": []string{"Is an MCP proxy"},
						},
					},
				},
			},
			{
				tool: "create_relations",
				args: map[string]any{
					"relations": []map[string]any{
						{"from": "Alice", "to": "Centian", "relationType": "maintains"},
					},
				},
			},
			{
				tool: "add_observations",
				args: map[string]any{
					"observations": []map[string]any{
						{"entityName": "Alice", "contents": []string{"Prefers concise plans"}},
					},
				},
			},
			{
				tool: "search_nodes",
				args: map[string]any{
					"query": "Centian",
				},
			},
			{
				tool: "open_nodes",
				args: map[string]any{
					"names": []string{"Alice", "Centian"},
				},
			},
			{
				tool: "delete_observations",
				args: map[string]any{
					"deletions": []map[string]any{
						{"entityName": "Alice", "observations": []string{"Likes Go"}},
					},
				},
			},
			{
				tool: "delete_relations",
				args: map[string]any{
					"relations": []map[string]any{
						{"from": "Alice", "to": "Centian", "relationType": "maintains"},
					},
				},
			},
			{
				tool: "delete_entities",
				args: map[string]any{
					"entityNames": []string{"Centian"},
				},
			},
			{
				tool: "read_graph",
				args: map[string]any{},
			},
		}

		for _, step := range steps {
			directResult, directErr := pair.Direct.session.CallTool(ctx, &mcp.CallToolParams{Name: step.tool, Arguments: step.args})
			proxiedResult, proxiedErr := pair.Proxied.session.CallTool(ctx, &mcp.CallToolParams{Name: step.tool, Arguments: step.args})
			assertCallOutcomeParity(t, memoryManifest, fixture, directResult, directErr, proxiedResult, proxiedErr)
		}
	})
}

func buildMemoryFixture(t *testing.T) *fixtureBundle {
	t.Helper()

	baseDir := t.TempDir()
	directMemoryPath := filepath.Join(baseDir, "direct-memory.jsonl")
	proxiedMemoryPath := filepath.Join(baseDir, "proxied-memory.jsonl")
	if err := os.WriteFile(directMemoryPath, nil, 0o644); err != nil {
		t.Fatalf("failed to create direct memory file: %v", err)
	}
	if err := os.WriteFile(proxiedMemoryPath, nil, 0o644); err != nil {
		t.Fatalf("failed to create proxied memory file: %v", err)
	}

	return &fixtureBundle{
		Direct: modeFixture{
			Env: map[string]string{
				"MEMORY_FILE_PATH": directMemoryPath,
			},
		},
		Proxied: modeFixture{
			Env: map[string]string{
				"MEMORY_FILE_PATH": proxiedMemoryPath,
			},
		},
	}
}

func normalizeMemoryResult(_ serverMode, value any, _ *fixtureBundle) any {
	return normalizeMemoryGraphValue(value)
}

func normalizeMemoryGraphValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if entities, ok := typed["entities"].([]any); ok {
			typed["entities"] = normalizeEntities(entities)
		}
		if relations, ok := typed["relations"].([]any); ok {
			typed["relations"] = normalizeRelations(relations)
		}

		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)

		normalized := make(map[string]any, len(typed))
		for _, key := range keys {
			normalized[key] = normalizeMemoryGraphValue(typed[key])
		}
		return normalized
	case []any:
		items := make([]any, len(typed))
		for i := range typed {
			items[i] = normalizeMemoryGraphValue(typed[i])
		}
		return items
	default:
		return typed
	}
}

func normalizeEntities(values []any) []any {
	normalized := make([]any, 0, len(values))
	for _, value := range values {
		entity, ok := value.(map[string]any)
		if !ok {
			normalized = append(normalized, normalizeMemoryGraphValue(value))
			continue
		}
		if observations, ok := entity["observations"].([]any); ok {
			sort.Slice(observations, func(i, j int) bool {
				return stringifyJSONValue(observations[i]) < stringifyJSONValue(observations[j])
			})
			entity["observations"] = observations
		}
		normalized = append(normalized, normalizeMemoryGraphValue(entity))
	}
	sort.Slice(normalized, func(i, j int) bool {
		left := normalized[i].(map[string]any)["name"]
		right := normalized[j].(map[string]any)["name"]
		return stringifyJSONValue(left) < stringifyJSONValue(right)
	})
	return normalized
}

func normalizeRelations(values []any) []any {
	normalized := make([]any, 0, len(values))
	for _, value := range values {
		normalized = append(normalized, normalizeMemoryGraphValue(value))
	}
	sort.Slice(normalized, func(i, j int) bool {
		left := normalized[i].(map[string]any)
		right := normalized[j].(map[string]any)
		leftKey := stringifyJSONValue(left["from"]) + "|" + stringifyJSONValue(left["relationType"]) + "|" + stringifyJSONValue(left["to"])
		rightKey := stringifyJSONValue(right["from"]) + "|" + stringifyJSONValue(right["relationType"]) + "|" + stringifyJSONValue(right["to"])
		return leftKey < rightKey
	})
	return normalized
}

func stringifyJSONValue(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
