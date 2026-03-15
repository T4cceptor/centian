package everything

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEverythingProgressNotificationsPhase3(t *testing.T) {
	runCapabilityComparison(t, "progress_notifications", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateProgressProbe(ctx, t, pair)
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingLoggingNotificationsPhase3(t *testing.T) {
	runCapabilityComparison(t, "logging_notifications", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateLoggingProbe(ctx, t, pair)
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingSamplingPhase3(t *testing.T) {
	runCapabilityComparison(t, "sampling_request", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateSimpleToolProbe(ctx, t, pair, "trigger-sampling-request", nil)
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingElicitationPhase3(t *testing.T) {
	runCapabilityComparison(t, "elicitation_request", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateSimpleToolProbe(ctx, t, pair, "trigger-elicitation-request", nil)
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingResearchTaskProbePhase3(t *testing.T) {
	runCapabilityComparison(t, "simulate_research_query", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateSimpleToolProbe(ctx, t, pair, "simulate-research-query", map[string]any{
			"topic":     "centian phase 3 probe",
			"ambiguous": false,
		})
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingResourcesSurfacePhase3(t *testing.T) {
	runCapabilityComparison(t, "resources_surface", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateResourcesProbe(ctx, t, pair)
		reportPhase3Outcome(t, &outcome)
	})
}

func evaluateProgressProbe(ctx context.Context, t *testing.T, pair *connectionPair) phase3Outcome {
	t.Helper()

	toolName := "trigger-long-running-operation"
	directTools, err := waitForTools(ctx, pair.Direct.session, defaultToolsWaitTimeout, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to list direct tools: %v", err)
	}
	proxiedTools, err := waitForTools(ctx, pair.Proxied.session, defaultToolsWaitTimeout, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to list proxied tools: %v", err)
	}

	if !hasTool(directTools.Tools, toolName) {
		t.Fatalf("direct everything server does not expose %q", toolName)
	}
	if !hasTool(proxiedTools.Tools, toolName) {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationUnsupportedInCentian,
			Summary:        "tool required for progress probe is missing from proxied tool catalog",
			DirectDetails:  strings.Join(toolNames(directTools.Tools), ", "),
			ProxiedDetails: strings.Join(toolNames(proxiedTools.Tools), ", "),
		}
	}

	directParams := &mcp.CallToolParams{
		Name:      toolName,
		Arguments: map[string]any{"duration": 1, "steps": 3},
	}
	directParams.SetProgressToken("phase3-direct-progress")
	proxiedParams := &mcp.CallToolParams{
		Name:      toolName,
		Arguments: map[string]any{"duration": 1, "steps": 3},
	}
	proxiedParams.SetProgressToken("phase3-proxied-progress")

	directResult := mustCallToolWithParams(t, ctx, pair.Direct.session, directParams)
	proxiedResult := mustCallToolWithParams(t, ctx, pair.Proxied.session, proxiedParams)

	time.Sleep(1500 * time.Millisecond)

	directSnapshot := pair.Direct.recorder.snapshot()
	proxiedSnapshot := pair.Proxied.recorder.snapshot()

	if directSnapshot.ProgressCount == proxiedSnapshot.ProgressCount && jsonEqual(t, directResult, proxiedResult) {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationMatch,
			Summary:        fmt.Sprintf("both paths produced %d progress notifications", directSnapshot.ProgressCount),
			DirectDetails:  prettyJSON(t, directSnapshot),
			ProxiedDetails: prettyJSON(t, proxiedSnapshot),
		}
	}

	if directSnapshot.ProgressCount > 0 && proxiedSnapshot.ProgressCount == 0 {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationProxyDivergence,
			Summary:        "direct path received progress notifications but proxied path did not",
			DirectDetails:  prettyJSON(t, directSnapshot),
			ProxiedDetails: prettyJSON(t, proxiedSnapshot),
		}
	}

	return phase3Outcome{
		Name:           toolName,
		Classification: classificationProxyDivergence,
		Summary:        "progress probe produced different direct/proxied outcomes",
		DirectDetails:  fmt.Sprintf("result:\n%s\nnotifications:\n%s", prettyJSON(t, directResult), prettyJSON(t, directSnapshot)),
		ProxiedDetails: fmt.Sprintf("result:\n%s\nnotifications:\n%s", prettyJSON(t, proxiedResult), prettyJSON(t, proxiedSnapshot)),
	}
}

func evaluateLoggingProbe(ctx context.Context, t *testing.T, pair *connectionPair) phase3Outcome {
	t.Helper()

	toolName := "toggle-simulated-logging"
	directTools, err := waitForTools(ctx, pair.Direct.session, defaultToolsWaitTimeout, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to list direct tools: %v", err)
	}
	proxiedTools, err := waitForTools(ctx, pair.Proxied.session, defaultToolsWaitTimeout, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to list proxied tools: %v", err)
	}

	if !hasTool(directTools.Tools, toolName) {
		t.Fatalf("direct everything server does not expose %q", toolName)
	}
	if !hasTool(proxiedTools.Tools, toolName) {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationUnsupportedInCentian,
			Summary:        "tool required for logging probe is missing from proxied tool catalog",
			DirectDetails:  strings.Join(toolNames(directTools.Tools), ", "),
			ProxiedDetails: strings.Join(toolNames(proxiedTools.Tools), ", "),
		}
	}

	if err := pair.Direct.session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		t.Fatalf("failed to set direct logging level: %v", err)
	}
	if err := pair.Proxied.session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationUnsupportedInCentian,
			Summary:        "proxied path does not support logging/setLevel",
			DirectDetails:  "logging/setLevel succeeded on direct path",
			ProxiedDetails: err.Error(),
		}
	}

	directResult := mustCallTool(t, ctx, pair.Direct.session, toolName, map[string]any{})
	proxiedResult := mustCallTool(t, ctx, pair.Proxied.session, toolName, map[string]any{})

	time.Sleep(1500 * time.Millisecond)

	directSnapshot := pair.Direct.recorder.snapshot()
	proxiedSnapshot := pair.Proxied.recorder.snapshot()

	if directSnapshot.LogCount > 0 && proxiedSnapshot.LogCount > 0 && jsonEqual(t, directResult, proxiedResult) {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationMatch,
			Summary:        fmt.Sprintf("both paths produced log notifications (direct=%d proxied=%d)", directSnapshot.LogCount, proxiedSnapshot.LogCount),
			DirectDetails:  renderLoggingOutcome(t, directResult, directSnapshot),
			ProxiedDetails: renderLoggingOutcome(t, proxiedResult, proxiedSnapshot),
		}
	}

	if directSnapshot.LogCount > 0 && proxiedSnapshot.LogCount == 0 {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationProxyDivergence,
			Summary:        "direct path received log notifications but proxied path did not",
			DirectDetails:  renderLoggingOutcome(t, directResult, directSnapshot),
			ProxiedDetails: renderLoggingOutcome(t, proxiedResult, proxiedSnapshot),
		}
	}

	return phase3Outcome{
		Name:           toolName,
		Classification: classificationProxyDivergence,
		Summary:        "logging probe produced different direct/proxied outcomes",
		DirectDetails:  renderLoggingOutcome(t, directResult, directSnapshot),
		ProxiedDetails: renderLoggingOutcome(t, proxiedResult, proxiedSnapshot),
	}
}

