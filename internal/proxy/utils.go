package proxy

import (
	"fmt"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
)

// getServerID returns a new serverID using the server name.
func getServerID(globalConfig *config.GlobalConfig) string {
	// TODO: better way of determining server ID.
	serverStr := "centian_server"
	if globalConfig.Name != "" {
		serverStr = globalConfig.Name
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
