package everything

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestEverythingInitializeAndListTools(t *testing.T) {
	runCapabilityComparison(t, "initialize_and_list_tools", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		// Given: both sessions completed initialize.
		directInit := pair.Direct.session.InitializeResult()
		proxiedInit := pair.Proxied.session.InitializeResult()

		if directInit == nil {
			t.Fatal("expected direct initialize result")
		}
		if proxiedInit == nil {
			t.Fatal("expected proxied initialize result")
		}

		// When: listing tools from both connections.
		directTools, err := waitForTools(ctx, pair.Direct.session, defaultToolsWaitTimeout, 250*time.Millisecond+time.Millisecond)
		if err != nil {
			t.Fatalf("failed to list direct tools: %v", err)
		}
		proxiedTools, err := waitForTools(ctx, pair.Proxied.session, defaultToolsWaitTimeout+time.Second, 250*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to list proxied tools: %v", err)
		}

		// Then: the proxy should expose the same tool names as the direct server.
		if len(directTools.Tools) == 0 {
			t.Fatal("expected direct everything server to expose tools")
		}
		if len(proxiedTools.Tools) == 0 {
			t.Fatal("expected proxied everything server to expose tools")
		}

		directNames := toolNames(directTools.Tools)
		proxiedNames := toolNames(proxiedTools.Tools)
		slices.Sort(directNames)
		slices.Sort(proxiedNames)

		if !reflect.DeepEqual(directNames, proxiedNames) {
			missingInProxied := diffNames(directNames, proxiedNames)
			extraInProxied := diffNames(proxiedNames, directNames)
			t.Fatalf(
				"tool list mismatch\nmissing in proxied: %v\nextra in proxied: %v\ndirect: %v\nproxied: %v",
				missingInProxied,
				extraInProxied,
				directNames,
				proxiedNames,
			)
		}

		directToolMap := toolMap(directTools.Tools)
		proxiedToolMap := toolMap(proxiedTools.Tools)
		for _, name := range directNames {
			directTool := directToolMap[name]
			proxiedTool := proxiedToolMap[name]
			if proxiedTool == nil {
				t.Fatalf("missing proxied tool metadata for %q", name)
			}
			if directTool.Description != proxiedTool.Description {
				t.Fatalf("description mismatch for tool %q: direct=%q proxied=%q", name, directTool.Description, proxiedTool.Description)
			}
		}

		if !jsonEqual(t, directInit.Capabilities, proxiedInit.Capabilities) {
			directCapsJSON, _ := json.MarshalIndent(directInit.Capabilities, "", "  ")
			proxiedCapsJSON, _ := json.MarshalIndent(proxiedInit.Capabilities, "", "  ")
			t.Logf("initialize capability divergence detected")
			t.Logf("direct capabilities:\n%s", directCapsJSON)
			t.Logf("proxied capabilities:\n%s", proxiedCapsJSON)
		}
	})
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()

	aJSON, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("failed to marshal first value: %v", err)
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("failed to marshal second value: %v", err)
	}

	return bytes.Equal(aJSON, bJSON)
}

func diffNames(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, name := range right {
		rightSet[name] = struct{}{}
	}

	diff := make([]string, 0)
	for _, name := range left {
		if _, ok := rightSet[name]; !ok {
			diff = append(diff, name)
		}
	}
	return diff
}