func evaluateSimpleToolProbe(
	ctx context.Context,
	t *testing.T,
	pair *connectionPair,
	toolName string,
	args map[string]any,
) phase3Outcome {
	t.Helper()

	directTools, err := waitForTools(ctx, pair.Direct.session, defaultToolsWaitTimeout, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to list direct tools: %v", err)
	}
	proxiedTools, err := waitForTools(ctx, pair.Proxied.session, defaultToolsWaitTimeout, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to list proxied tools: %v", err)
	}

	directHasTool := hasTool(directTools.Tools, toolName)
	proxiedHasTool := hasTool(proxiedTools.Tools, toolName)

	if directHasTool && !proxiedHasTool {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationUnsupportedInCentian,
			Summary:        "tool is available directly but missing from proxied catalog",
			DirectDetails:  strings.Join(toolNames(directTools.Tools), ", "),
			ProxiedDetails: strings.Join(toolNames(proxiedTools.Tools), ", "),
		}
	}
	if !directHasTool && !proxiedHasTool {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationMatch,
			Summary:        "tool is absent on both sides",
			DirectDetails:  strings.Join(toolNames(directTools.Tools), ", "),
			ProxiedDetails: strings.Join(toolNames(proxiedTools.Tools), ", "),
		}
	}
	if !directHasTool {
		t.Fatalf("proxied tool %q exists but direct tool does not", toolName)
	}

	directResult, directErr := pair.Direct.session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: args})
	proxiedResult, proxiedErr := pair.Proxied.session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: args})

	if sameErrorMessage(directErr, proxiedErr) && jsonEqual(t, directResult, proxiedResult) {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationMatch,
			Summary:        "direct and proxied tool execution matched",
			DirectDetails:  renderCallOutcome(t, directResult, directErr),
			ProxiedDetails: renderCallOutcome(t, proxiedResult, proxiedErr),
		}
	}

	if directErr == nil && proxiedErr != nil {
		return phase3Outcome{
			Name:           toolName,
			Classification: classificationProxyDivergence,
			Summary:        "direct tool execution succeeded but proxied execution failed",
			DirectDetails:  renderCallOutcome(t, directResult, directErr),
			ProxiedDetails: renderCallOutcome(t, proxiedResult, proxiedErr),
		}
	}

	return phase3Outcome{
		Name:           toolName,
		Classification: classificationProxyDivergence,
		Summary:        "direct and proxied tool execution produced different outcomes",
		DirectDetails:  renderCallOutcome(t, directResult, directErr),
		ProxiedDetails: renderCallOutcome(t, proxiedResult, proxiedErr),
	}
}

