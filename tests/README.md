# Tests

This folder contains end-to-end integration tests and test configuration fixtures for Centian.

## Directory Structure

```
tests/
├── README.md                     # This file
├── integrationtests/             # Go integration tests
│   ├── processor_test.go         # Go integration tests
│   ├── processors/               # Example processor scripts
│   └── testdata/                 # Test input fixtures
└── test_configs/                 # Test configuration fixtures
```

## Integration Tests

This directory contains end-to-end integration tests for the Centian.

### Processors

Integration tests verify that real processor scripts work correctly with the processor execution engine. Unlike unit tests, these tests execute actual Python scripts and validate the complete request/response flow.

### Directory Structure

```
tests/integrationtests/
├── processor_test.go              # Go integration tests
├── processors/                    # Example processor scripts
│   ├── passthrough.py            # Returns input unchanged
│   ├── security_validator.py     # Blocks delete operations
│   ├── request_logger.py         # Logs requests to file
│   └── payload_transformer.py    # Adds custom headers
└── testdata/                      # Test input fixtures
    ├── request_normal.json       # Normal tool call request
    ├── request_delete.json       # Delete operation request
    ├── request_malformed.json    # Malformed request data
    └── response_success.json     # Successful response
```

### Test Coverage

#### Processor Types

The integration tests cover several major processor patterns:

1. **Passthrough** - No-op processor for testing baseline functionality
2. **Validator** - Accept/reject based on rules (security policies)
3. **Logger** - Side effects with passthrough behavior
4. **Transformer** - Payload modification

#### Test Scenarios

| Test | Description | Validates |
|------|-------------|-----------|
| `TestPassthroughProcessor` | Passthrough returns input unchanged | Basic execution, status 200, metadata |
| `TestSecurityValidatorAllowsNormalRequests` | Normal requests pass through | Validator logic with safe input |
| `TestSecurityValidatorBlocksDeleteRequests` | Delete operations rejected | Validator logic with blocked input, status 403 |
| `TestRequestLoggerPassesThrough` | Logger doesn't modify payload | Side effect processors |
| `TestPayloadTransformerModifiesRequest` | Transformer adds headers | Payload modification |
| `TestProcessorWithResponseData` | Response messages handled | Request vs response handling |
| `TestProcessorChain` | Multiple processors execute sequentially | Processor chaining |
| `TestProcessorChainWithRejection` | Rejection stops the chain | Fail-fast behavior |

### Running Tests

#### Run All Integration Tests

```bash
make test-integration
```

Or directly with Go:

```bash
go test -v ./tests/integrationtests/...
```

#### Run Specific Test

```bash
go test -v ./tests/integrationtests -run TestPassthroughProcessor
```

#### Run with Coverage

```bash
go test -v -coverprofile=coverage.out ./tests/integrationtests/...
go tool cover -html=coverage.out
```

### Test Processors

- **passthrough.py**

    **Purpose**: Returns the input payload unchanged.

    **Use Cases**:
    - Baseline functionality testing
    - Debugging processor chain flow
    - Template for new processors

    **Expected Behavior**:
    - Status: 200
    - Payload: Unchanged
    - Error: null

- **security_validator.py**

    **Purpose**: Blocks requests containing "delete" in the method name.

    **Use Cases**:
    - Security policy enforcement
    - Request validation
    - Testing rejection flow

    **Expected Behavior**:
    - Normal requests: Status 200, payload unchanged
    - Delete requests: Status 403, error message, empty payload

- **request_logger.py**

    **Purpose**: Logs request metadata to a file and passes through.

    **Use Cases**:
    - Audit logging
    - Monitoring
    - Side effect testing

    **Expected Behavior**:
    - Status: 200
    - Payload: Unchanged
    - Side effect: Writes to `~/centian/logs/processor.log`

- **payload_transformer.py**

    **Purpose**: Adds custom `x-processor` header to request arguments.

    **Use Cases**:
    - Payload enrichment
    - Request modification
    - Header injection

    **Expected Behavior**:
    - Status: 200
    - Payload: Modified with `x-processor` header
    - Metadata: Lists modifications

### Test Data Fixtures

#### request_normal.json

Standard tool call request with safe parameters.

```json
{
  "type": "request",
  "payload": {
    "method": "tools/call",
    "params": {
      "name": "query_database",
      "arguments": {...}
    }
  }
}
```

#### request_delete.json

Request that should be blocked by security validator.

```json
{
  "type": "request",
  "payload": {
    "method": "tools/delete_user",
    ...
  }
}
```

#### response_success.json

Successful response from MCP server.

```json
{
  "type": "response",
  "payload": {
    "jsonrpc": "2.0",
    "result": {...}
  }
}
```

### Adding a New Test

#### 1. Create a Processor Script

Use the scaffold generator:

```bash
./scripts/create-processor.sh
```

Or manually create in `tests/integrationtests/processors/`:

