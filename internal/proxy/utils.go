package proxy

import (
	"fmt"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/google/uuid"
)

// NamespaceSeparator is used to create tool names in an aggregated proxy server.
const NamespaceSeparator = "___"

// parseAggregatedToolName validates an aggregated tool name and returns the
// downstream tool name that should be used for processing and dispatch.
//
// Note: this is to be used BEFORE processing the request, otherwise it
// might return false-positive errors!
func parseAggregatedToolName(rawName, expectedServer string) (string, error) {
	parts := strings.SplitN(rawName, NamespaceSeparator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid aggregated tool name %q", rawName)
	}
	if strings.Contains(parts[1], NamespaceSeparator) {
		return "", fmt.Errorf("invalid aggregated tool name %q", rawName)
	}
	if expectedServer != "" && parts[0] != expectedServer {
		return "", fmt.Errorf("aggregated tool %q targets server %q, expected %q", rawName, parts[0], expectedServer)
	}
	return parts[1], nil
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
