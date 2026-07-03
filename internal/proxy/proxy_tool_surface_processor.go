package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolSurfaceActionModify = "modify"
	toolSurfaceActionHide   = "hide"
	toolSurfaceActionFail   = "fail"
)

// ToolSurfaceProcessingController runs registration-time processors against tool catalogs.
type ToolSurfaceProcessingController struct {
	processors []processor.ProcessorInterface
}

type toolSurfaceState struct {
	gateway    string
	serverName string
	tools      []*processedToolEntry
}

type toolSurfaceProcessResult struct {
	tools       []*processedToolEntry
	annotations []common.EventAnnotation
}

type processedToolEntry struct {
	serverName            string
	originalName          string
	defaultExposedName    string
	definitionFingerprint string
	tool                  *mcp.Tool
}

// NewToolSurfaceProcessingController returns a controller for processors with the tool_surface part.
func NewToolSurfaceProcessingController(processorConfigs []*config.ProcessorConfig) (*ToolSurfaceProcessingController, error) {
	result := &ToolSurfaceProcessingController{
		processors: make([]processor.ProcessorInterface, 0),
	}
	for _, config := range processorConfigs {
		if !config.IsToolSurfaceProcessor() {
			continue
		}
		p, err := processor.NewProcessor(config)
		if err == nil {
			result.processors = append(result.processors, p)
			continue
		}
		if errors.Is(err, processor.ErrProcessorDisabled) {
			if config.Required {
				return nil, fmt.Errorf("unable to configure required processor '%s', Error: %w", config.Name, err)
			}
			common.LogInfo("processor '%s' is disabled, skipping", config.Name)
			continue
		}
		if config.Required {
			return nil, fmt.Errorf("unable to configure required processor '%s', Error: %w", config.Name, err)
		}
		common.LogWarn("unable to configure processor '%s': %s", config.Name, err.Error())
	}
	return result, nil
}

func (c *ToolSurfaceProcessingController) hasProcessors() bool {
	return c != nil && len(c.processors) > 0
}

func (c *ToolSurfaceProcessingController) Process(state *toolSurfaceState) (*toolSurfaceProcessResult, error) {
	result := &toolSurfaceProcessResult{
		tools: cloneProcessedToolEntries(state.tools),
	}
	if !c.hasProcessors() {
		return result, nil
	}

	current := &toolSurfaceState{
		gateway:    state.gateway,
		serverName: state.serverName,
		tools:      result.tools,
	}

	for _, p := range c.processors {
		cfg := p.GetConfig()
		input := buildToolSurfaceInput(cfg, current, result.annotations)
		output, err := p.Process(input)
		if err != nil {
			if cfg.Required {
				return nil, err
			}
			common.LogWarn("tool surface processor '%s' failed during execution: %v", cfg.Name, err)
			continue
		}

		nextTools := cloneProcessedToolEntries(current.tools)
		if err := applyToolSurfaceOutput(nextTools, output); err != nil {
			if cfg.Required {
				return nil, err
			}
			common.LogWarn("tool surface processor '%s' output ignored: %v", cfg.Name, err)
			continue
		}
		if err := validateProcessedToolEntries(nextTools); err != nil {
			if cfg.Required {
				return nil, err
			}
			common.LogWarn("tool surface processor '%s' output ignored: %v", cfg.Name, err)
			continue
		}

		current.tools = nextTools
		result.tools = nextTools
		if output != nil && output.Annotations != nil {
			result.annotations = append(result.annotations, output.Annotations.Reports...)
		}
	}

	return result, nil
}

func (p *CentianEndpoint) processToolSurface(
	serverName string,
	tools []*mcp.Tool,
) ([]*processedToolEntry, []common.EventAnnotation, error) {
	state := &toolSurfaceState{
		gateway:    getGatewayFromPath(p.endpoint),
		serverName: serverName,
		tools:      p.defaultToolSurfaceEntries(serverName, tools),
	}
	if p.surfaceProcessor == nil || !p.surfaceProcessor.hasProcessors() {
		return compactProcessedToolEntries(state.tools), nil, nil
	}
	result, err := p.surfaceProcessor.Process(state)
	if err != nil {
		return nil, nil, err
	}
	return compactProcessedToolEntries(result.tools), result.annotations, nil
}

func (p *CentianEndpoint) defaultToolSurfaceEntries(serverName string, tools []*mcp.Tool) []*processedToolEntry {
	entries := make([]*processedToolEntry, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || isProxyToolName(tool.Name) {
			continue
		}
		clonedTool := copyToolForRegistration(tool)
		defaultExposedName := tool.Name
		if p.isAggregatedProxy {
			defaultExposedName = fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
			clonedTool.Description = fmt.Sprintf("[%s] %s", serverName, tool.Description)
		}
		clonedTool.Name = defaultExposedName
		entries = append(entries, &processedToolEntry{
			serverName:            serverName,
			originalName:          tool.Name,
			defaultExposedName:    defaultExposedName,
			definitionFingerprint: fingerprintToolDefinition(tool),
			tool:                  clonedTool,
		})
	}
	return entries
}

func buildToolSurfaceInput(
	cfg *config.ProcessorConfig,
	state *toolSurfaceState,
	annotations []common.EventAnnotation,
) *processor.DataContext {
	input := &processor.DataContext{Version: processor.CurrentDataContextVersion}
	if cfg.HasPart("tool_surface") {
		input.ToolSurface = &processor.ToolSurfacePart{
			Gateway:    state.gateway,
			ServerName: state.serverName,
			Tools:      toolSurfaceToolsForInput(state.tools),
		}
	}
	if cfg.HasPart("annotations") {
		input.Annotations = &processor.AnnotationPart{Reports: annotations}
	}
	return input
}

