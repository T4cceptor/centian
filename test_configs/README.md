# Test Configuration Files

This directory contains configuration files used for testing and examples.

## Integration Test Configs

### `integrationtest_config.json`
Configuration used by automated integration tests in `internal/cli/server_integrationtest.go`.
- Uses mock MCP servers on localhost ports 8888-8889
- Demonstrates multiple gateways and servers
- Shows metadata usage

### `example_http_proxy_config.json`
Reference configuration showing HTTP proxy setup with real-world patterns:
- Environment variable substitution (`${GITHUB_PAT}`)
- Multiple gateways for different environments
- Proper header configuration
- Descriptive metadata

## Discovery Test Configs

### `vscode_mcp.json`, `claude_desktop_config.json`
Test configurations for MCP server discovery from IDE settings.

### `duplicate_test_config*.json`
Configurations used to test duplicate server detection during discovery.

### `current_project_mcp.json`
Project-specific MCP configuration for testing.

## Usage

### Running Integration Tests

```bash
# Run all CLI integration tests
go test -v ./internal/cli -run TestServerStartIntegration

# Run config validation tests
go test -v ./internal/cli -run TestConfigFileValidation
```

### Using Example Configs

```bash
# Start server with example config
centian server start --config-path ./test_configs/example_http_proxy_config.json

# Note: You'll need to set environment variables referenced in configs
export GITHUB_PAT=your_github_token_here

# Note: With auth enabled, generate a key and store it
centian server get-key
```

## Config Structure

All HTTP proxy configs follow this structure:

```json
{
  "name": "Server Name",
  "version": "1.0.0",
  "auth": true,
  "authHeader": "X-Centian-Auth",
  "proxy": {
    "port": "8080",
    "timeout": 30,
    "logLevel": "info"
  },
  "gateways": {
    "gateway-name": {
      "mcpServers": {
        "server-name": {
          "url": "https://example.com/mcp/",
          "headers": {
            "Authorization": "Bearer ${ENV_VAR}",
            "Content-Type": "application/json"
          },
          "enabled": true,
          "description": "Server description"
        }
      }
    }
  },
  "processors": [],
  "metadata": {}
}
```

### Endpoint Patterns

Servers are accessible at: `http://localhost:{port}/mcp/{gateway}/{server}`

Example:
- Config: `gateways.production.mcpServers.github`
- Endpoint: `http://localhost:8080/mcp/production/github`

### Environment Variables

Headers support environment variable substitution:
- `${VAR_NAME}` - Standard format
- `$VAR_NAME` - Also supported

Example: `"Authorization": "Bearer ${GITHUB_PAT}"` will substitute the value of the `GITHUB_PAT` environment variable.
