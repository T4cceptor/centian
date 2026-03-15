package everything

import (
	"context"
	"fmt"
	"slices"
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
	runCapabilityComparison(t, "resources_list", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateResourcesListProbe(ctx, t, pair)
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingResourceReadPhase3(t *testing.T) {
	runCapabilityComparison(t, "resources_read", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateResourceReadProbe(ctx, t, pair)
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingResourceSubscriptionsPhase3(t *testing.T) {
	runCapabilityComparison(t, "resources_subscribe", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateResourceSubscriptionProbe(ctx, t, pair)
		reportPhase3Outcome(t, &outcome)
	})
}

func TestEverythingResourceTemplatesPhase3(t *testing.T) {
	runCapabilityComparison(t, "resources_templates_list", func(ctx context.Context, t *testing.T, pair *connectionPair) {
		outcome := evaluateResourceTemplatesProbe(ctx, t, pair)
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

func evaluateResourcesListProbe(ctx context.Context, t *testing.T, pair *connectionPair) phase3Outcome {
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

func evaluateResourceReadProbe(ctx context.Context, t *testing.T, pair *connectionPair) phase3Outcome {
	t.Helper()

	resourceURI, directResources, proxiedResources := selectProbeResourceURI(ctx, t, pair)
	if resourceURI == "" {
		return phase3Outcome{
			Name:           "resources/read",
			Classification: classificationMatch,
			Summary:        "resource read probe skipped because neither side exposes resources",
			DirectDetails:  renderOperationOutcome(t, directResources, nil),
			ProxiedDetails: renderOperationOutcome(t, proxiedResources, nil),
		}
	}

	directResult, directErr := pair.Direct.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: resourceURI})
	proxiedResult, proxiedErr := pair.Proxied.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: resourceURI})

	if sameErrorMessage(directErr, proxiedErr) && jsonEqual(t, directResult, proxiedResult) {
		return phase3Outcome{
			Name:           "resources/read",
			Classification: classificationMatch,
			Summary:        "direct and proxied resources/read behaved the same",
			DirectDetails:  renderOperationOutcome(t, directResult, directErr),
			ProxiedDetails: renderOperationOutcome(t, proxiedResult, proxiedErr),
		}
	}

	if directErr == nil && proxiedErr != nil {
		return phase3Outcome{
			Name:           "resources/read",
			Classification: classificationUnsupportedInCentian,
			Summary:        "direct path supports resources/read but proxied path does not",
			DirectDetails:  renderOperationOutcome(t, directResult, directErr),
			ProxiedDetails: renderOperationOutcome(t, proxiedResult, proxiedErr),
		}
	}

	return phase3Outcome{
		Name:           "resources/read",
		Classification: classificationProxyDivergence,
		Summary:        "resources/read produced different direct/proxied outcomes",
		DirectDetails:  renderOperationOutcome(t, directResult, directErr),
		ProxiedDetails: renderOperationOutcome(t, proxiedResult, proxiedErr),
	}
}

func evaluateResourceSubscriptionProbe(ctx context.Context, t *testing.T, pair *connectionPair) phase3Outcome {
	t.Helper()

	resourceURI, _, _ := selectProbeResourceURI(ctx, t, pair)
	if resourceURI == "" {
		return phase3Outcome{
			Name:           "resources/subscribe",
			Classification: classificationMatch,
			Summary:        "resource subscription probe skipped because neither side exposes resources",
			DirectDetails:  "no resources discovered",
			ProxiedDetails: "no resources discovered",
		}
	}

	directSubErr := pair.Direct.session.Subscribe(ctx, &mcp.SubscribeParams{URI: resourceURI})
	proxiedSubErr := pair.Proxied.session.Subscribe(ctx, &mcp.SubscribeParams{URI: resourceURI})
	if sameErrorMessage(directSubErr, proxiedSubErr) && directSubErr != nil {
		return phase3Outcome{
			Name:           "resources/subscribe",
			Classification: classificationMatch,
			Summary:        "resources/subscribe is unsupported on both paths",
			DirectDetails:  directSubErr.Error(),
			ProxiedDetails: proxiedSubErr.Error(),
		}
	}
	if directSubErr == nil && proxiedSubErr != nil {
		return phase3Outcome{
			Name:           "resources/subscribe",
			Classification: classificationUnsupportedInCentian,
			Summary:        "direct path supports resources/subscribe but proxied path does not",
			DirectDetails:  "subscribe succeeded on direct path",
			ProxiedDetails: proxiedSubErr.Error(),
		}
	}
	if directSubErr != nil || proxiedSubErr != nil {
		return phase3Outcome{
			Name:           "resources/subscribe",
			Classification: classificationProxyDivergence,
			Summary:        "resources/subscribe produced different direct/proxied outcomes",
			DirectDetails:  renderError(directSubErr),
			ProxiedDetails: renderError(proxiedSubErr),
		}
	}

	directBefore := pair.Direct.recorder.snapshot()
	proxiedBefore := pair.Proxied.recorder.snapshot()

	directToggle := mustCallTool(t, ctx, pair.Direct.session, "toggle-subscriber-updates", map[string]any{})
	proxiedToggle := mustCallTool(t, ctx, pair.Proxied.session, "toggle-subscriber-updates", map[string]any{})

	directAfter := waitForRecorderState(t, pair.Direct.recorder, 3*time.Second, func(snapshot notificationSnapshot) bool {
		return snapshot.ResourceUpdateCount > directBefore.ResourceUpdateCount
	})
	proxiedAfter := waitForRecorderState(t, pair.Proxied.recorder, 3*time.Second, func(snapshot notificationSnapshot) bool {
		return snapshot.ResourceUpdateCount > proxiedBefore.ResourceUpdateCount
	})

	if directAfter.ResourceUpdateCount == directBefore.ResourceUpdateCount {
		return phase3Outcome{
			Name:           "notifications/resources/updated",
			Classification: classificationProxyDivergence,
			Summary:        "direct path did not emit resource update notifications after enabling subscriber updates",
			DirectDetails:  prettyJSON(t, directAfter),
			ProxiedDetails: prettyJSON(t, proxiedAfter),
		}
	}
	if proxiedAfter.ResourceUpdateCount == proxiedBefore.ResourceUpdateCount {
		return phase3Outcome{
			Name:           "notifications/resources/updated",
			Classification: classificationProxyDivergence,
			Summary:        "direct path emitted resource update notifications but proxied path did not",
			DirectDetails:  prettyJSON(t, directAfter),
			ProxiedDetails: prettyJSON(t, proxiedAfter),
		}
	}

	directNewURIs := directAfter.ResourceUpdateURIs[directBefore.ResourceUpdateCount:]
	proxiedNewURIs := proxiedAfter.ResourceUpdateURIs[proxiedBefore.ResourceUpdateCount:]
	if len(directNewURIs) == 0 || len(proxiedNewURIs) == 0 || directNewURIs[0] != proxiedNewURIs[0] {
		return phase3Outcome{
			Name:           "notifications/resources/updated",
			Classification: classificationProxyDivergence,
			Summary:        "resource update notifications carried different URIs on direct and proxied paths",
			DirectDetails:  prettyJSON(t, directAfter),
			ProxiedDetails: prettyJSON(t, proxiedAfter),
		}
	}

	if directAfter.ResourceListChanged > directBefore.ResourceListChanged && proxiedAfter.ResourceListChanged == proxiedBefore.ResourceListChanged {
		return phase3Outcome{
			Name:           "notifications/resources/list_changed",
			Classification: classificationProxyDivergence,
			Summary:        "direct path emitted resources/list_changed but proxied path did not",
			DirectDetails:  prettyJSON(t, directAfter),
			ProxiedDetails: prettyJSON(t, proxiedAfter),
		}
	}

	directUnsubErr := pair.Direct.session.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: resourceURI})
	proxiedUnsubErr := pair.Proxied.session.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: resourceURI})
	if directUnsubErr == nil && proxiedUnsubErr != nil {
		return phase3Outcome{
			Name:           "resources/unsubscribe",
			Classification: classificationUnsupportedInCentian,
			Summary:        "direct path supports resources/unsubscribe but proxied path does not",
			DirectDetails:  "unsubscribe succeeded on direct path",
			ProxiedDetails: proxiedUnsubErr.Error(),
		}
	}
	if !sameErrorMessage(directUnsubErr, proxiedUnsubErr) {
		return phase3Outcome{
			Name:           "resources/unsubscribe",
			Classification: classificationProxyDivergence,
			Summary:        "resources/unsubscribe produced different direct/proxied outcomes",
			DirectDetails:  renderError(directUnsubErr),
			ProxiedDetails: renderError(proxiedUnsubErr),
		}
	}

	time.Sleep(1200 * time.Millisecond)
	directFinal := pair.Direct.recorder.snapshot()
	proxiedFinal := pair.Proxied.recorder.snapshot()
	directPostUnsubDelta := directFinal.ResourceUpdateCount - directAfter.ResourceUpdateCount
	proxiedPostUnsubDelta := proxiedFinal.ResourceUpdateCount - proxiedAfter.ResourceUpdateCount
	if directPostUnsubDelta != proxiedPostUnsubDelta {
		return phase3Outcome{
			Name:           "resources/unsubscribe",
			Classification: classificationProxyDivergence,
			Summary:        "direct and proxied paths diverged after unsubscribe while subscriber updates were still running",
			DirectDetails:  prettyJSON(t, directFinal),
			ProxiedDetails: prettyJSON(t, proxiedFinal),
		}
	}

	return phase3Outcome{
		Name:           "resources/subscribe",
		Classification: classificationMatch,
		Summary:        fmt.Sprintf("resource subscription parity held (updates direct=%d proxied=%d, post-unsubscribe delta=%d)", directAfter.ResourceUpdateCount-directBefore.ResourceUpdateCount, proxiedAfter.ResourceUpdateCount-proxiedBefore.ResourceUpdateCount, directPostUnsubDelta),
		DirectDetails:  fmt.Sprintf("toggle result:\n%s\nnotifications:\n%s", prettyJSON(t, directToggle), prettyJSON(t, directFinal)),
		ProxiedDetails: fmt.Sprintf("toggle result:\n%s\nnotifications:\n%s", prettyJSON(t, proxiedToggle), prettyJSON(t, proxiedFinal)),
	}
}