func toolSurfaceToolsForInput(entries []*processedToolEntry) []processor.ToolSurfaceTool {
	tools := make([]processor.ToolSurfaceTool, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.tool == nil {
			continue
		}
		tools = append(tools, processor.ToolSurfaceTool{
			OriginalName:       entry.originalName,
			DefaultExposedName: entry.defaultExposedName,
			ExposedName:        entry.tool.Name,
			Fingerprint:        entry.definitionFingerprint,
			Tool:               copyToolForRegistration(entry.tool),
		})
	}
	return tools
}

func applyToolSurfaceOutput(entries []*processedToolEntry, output *processor.DataContext) error {
	if output == nil || output.ToolSurface == nil {
		return nil
	}
	for _, decision := range output.ToolSurface.Decisions {
		if err := applyToolSurfaceDecision(entries, decision); err != nil {
			return err
		}
	}
	return nil
}

func applyToolSurfaceDecision(entries []*processedToolEntry, decision processor.ToolSurfaceDecision) error {
	if decision.Action == "" {
		decision.Action = toolSurfaceActionModify
	}
	if decision.Action == toolSurfaceActionFail {
		if decision.Message != "" {
			return fmt.Errorf("%s", decision.Message)
		}
		return fmt.Errorf("tool surface processor requested failure for %q", decision.ToolName)
	}

	idx := findProcessedToolEntry(entries, decision)
	if idx < 0 {
		return fmt.Errorf("tool surface decision references unknown tool %q", decision.ToolName)
	}

	switch decision.Action {
	case toolSurfaceActionHide:
		entries[idx] = nil
	case toolSurfaceActionModify:
		tool := entries[idx].tool
		if decision.ExposedName != "" {
			tool.Name = decision.ExposedName
		}
		if decision.Description != nil {
			tool.Description = *decision.Description
		}
		if decision.Annotations != nil {
			tool.Annotations = decision.Annotations
		}
		if decision.Meta != nil {
			tool.Meta = decision.Meta
		}
	default:
		return fmt.Errorf("unsupported tool surface action %q", decision.Action)
	}
	return nil
}

func findProcessedToolEntry(entries []*processedToolEntry, decision processor.ToolSurfaceDecision) int {
	if decision.ToolName == "" {
		return -1
	}
	for idx, entry := range entries {
		if entry == nil || entry.tool == nil {
			continue
		}
		if decision.ToolName == entry.originalName {
			return idx
		}
		if decision.ToolName == entry.tool.Name {
			return idx
		}
	}
	return -1
}

func validateProcessedToolEntries(entries []*processedToolEntry) error {
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry == nil || entry.tool == nil {
			continue
		}
		if entry.tool.Name == "" {
			return fmt.Errorf("tool surface produced an empty exposed tool name")
		}
		if isProxyToolName(entry.tool.Name) {
			return fmt.Errorf("tool surface produced reserved tool name %q", entry.tool.Name)
		}
		if _, exists := seen[entry.tool.Name]; exists {
			return fmt.Errorf("tool surface produced duplicate exposed tool name %q", entry.tool.Name)
		}
		seen[entry.tool.Name] = struct{}{}
	}
	return nil
}

func cloneProcessedToolEntries(entries []*processedToolEntry) []*processedToolEntry {
	cloned := make([]*processedToolEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			cloned = append(cloned, nil)
			continue
		}
		cloned = append(cloned, &processedToolEntry{
			serverName:            entry.serverName,
			originalName:          entry.originalName,
			defaultExposedName:    entry.defaultExposedName,
			definitionFingerprint: entry.definitionFingerprint,
			tool:                  copyToolForRegistration(entry.tool),
		})
	}
	return cloned
}

func compactProcessedToolEntries(entries []*processedToolEntry) []*processedToolEntry {
	compacted := make([]*processedToolEntry, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && entry.tool != nil {
			compacted = append(compacted, entry)
		}
	}
	return compacted
}

func fingerprintToolDefinition(tool *mcp.Tool) string {
	if tool == nil {
		return ""
	}
	wire := map[string]any{
		"name":         tool.Name,
		"description":  tool.Description,
		"inputSchema":  tool.InputSchema,
		"annotations":  tool.Annotations,
		"_meta":        tool.Meta,
		"outputSchema": tool.OutputSchema,
		"title":        tool.Title,
		"icons":        tool.Icons,
	}
	encoded, err := marshalCanonicalJSON(wire)
	if err != nil {
		return "invalid"
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func fingerprintRegisteredTool(tool *mcp.Tool) string {
	encoded, err := marshalCanonicalJSON(tool)
	if err != nil {
		return "invalid"
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func marshalCanonicalJSON(value any) ([]byte, error) {
	normalized, err := normalizeJSONValue(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func normalizeJSONValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	return sortJSONValue(decoded), nil
}

func sortJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		ordered := make([][2]any, 0, len(keys))
		for _, key := range keys {
			ordered = append(ordered, [2]any{key, sortJSONValue(typed[key])})
		}
		result := make(map[string]any, len(ordered))
		for _, pair := range ordered {
			result[pair[0].(string)] = pair[1]
		}
		return result
	case []any:
		for idx := range typed {
			typed[idx] = sortJSONValue(typed[idx])
		}
		return typed
	default:
		return value
	}
}
