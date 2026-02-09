package integrationtests

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPassthroughProcessor(t *testing.T) {
	processorConfig := createProcessorConfig("passthrough", "processors/passthrough.py")
	input := requestContext(t, "query_database", map[string]any{"query": "SELECT 1"})

	output, err := executeProcessor(processorConfig, input)
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	if output.Payload == nil || output.Payload.Request == nil || output.Payload.Request.Params == nil {
		t.Fatal("Expected request payload in output")
	}

	if !jsonEqual(t, input.Payload.Request.Params.Arguments, output.Payload.Request.Params.Arguments) {
		t.Errorf("Payload was modified by passthrough processor")
	}
}

func TestSecurityValidatorAllowsNormalRequests(t *testing.T) {
	processorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := requestContext(t, "tools/list", map[string]any{"q": "safe"})

	output, err := executeProcessor(processorConfig, input)
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	if output.Event == nil {
		t.Fatal("Expected event in output")
	}
	if output.Event.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Event.Status)
	}
}

func TestSecurityValidatorBlocksDeleteRequests(t *testing.T) {
	processorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := requestContext(t, "tools/delete_user", map[string]any{"user_id": 42})

	output, err := executeProcessor(processorConfig, input)
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	if output.Event == nil {
		t.Fatal("Expected event in output")
	}
	if output.Event.Status != 403 {
		t.Errorf("Expected status 403, got %d", output.Event.Status)
	}
	if output.Event.Error != "Delete operations not allowed" {
		t.Errorf("Expected error 'Delete operations not allowed', got '%s'", output.Event.Error)
	}
	if output.Event.Success {
		t.Errorf("Expected success=false for blocked request")
	}
}

func TestRequestLoggerPassesThrough(t *testing.T) {
	processorConfig := createProcessorConfig("request_logger", "processors/request_logger.py")
	input := requestContext(t, "tools/call", map[string]any{"a": 1})

	output, err := executeProcessor(processorConfig, input)
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	if !jsonEqual(t, input.Payload.Request.Params.Arguments, output.Payload.Request.Params.Arguments) {
		t.Errorf("Payload was modified by logger processor")
	}
}

func TestPayloadTransformerModifiesRequest(t *testing.T) {
	processorConfig := createProcessorConfig("payload_transformer", "processors/payload_transformer.py")
	input := requestContext(t, "tools/call", map[string]any{"query": "SELECT * FROM users"})

	output, err := executeProcessor(processorConfig, input)
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	var args map[string]any
	if err := json.Unmarshal(output.Payload.Request.Params.Arguments, &args); err != nil {
		t.Fatalf("Failed to decode output args: %v", err)
	}

	if args["x-processor"] != "payload_transformer" {
		t.Errorf("Expected x-processor=payload_transformer, got %v", args["x-processor"])
	}
}

func TestProcessorWithResponseData(t *testing.T) {
	processorConfig := createProcessorConfig("passthrough", "processors/passthrough.py")
	input := responseContext(t, "Query executed successfully")

	output, err := executeProcessor(processorConfig, input)
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	if output.Payload == nil || output.Payload.Result == nil {
		t.Fatal("Expected response payload result")
	}
}

func TestProcessorChain(t *testing.T) {
	loggerConfig := createProcessorConfig("request_logger", "processors/request_logger.py")
	validatorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := requestContext(t, "tools/list", map[string]any{"q": "safe"})

	output1, err := executeProcessor(loggerConfig, input)
	if err != nil {
		t.Fatalf("Logger processor failed: %v", err)
	}

	output2, err := executeProcessor(validatorConfig, output1)
	if err != nil {
		t.Fatalf("Validator processor failed: %v", err)
	}

	if output2.Event == nil || output2.Event.Status != 200 {
		t.Errorf("Expected status 200, got %+v", output2.Event)
	}
}

func TestProcessorChainWithRejection(t *testing.T) {
	passthroughConfig := createProcessorConfig("passthrough", "processors/passthrough.py")
	validatorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := requestContext(t, "tools/delete_user", map[string]any{"user_id": 42})

	output1, err := executeProcessor(passthroughConfig, input)
	if err != nil {
		t.Fatalf("Passthrough processor failed: %v", err)
	}

	output2, err := executeProcessor(validatorConfig, output1)
	if err != nil {
		t.Fatalf("Validator processor failed: %v", err)
	}

	if output2.Event == nil || output2.Event.Status != 403 {
		t.Errorf("Expected status 403, got %+v", output2.Event)
	}
}

func executeProcessor(processorConfig *config.ProcessorConfig, input *processor.DataContext) (*processor.DataContext, error) {
	p, err := processor.NewProcessor(processorConfig)
	if err != nil {
		return nil, err
	}
	return p.Process(input)
}

func createProcessorConfig(name, scriptPath string) *config.ProcessorConfig {
	absPath, _ := filepath.Abs(scriptPath)
	return &config.ProcessorConfig{
		Name:    name,
		Type:    "cli",
		Enabled: true,
		Timeout: 15,
		Config: map[string]interface{}{
			"command": "python3",
			"args":    []interface{}{absPath},
		},
	}
}

func requestContext(t *testing.T, toolName string, args map[string]any) *processor.DataContext {
	t.Helper()
	rawArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Failed to marshal args: %v", err)
	}
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      toolName,
			Arguments: rawArgs,
		},
	}
	return &processor.DataContext{
		Version: "1.0",
		Event: &common.MCPEvent{
			BaseMcpEvent: common.BaseMcpEvent{
				Status:      200,
				Success:     true,
				MessageType: common.MessageTypeRequest,
				Direction:   common.DirectionClientToServer,
			},
		},
		Payload: &processor.PayloadPart{
			Request: req,
		},
		Routing: &processor.RoutingPart{
			ServerName: "test_server",
			ToolName:   toolName,
		},
	}
}

func responseContext(t *testing.T, text string) *processor.DataContext {
	t.Helper()
	return &processor.DataContext{
		Version: "1.0",
		Event: &common.MCPEvent{
			BaseMcpEvent: common.BaseMcpEvent{
				Status:      200,
				Success:     true,
				MessageType: common.MessageTypeResponse,
				Direction:   common.DirectionServerToClient,
			},
		},
		Payload: &processor.PayloadPart{
			Result: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: text}},
			},
		},
	}
}

func jsonEqual(t *testing.T, a, b interface{}) bool {
	t.Helper()
	aJSON, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Failed to marshal first object: %v", err)
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Failed to marshal second object: %v", err)
	}
	return bytes.Equal(aJSON, bJSON)
}