```python
#!/usr/bin/env python3
import sys
import json

event = json.load(sys.stdin)

# Your logic here

print(json.dumps({
    "status": 200,
    "payload": event["payload"],
    "error": None,
    "metadata": {"processor_name": "my_processor"}
}))
```

#### 2. Create Test Fixture (if needed)

Add to `tests/integrationtests/testdata/`:

```json
{
  "type": "request",
  "timestamp": "2025-12-28T10:00:00Z",
  "connection": {...},
  "payload": {...},
  "metadata": {...}
}
```

#### 3. Write Go Test

Add to `tests/integrationtests/processor_test.go`:

```go
func TestMyNewProcessor(t *testing.T) {
    // Given: a processor configuration
    processorConfig := createProcessorConfig("my_processor", "processors/my_processor.py")
    input := loadTestInput(t, "testdata/my_test.json")

    // When: executing the processor
    output, err := executeProcessor(t, processorConfig, input)

    // Then: verify expectations
    if err != nil {
        t.Fatalf("Processor execution failed: %v", err)
    }

    // Add assertions...
}
```

#### 4. Run Tests

```bash
go test -v ./tests/integrationtests -run TestMyNewProcessor
```

### Processor Contract

All processors must adhere to the processor contract:

#### Input (via stdin)

```json
{
  "type": "request" | "response",
  "timestamp": "ISO 8601",
  "connection": {
    "server_name": "string",
    "transport": "string",
    "session_id": "string"
  },
  "payload": {...},
  "metadata": {
    "processor_chain": [],
    "original_payload": {...}
  }
}
```

#### Output (via stdout)

```json
{
  "status": 200 | 40x | 50x,
  "payload": {...},
  "error": "string" | null,
  "metadata": {
    "processor_name": "string",
    "modifications": []
  }
}
```

#### Exit Codes

- **0**: Processor executed successfully (check JSON status for decision)
- **≠0**: Processor crashed (treated as 50x error)

#### Status Codes

- **200**: Success, continue to next processor
- **40x**: Client error, reject request (403 = forbidden, 400 = bad request)
- **50x**: Server error, internal processor failure

### Processors: Troubleshooting

1. Test Fails with "permission denied"
    - Ensure processor scripts are executable:
      ```bash
      chmod +x tests/integrationtests/processors/*.py
      ```
2. Test Fails with "python3: command not found"
    - Install Python 3 or update processor config to use correct interpreter path.
3. Test Timeout
    - Increase timeout in test or processor config:
        ```go
        processorConfig.Timeout = 30 // 30 seconds
        ```

4. Processor Not Found
    - Ensure paths are correct. Integration tests use relative paths from `tests/integrationtests`:
        ```go
        absPath, _ := filepath.Abs("processors/my_processor.py")
        ```

## Test Configuration Files

This directory contains configuration files used for testing and examples.

### Integration Test Configs

#### `integration_test_config.json`
Configuration used by automated integration tests in `internal/cli/server_integrationtest.go`.
- Uses mock MCP servers on localhost ports 8888-8889
- Demonstrates multiple gateways and servers
- Shows metadata usage

#### `example_http_proxy_config.json`
Reference configuration showing HTTP proxy setup with real-world patterns:
- Environment variable substitution (`${GITHUB_PAT}`)
- Multiple gateways for different environments
- Proper header configuration
- Descriptive metadata

### Discovery Test Configs

#### `vscode_mcp.json`, `claude_desktop_config.json`
Test configurations for MCP server discovery from IDE settings.

#### `duplicate_test_config*.json`
Configurations used to test duplicate server detection during discovery.

#### `current_project_mcp.json`
Project-specific MCP configuration for testing.

### Usage

#### Running Integration Tests

```bash
# Run all CLI integration tests
go test -v ./internal/cli -run TestServerStartIntegration

# Run config validation tests
go test -v ./internal/cli -run TestConfigFileValidation
```

#### Using Example Configs

```bash
# Start server with example config
centian start --config-path ./tests/test_configs/example_http_proxy_config.json

# Note: You'll need to set environment variables referenced in configs
export GITHUB_PAT=your_github_token_here

# Note: With auth enabled, generate a key and store it
centian auth new-key
```

### Config Structure

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

#### Endpoint Patterns

Servers are accessible at: `http://localhost:{port}/mcp/{gateway}/{server}`

Example:
- Config: `gateways.production.mcpServers.github`
- Endpoint: `http://localhost:8080/mcp/production/github`

#### Environment Variables

Headers support environment variable substitution:
- `${VAR_NAME}` - Standard format
- `$VAR_NAME` - Also supported

Example: `"Authorization": "Bearer ${GITHUB_PAT}"` will substitute the value of the `GITHUB_PAT` environment variable.

## Reference Documentation

- [Processor Development Guide](../docs/processor_development_guide.md)

## Contributing

When adding new processors or test cases:

1. Follow the existing test patterns
2. Use descriptive test names
3. Include Given-When-Then comments
4. Test both success and failure paths
5. Verify processors work standalone before integrating

## License

Same as the main project.