func evaluateResourceTemplatesProbe(ctx context.Context, t *testing.T, pair *connectionPair) phase3Outcome {
	t.Helper()

	directTemplates, directErr := pair.Direct.session.ListResourceTemplates(ctx, nil)
	proxiedTemplates, proxiedErr := pair.Proxied.session.ListResourceTemplates(ctx, nil)

	normalizedDirectTemplates := normalizeTemplateListResult(directTemplates)
	normalizedProxiedTemplates := normalizeTemplateListResult(proxiedTemplates)
	if sameErrorMessage(directErr, proxiedErr) && jsonEqual(t, normalizedDirectTemplates, normalizedProxiedTemplates) {
		return phase3Outcome{
			Name:           "resources/templates/list",
			Classification: classificationMatch,
			Summary:        "direct and proxied resources/templates/list behaved the same",
			DirectDetails:  renderOperationOutcome(t, normalizedDirectTemplates, directErr),
			ProxiedDetails: renderOperationOutcome(t, normalizedProxiedTemplates, proxiedErr),
		}
	}

	if directErr == nil && proxiedErr != nil {
		return phase3Outcome{
			Name:           "resources/templates/list",
			Classification: classificationUnsupportedInCentian,
			Summary:        "direct path supports resources/templates/list but proxied path does not",
			DirectDetails:  renderOperationOutcome(t, directTemplates, directErr),
			ProxiedDetails: renderOperationOutcome(t, proxiedTemplates, proxiedErr),
		}
	}

	return phase3Outcome{
		Name:           "resources/templates/list",
		Classification: classificationProxyDivergence,
		Summary:        "resources/templates/list produced different direct/proxied outcomes",
		DirectDetails:  renderOperationOutcome(t, normalizedDirectTemplates, directErr),
		ProxiedDetails: renderOperationOutcome(t, normalizedProxiedTemplates, proxiedErr),
	}
}

