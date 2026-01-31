package proxy

import (
	"fmt"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/google/uuid"
)

// NamespaceSeparator is used to create tool names in an aggregated proxy server.
const NamespaceSeparator = "___"

func getNewUUIDV7() string {
	result := ""
	if id, err := uuid.NewV7(); err == nil {
		result = id.String()
	}
	if result == "" {
		result = fmt.Sprintf("req_%d", time.Now().UnixMicro())
	}
	return result
}

// getServerID returns a new serverID using the server name.
func getServerID(serverName string) string {
	// TODO: better way of determining server ID.
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
