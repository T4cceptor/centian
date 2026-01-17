# HTTP Proxy Server Setup

The `centian server start` command launches an HTTP proxy server that forwards requests to configured MCP servers.

## Quick Start

### 1. Create a Configuration File

Create `~/.centian/config.jsonc` with your MCP server configurations:

```json
{
  "name": "My Centian Server",
  "version": "1.0.0",
  "proxy": {
    "port": "8080",
    "timeout": 30,
    "logLevel": "info"
  },
  "gateways": {
    "production": {
      "mcpServers": {
        "github": {
          "url": "https://api.githubcopilot.com/mcp/",
          "headers": {
            "Authorization": "Bearer ${GITHUB_PAT}",
            "Content-Type": "application/json"
          },
          "enabled": true,
          "description": "GitHub MCP server"
        }
      }
    }
  }
}
```

### 2. Set Environment Variables

If your configuration uses environment variable substitution:

```bash
export GITHUB_PAT=your_github_personal_access_token
```

### 3. Start the Server

```bash
# Using default config path (~/.centian/config.jsonc)
centian server start

# Or specify a custom config path
centian server start --config-path ./my-config.json
```

### 4. Access Your MCP Servers

Your configured servers are now accessible at:

```
http://localhost:{port}/mcp/{gateway}/{server}
```

Example:
- Config: `gateways.production.mcpServers.github`
- Endpoint: `http://localhost:8080/mcp/production/github`

## Configuration Reference

### Global Config Structure

```json
{
  "name": "string",           // Server name for identification
  "version": "string",        // Config schema version (required)
  "proxy": {
    "port": "string",         // HTTP server port (e.g., "8080")
    "timeout": number,        // Request timeout in seconds
    "logLevel": "string"      // Log level: debug, info, warn, error
  },
  "gateways": {
    "{gateway-name}": {
      "mcpServers": {
        "{server-name}": {
          "url": "string",              // MCP server URL
          "headers": {                  // HTTP headers to forward
            "key": "value"
          },
          "enabled": boolean,           // Whether server is active
          "description": "string"       // Human-readable description
        }
      }
    }
  },
  "processors": [],           // Processor chain (for future use)
  "metadata": {}             // Additional metadata
}
```

### Environment Variable Substitution

Headers support automatic environment variable substitution:

```json
{
  "headers": {
    "Authorization": "Bearer ${GITHUB_PAT}",
    "X-API-Key": "${API_KEY}",
    "X-Custom": "prefix-${ENV_VAR}-suffix"
  }
}
```

Supports both `${VAR}` and `$VAR` syntax.

### Multiple Gateways

Organize servers by environment, team, or purpose:

```json
{
  "gateways": {
    "production": {
      "mcpServers": {
        "github": { "url": "https://api.githubcopilot.com/mcp/" }
      }
    },
    "staging": {
      "mcpServers": {
        "github-staging": { "url": "https://staging.api.github.com/mcp/" }
      }
    },
    "development": {
      "mcpServers": {
        "local-server": { "url": "http://localhost:3000/mcp/" }
      }
    }
  }
}
```

## Usage Examples

### Example 1: Single Server Setup

```json
{
  "name": "Simple Setup",
  "version": "1.0.0",
  "proxy": {
    "port": "8080",
    "timeout": 30
  },
  "gateways": {
    "main": {
      "mcpServers": {
        "my-server": {
          "url": "http://localhost:3000/mcp/",
          "enabled": true
        }
      }
    }
  }
}
```

Access at: `http://localhost:8080/mcp/main/my-server`

### Example 2: Multiple Servers with Authentication

```json
{
  "name": "Multi-Server Setup",
  "version": "1.0.0",
  "proxy": {
    "port": "8080",
    "timeout": 30
  },
  "gateways": {
    "apis": {
      "mcpServers": {
        "github": {
          "url": "https://api.githubcopilot.com/mcp/",
          "headers": {
            "Authorization": "Bearer ${GITHUB_PAT}"
          },
          "enabled": true
        },
        "anthropic": {
          "url": "https://api.anthropic.com/mcp/",
          "headers": {
            "X-API-Key": "${ANTHROPIC_API_KEY}"
          },
          "enabled": true
        }
      }
    }
  }
}
```

Access at:
- `http://localhost:8080/mcp/apis/github`
- `http://localhost:8080/mcp/apis/anthropic`

### Example 3: Development vs Production

```json
{
  "name": "Multi-Environment Setup",
  "version": "1.0.0",
  "proxy": {
    "port": "8080",
    "timeout": 30
  },
  "gateways": {
    "production": {
      "mcpServers": {
        "api": {
          "url": "https://api.production.com/mcp/",
          "headers": {
            "Authorization": "Bearer ${PROD_TOKEN}"
          },
          "enabled": true
        }
      }
    },
    "development": {
      "mcpServers": {
        "api": {
          "url": "http://localhost:3000/mcp/",
          "enabled": true
        }
      }
    }
  }
}
```

Switch between environments by changing the endpoint:
- Production: `http://localhost:8080/mcp/production/api`
- Development: `http://localhost:8080/mcp/development/api`

## Testing

### Integration Tests

Run the integration tests to verify your setup:

```bash
# Run server integration tests
go test -v ./internal/cli -run TestServerStartIntegration

# Run config validation tests
go test -v ./internal/cli -run TestConfigFileValidation
```

### Manual Testing

1. Start the server:
   ```bash
   centian server start --config-path ./test_configs/example_http_proxy_config.json
   ```

2. In another terminal, test with curl:
   ```bash
   # List tools
   curl -X POST http://localhost:8080/mcp/production/github \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
   ```

## Troubleshooting

### Server Won't Start

**Problem**: `failed to load config: configuration file not found`

**Solution**:
- Run `centian init` to create default config
- Or specify config path: `centian server start --config-path ./config.json`

### Connection Refused

**Problem**: Client can't connect to proxy

**Solution**:
- Check server is running
- Verify port isn't already in use: `lsof -i :{port}`
- Check firewall settings

### Authentication Errors

**Problem**: Downstream server returns 401/403

**Solution**:
- Verify environment variables are set: `echo $GITHUB_PAT`
- Check header syntax in config
- Ensure variable names match exactly (case-sensitive)

### Invalid Configuration

**Problem**: `invalid configuration: version field is required`

**Solution**:
- Add `"version": "1.0.0"` to your config
- Validate JSON syntax
- Check required fields are present

## Graceful Shutdown

Press `Ctrl+C` to gracefully shutdown the server. The server will:
1. Stop accepting new connections
2. Wait for active requests to complete (up to 10 seconds)
3. Close all connections
4. Exit cleanly

## Next Steps

- [Configuration Management](./CONFIG_MANAGEMENT.md)
- [Processor Chains](./PROCESSORS.md)
- [API Reference](./API_REFERENCE.md)