package proxy

import (
	"fmt"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/google/uuid"
)

// MaxBodySize represents the maximal allowed size of a request/response body.
const MaxBodySize = 10 * 1024 * 1024 // 10MB

// NamespaceSeparator is used to create tool names in an aggregated proxy server
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
	result := fmt.Sprintf("/mcp/%s", gatewayName)
	if mcpServerName != "" {
		result = fmt.Sprintf("%s/%s", result, mcpServerName)
	}
	if !common.IsURLCompliant(result) {
		return "", fmt.Errorf("endpoint '%s' is not a compliant URL", result)
	}
	return result, nil
}