func evaluateResourcesProbe(ctx context.Context, t *testing.T, pair *connectionPair) phase3Outcome {
	t.Helper()

	directResources, directErr := pair.Direct.session.ListResources(ctx, nil)
	proxiedResources, proxiedErr := pair.Proxied.session.ListResources(ctx, nil)

	if sameErrorMessage(directErr, proxiedErr) && jsonEqual(t, directResources, proxiedResources) {
		return phase3Outcome{
			Name:           "resources/list",
			Classification: classificationMatch,
			Summary:        "direct and proxied resources/list behaved the same",
			DirectDetails:  renderOperationOutcome(t, directResources, directErr),
			ProxiedDetails: renderOperationOutcome(t, proxiedResources, proxiedErr),
		}
	}

	if directErr == nil && proxiedErr != nil {
		return phase3Outcome{
			Name:           "resources/list",
			Classification: classificationUnsupportedInCentian,
			Summary:        "direct path supports resources/list but proxied path does not",
			DirectDetails:  renderOperationOutcome(t, directResources, directErr),
			ProxiedDetails: renderOperationOutcome(t, proxiedResources, proxiedErr),
		}
	}

	return phase3Outcome{
		Name:           "resources/list",
		Classification: classificationProxyDivergence,
		Summary:        "resources/list produced different direct/proxied outcomes",
		DirectDetails:  renderOperationOutcome(t, directResources, directErr),
		ProxiedDetails: renderOperationOutcome(t, proxiedResources, proxiedErr),
	}
}

func hasTool(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func renderCallOutcome(t *testing.T, result *mcp.CallToolResult, err error) string {
	t.Helper()

	if err != nil {
		return err.Error()
	}
	return prettyJSON(t, result)
}

func renderOperationOutcome(t *testing.T, result any, err error) string {
	t.Helper()

	if err != nil {
		return err.Error()
	}
	return prettyJSON(t, result)
}

func renderLoggingOutcome(t *testing.T, result *mcp.CallToolResult, snapshot notificationSnapshot) string {
	t.Helper()

	return fmt.Sprintf("result:\n%s\nLogCount: %d", prettyJSON(t, result), snapshot.LogCount)
}

func sameErrorMessage(left, right error) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left != nil && right != nil:
		return left.Error() == right.Error()
	default:
		return false
	}
}