func selectProbeResourceURI(ctx context.Context, t *testing.T, pair *connectionPair) (string, *mcp.ListResourcesResult, *mcp.ListResourcesResult) {
	t.Helper()

	directResources, directErr := pair.Direct.session.ListResources(ctx, nil)
	if directErr != nil {
		t.Fatalf("failed to list direct resources: %v", directErr)
	}
	proxiedResources, proxiedErr := pair.Proxied.session.ListResources(ctx, nil)
	if proxiedErr != nil {
		t.Fatalf("failed to list proxied resources: %v", proxiedErr)
	}
	if len(directResources.Resources) == 0 {
		return "", directResources, proxiedResources
	}
	return directResources.Resources[0].URI, directResources, proxiedResources
}

func waitForRecorderState(
	t *testing.T,
	recorder *notificationRecorder,
	timeout time.Duration,
	ready func(notificationSnapshot) bool,
) notificationSnapshot {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := recorder.snapshot()
		if ready(snapshot) {
			return snapshot
		}
		time.Sleep(25 * time.Millisecond)
	}
	return recorder.snapshot()
}

func normalizeTemplateListResult(result *mcp.ListResourceTemplatesResult) *mcp.ListResourceTemplatesResult {
	if result == nil {
		return nil
	}

	cloned := &mcp.ListResourceTemplatesResult{
		Meta:              result.Meta,
		NextCursor:        result.NextCursor,
		ResourceTemplates: append([]*mcp.ResourceTemplate(nil), result.ResourceTemplates...),
	}
	slices.SortFunc(cloned.ResourceTemplates, func(left, right *mcp.ResourceTemplate) int {
		switch {
		case left == nil && right == nil:
			return 0
		case left == nil:
			return -1
		case right == nil:
			return 1
		case left.URITemplate < right.URITemplate:
			return -1
		case left.URITemplate > right.URITemplate:
			return 1
		default:
			return 0
		}
	})
	return cloned
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

func renderError(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
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
