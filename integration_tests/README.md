# Processor Integration Tests

This directory contains end-to-end integration tests for the Centian processor system.

## Overview

Integration tests verify that real processor scripts work correctly with the processor execution engine. Unlike unit tests, these tests execute actual Python scripts and validate the complete request/response flow.

## Directory Structure

```
integration_tests/
├── README.md                      # This file
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

## Test Coverage

### Processor Types

The integration tests cover all major processor patterns:

1. **Passthrough** - No-op processor for testing baseline functionality
2. **Validator** - Accept/reject based on rules (security policies)
3. **Logger** - Side effects with passthrough behavior
4. **Transformer** - Payload modification

### Test Scenarios

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

## Running Tests

### Run All Integration Tests

```bash
make test-integration
```

Or directly with Go:

```bash
go test -v ./integration_tests/...
```

### Run Specific Test

```bash
go test -v ./integration_tests -run TestPassthroughProcessor
```

### Run with Coverage

```bash
go test -v -coverprofile=coverage.out ./integration_tests/...
go tool cover -html=coverage.out
```

## Test Processors

### passthrough.py

**Purpose**: Returns the input payload unchanged.

**Use Cases**:
- Baseline functionality testing
- Debugging processor chain flow
- Template for new processors

**Expected Behavior**:
- Status: 200
- Payload: Unchanged
- Error: null

### security_validator.py

**Purpose**: Blocks requests containing "delete" in the method name.

**Use Cases**:
- Security policy enforcement
- Request validation
- Testing rejection flow

**Expected Behavior**:
- Normal requests: Status 200, payload unchanged
- Delete requests: Status 403, error message, empty payload

### request_logger.py

**Purpose**: Logs request metadata to a file and passes through.

**Use Cases**:
- Audit logging
- Monitoring
- Side effect testing

**Expected Behavior**:
- Status: 200
- Payload: Unchanged
- Side effect: Writes to `~/centian/logs/processor.log`

### payload_transformer.py

**Purpose**: Adds custom `x-processor` header to request arguments.

**Use Cases**:
- Payload enrichment
- Request modification
- Header injection

**Expected Behavior**:
- Status: 200
- Payload: Modified with `x-processor` header
- Metadata: Lists modifications

## Test Data Fixtures

### request_normal.json

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

### request_delete.json

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

### response_success.json

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

## Adding New Tests

### 1. Create a Processor Script

Use the scaffold generator:

```bash
./scripts/create-processor.sh
```

Or manually create in `processors/`:

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

### 2. Create Test Fixture (if needed)

Add to `testdata/`:

```json
{
  "type": "request",
  "timestamp": "2025-12-28T10:00:00Z",
  "connection": {...},
  "payload": {...},
  "metadata": {...}
}
```

### 3. Write Go Test

Add to `processor_test.go`:

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

### 4. Run Tests

```bash
go test -v ./integration_tests -run TestMyNewProcessor
```

## Processor Contract

All processors must adhere to the processor contract:

### Input (via stdin)

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

### Output (via stdout)

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

### Exit Codes

- **0**: Processor executed successfully (check JSON status for decision)
- **≠0**: Processor crashed (treated as 50x error)

### Status Codes

- **200**: Success, continue to next processor
- **40x**: Client error, reject request (403 = forbidden, 400 = bad request)
- **50x**: Server error, internal processor failure

## Troubleshooting

### Test Fails with "permission denied"

Ensure processor scripts are executable:

```bash
chmod +x integration_tests/processors/*.py
```

### Test Fails with "python3: command not found"

Install Python 3 or update processor config to use correct interpreter path.

### Test Timeout

Increase timeout in test or processor config:

```go
processorConfig.Timeout = 30 // 30 seconds
```

### Processor Not Found

Ensure paths are correct. Integration tests use relative paths from the test directory:

```go
absPath, _ := filepath.Abs("processors/my_processor.py")
```

## CI/CD Integration

These tests run in GitHub Actions on every PR and commit to main:

```yaml
- name: Run Integration Tests
  run: make test-integration
```

Ensure all tests pass before merging.

## Reference Documentation

- [Processor Development Guide](../docs/processor_development_guide.md)
- [Processor Requirements](../.tmp/processor-system-requirements.md)
- [Issue #16 - Processor System](https://github.com/CentianAI/centian-cli/issues/16)

## Contributing

When adding new processors or test cases:

1. Follow the existing test patterns
2. Use descriptive test names
3. Include Given-When-Then comments
4. Test both success and failure paths
5. Verify processors work standalone before integrating

## License

Same as the main project.
