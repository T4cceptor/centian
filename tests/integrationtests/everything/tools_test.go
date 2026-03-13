package everything

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestEverythingToolCatalogPhase2(t *testing.T) {
	runCapabilityComparison(t, "tool_catalog_and_metadata", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		// Given: tool catalogs from the direct and proxied connections.
		directTools, err := waitForTools(ctx, pair.Direct.session, defaultToolsWaitTimeout, 250*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to list direct tools: %v", err)
		}
		proxiedTools, err := waitForTools(ctx, pair.Proxied.session, defaultToolsWaitTimeout, 250*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to list proxied tools: %v", err)
		}

		directNames := toolNames(directTools.Tools)
		proxiedNames := toolNames(proxiedTools.Tools)
		slices.Sort(directNames)
		slices.Sort(proxiedNames)

		// When: comparing the available tool names.
		missingInProxied := diffNames(directNames, proxiedNames)
		extraInProxied := diffNames(proxiedNames, directNames)

		// Then: both catalogs should expose the same tool set.
		if len(missingInProxied) > 0 || len(extraInProxied) > 0 {
			t.Fatalf(
				"tool catalog mismatch\nmissing in proxied: %v\nextra in proxied: %v\ndirect: %v\nproxied: %v",
				missingInProxied,
				extraInProxied,
				directNames,
				proxiedNames,
			)
		}

		// Then: metadata for each shared tool should match exactly.
		directToolMap := toolMap(directTools.Tools)
		proxiedToolMap := toolMap(proxiedTools.Tools)
		for _, name := range directNames {
			directTool := directToolMap[name]
			proxiedTool := proxiedToolMap[name]
			if proxiedTool == nil {
				t.Fatalf("missing proxied tool metadata for %q", name)
			}
			if !jsonEqual(t, directTool, proxiedTool) {
				t.Fatalf(
					"tool metadata mismatch for %q\ndirect:\n%s\nproxied:\n%s",
					name,
					prettyJSON(t, directTool),
					prettyJSON(t, proxiedTool),
				)
			}
		}
	})
}

func TestEverythingDeterministicToolCallsPhase2(t *testing.T) {
	testCases := []struct {
		name string
		args map[string]any
	}{
		{
			name: "echo",
			args: map[string]any{"message": "hello from centian"},
		},
		{
			name: "get-sum",
			args: map[string]any{"a": 7, "b": 35},
		},
		{
			name: "get-structured-content",
			args: map[string]any{"location": "berlin"},
		},
		{
			name: "get-annotated-message",
			args: map[string]any{"messageType": "success", "includeImage": false},
		},
		{
			name: "get-resource-links",
			args: map[string]any{"count": 3},
		},
		{
			name: "get-resource-reference",
			args: map[string]any{"resourceId": 1, "resourceType": "text"},
		},
		{
			name: "get-tiny-image",
			args: map[string]any{},
		},
		{
			name: "gzip-file-as-resource",
			args: map[string]any{
				"data":       "data:text/plain;base64,SGVsbG8gQ2VudGlhbg==",
				"name":       "hello.txt",
				"outputType": "resource",
			},
		},
		{
			name: "get-roots-list",
			args: map[string]any{},
		},
	}

	for _, tc := range testCases {
		runCapabilityComparison(t, "tool_call_"+tc.name, func(ctx context.Context, t *testing.T, pair *connectionPair) {
			// Given: the tool exists on both sides before comparing call results.
			directTools, err := waitForTools(ctx, pair.Direct.session, defaultToolsWaitTimeout, 250*time.Millisecond)
			if err != nil {
				t.Fatalf("failed to list direct tools: %v", err)
			}
			proxiedTools, err := waitForTools(ctx, pair.Proxied.session, defaultToolsWaitTimeout, 250*time.Millisecond)
			if err != nil {
				t.Fatalf("failed to list proxied tools: %v", err)
			}

			mustFindTool(t, directTools.Tools, tc.name)
			mustFindTool(t, proxiedTools.Tools, tc.name)

			// When: calling the same deterministic tool through both paths.
			directResult := mustCallTool(t, ctx, pair.Direct.session, tc.name, tc.args)
			proxiedResult := mustCallTool(t, ctx, pair.Proxied.session, tc.name, tc.args)

			// Then: the proxied result should match the direct server result.
			if !jsonEqual(t, directResult, proxiedResult) {
				t.Fatalf(
					"tool result mismatch for %q\ndirect:\n%s\nproxied:\n%s",
					tc.name,
					prettyJSON(t, directResult),
					prettyJSON(t, proxiedResult),
				)
			}
		})
	}
}

func TestEverythingAdvertisedCapabilityDivergencePhase2(t *testing.T) {
	runCapabilityComparison(t, "initialize_capabilities", func(_ context.Context, t *testing.T, pair *connectionPair) {
		// Given: initialize results from both connections.
		directInit := pair.Direct.session.InitializeResult()
		proxiedInit := pair.Proxied.session.InitializeResult()

		if directInit == nil {
			t.Fatal("expected direct initialize result")
		}
		if proxiedInit == nil {
			t.Fatal("expected proxied initialize result")
		}

		// When: comparing the advertised capabilities.
		directCaps := directInit.Capabilities
		proxiedCaps := proxiedInit.Capabilities

		// Then: any divergence is reported with the full capability payloads.
		if !reflect.DeepEqual(directCaps, proxiedCaps) {
			t.Fatalf(
				"initialize capability mismatch\ndirect:\n%s\nproxied:\n%s",
				prettyJSON(t, directCaps),
				prettyJSON(t, proxiedCaps),
			)
		}
	})
}

func TestEverythingExpectedToolSetPhase2(t *testing.T) {
	expectedTools := []string{
		"echo",
		"get-annotated-message",
		"get-env",
		"get-resource-links",
		"get-resource-reference",
		"get-roots-list",
		"get-structured-content",
		"get-sum",
		"get-tiny-image",
		"gzip-file-as-resource",
		"simulate-research-query",
		"toggle-simulated-logging",
		"toggle-subscriber-updates",
		"trigger-long-running-operation",
	}

	runCapabilityComparison(t, "expected_tool_set", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		// Given: direct and proxied tool catalogs.
		directTools, err := waitForTools(ctx, pair.Direct.session, defaultToolsWaitTimeout, 250*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to list direct tools: %v", err)
		}
		proxiedTools, err := waitForTools(ctx, pair.Proxied.session, defaultToolsWaitTimeout, 250*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to list proxied tools: %v", err)
		}

		directNames := toolNames(directTools.Tools)
		proxiedNames := toolNames(proxiedTools.Tools)
		slices.Sort(directNames)
		slices.Sort(proxiedNames)

		// When: comparing both catalogs against the Phase 2 expected set.
		sortedExpected := append([]string(nil), expectedTools...)
		slices.Sort(sortedExpected)

		missingExpectedInDirect := diffNames(sortedExpected, directNames)
		missingExpectedInProxied := diffNames(sortedExpected, proxiedNames)

		// Then: both sides should expose the planned deterministic Phase 2 set.
		if len(missingExpectedInDirect) > 0 || len(missingExpectedInProxied) > 0 {
			t.Fatalf(
				"expected Phase 2 tools missing\nmissing in direct: %v\nmissing in proxied: %v\nexpected: %v\ndirect: %v\nproxied: %v",
				missingExpectedInDirect,
				missingExpectedInProxied,
				sortedExpected,
				directNames,
				proxiedNames,
			)
		}
	})
}
