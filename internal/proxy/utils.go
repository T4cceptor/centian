package proxy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/google/uuid"
)

// NamespaceSeparator is used to create tool names in an aggregated proxy server.
const NamespaceSeparator = "___"

var (
	// ErrMalformedAggregatedToolName indicates that an aggregated tool name does
	// not match the expected `<server>___<tool>` shape.
	ErrMalformedAggregatedToolName = errors.New("malformed aggregated tool name")
	// ErrUnexpectedToolNamespace indicates that an aggregated tool name is
	// namespaced for a different downstream server than expected.
	ErrUnexpectedToolNamespace = errors.New("unexpected aggregated tool namespace")
)

// AggregatedToolName captures the parsed state of a tool name in aggregated mode.
type AggregatedToolName struct {
	ToolName        string
	IsNamespaced    bool
	NamespaceServer string
}

var newUUIDV7 = uuid.NewV7

func getNewUUIDV7() string {
	result := ""
	if id, err := newUUIDV7(); err == nil {
		result = id.String()
	}
	if result == "" {
		result = fmt.Sprintf("req_%d", time.Now().UnixMicro())
	}
	return result
}

// ParseAggregatedToolName parses an aggregated tool name and optionally validates
// that the namespace matches the expected server.
func ParseAggregatedToolName(rawName, expectedServer string) (AggregatedToolName, error) {
	if rawName == "" {
		return AggregatedToolName{}, ErrMalformedAggregatedToolName
	}

	if !strings.Contains(rawName, NamespaceSeparator) {
		return AggregatedToolName{ToolName: rawName}, nil
	}

	parts := strings.Split(rawName, NamespaceSeparator)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return AggregatedToolName{ToolName: rawName}, ErrMalformedAggregatedToolName
	}

	result := AggregatedToolName{
		ToolName:        parts[1],
		IsNamespaced:    true,
		NamespaceServer: parts[0],
	}
	if expectedServer != "" && result.NamespaceServer != expectedServer {
		return result, fmt.Errorf("%w: got %q, expected %q", ErrUnexpectedToolNamespace, result.NamespaceServer, expectedServer)
	}
	return result, nil
}

func formatAggregatedToolName(serverName, toolName string) (string, error) {
	if serverName == "" || toolName == "" {
		return "", ErrMalformedAggregatedToolName
	}
	if strings.Contains(serverName, NamespaceSeparator) || strings.Contains(toolName, NamespaceSeparator) {
		return "", ErrMalformedAggregatedToolName
	}
	return fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, toolName), nil
}

// getServerID returns a new serverID using the server name.
func getServerID(serverName string) string {
	serverStr := "centian_server"
	if serverName != "" {
		serverStr = serverName
	}
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s_%d", serverStr, timestamp)
}

// getEndpointString returns a new endpoint path for the given gatewayName and mcpServerName.
func getEndpointString(gatewayName, mcpServerName string) (string, error) {
	if !common.IsURLCompliant(gatewayName) {
		return "", fmt.Errorf("gatewayName '%s' is not a compliant URL", gatewayName)
	}
	if mcpServerName != "" && !common.IsURLCompliant(mcpServerName) {
		return "", fmt.Errorf("mcpServerName '%s' is not a compliant URL", mcpServerName)
	}
	result := fmt.Sprintf("/mcp/%s", gatewayName)
	if mcpServerName != "" {
		result = fmt.Sprintf("%s/%s", result, mcpServerName)
	}
	return result, nil
}
