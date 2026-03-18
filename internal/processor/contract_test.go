package processor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// contractV1Dir is the path to the v1 golden fixtures, relative to this package.
const contractV1Dir = "../../tests/contracts/v1"

const contractViolationMsg = `
CONTRACT VIOLATION (v1): The serialized DataContext no longer matches the golden fixture.
This is a DATA CONTRACT change that affects all external processors (CLI and webhook).

DO NOT update the golden fixture in tests/contracts/v1/ — that would silently break
processors written against the v1 contract.

If this is a BREAKING change (field removed, renamed, or semantically incompatible):
  1. Create tests/contracts/v2/ with new golden fixtures
  2. Bump CurrentDataContextVersion to "2.0"
  3. Update scaffold templates and docs to reflect the new contract

If this is an ADDITIVE change (new optional field that processors can safely ignore):
  1. Update both v1 fixtures (existing processors remain compatible)
  2. Bump CurrentDataContextVersion minor to "1.1"
`

// contractFixtureTime is a fixed, deterministic timestamp used for fixture generation.
// Using a zero timestamp would serialize as "0001-01-01T00:00:00Z" which is unclear.
var contractFixtureTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// buildContractInputDataContext returns a representative, fully-populated DataContext
// used to generate and validate the processor input (proxy → processor) golden fixture.
// All values are stable and deterministic.
func buildContractInputDataContext() *DataContext {
	rawArgs := json.RawMessage(`{"key":"value"}`)

	event := &common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			Status:      0,
			Timestamp:   contractFixtureTime,
			Transport:   "http",
			RequestID:   "req-contract-v1",
			Direction:   common.DirectionClientToServer,
			MessageType: common.MessageTypeRequest,
			Success:     true,
			Modified:    false,
		},
		Routing: common.RoutingContext{
			Transport:  common.HTTPTransport,
			ServerName: "test-server",
		},
	}

	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test_tool",
			Arguments: rawArgs,
		},
	}

	return &DataContext{
		Version: CurrentDataContextVersion,
		Event:   event,
		Payload: &PayloadPart{
			Request:         request,
			OriginalRequest: request,
		},
		Routing: &RoutingPart{
			ServerName:         "test-server",
			ToolName:           "test_tool",
			OriginalServerName: "test-server",
			OriginalToolname:   "test_tool",
		},
		Auth: &common.AuthContext{
			Authenticated: true,
			PrincipalID:   "test-user",
			PrincipalType: "api_key",
		},
	}
}

// buildContractOutputJSON returns the JSON representation of a representative processor
// output (processor → proxy direction). Uses a plain map to avoid MCP SDK type uncertainties.
func buildContractOutputJSON() ([]byte, error) {
	output := map[string]interface{}{
		"version": CurrentDataContextVersion,
		"payload": map[string]interface{}{
			"result": map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "processed result",
					},
				},
				"isError": false,
			},
		},
	}
	return json.Marshal(output)
}

// TestGenerateContractFixtures generates the golden fixture files from the current
// DataContext serialisation. Run this explicitly when intentionally bumping the contract version.
//
//	go test ./internal/processor/ -run TestGenerateContractFixtures
//
// Commit the generated files under tests/contracts/v1/ to lock the contract.
func TestGenerateContractFixtures(t *testing.T) {
	if err := os.MkdirAll(contractV1Dir, 0o755); err != nil {
		t.Fatalf("Failed to create fixture directory: %v", err)
	}

	// Generate processor_input.json (proxy → processor direction, via DTO)
	inputCtx := buildContractInputDataContext()
	inputBytes, err := marshalProcessorInput(inputCtx)
	if err != nil {
		t.Fatalf("Failed to marshal input DataContext: %v", err)
	}
	writeFixtureFile(t, filepath.Join(contractV1Dir, "processor_input.json"), inputBytes)

	// Generate processor_output.json (processor → proxy direction)
	outputBytes, err := buildContractOutputJSON()
	if err != nil {
		t.Fatalf("Failed to build output JSON: %v", err)
	}
	writeFixtureFile(t, filepath.Join(contractV1Dir, "processor_output.json"), outputBytes)

	t.Log("Golden fixtures generated — commit tests/contracts/v1/ to lock the contract.")
}

func writeFixtureFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	pretty, err := prettyFormatJSON(raw)
	if err != nil {
		t.Fatalf("Failed to format fixture JSON: %v", err)
	}
	if err := os.WriteFile(path, pretty, 0o644); err != nil {
		t.Fatalf("Failed to write fixture %s: %v", path, err)
	}
	t.Logf("Wrote %s", path)
}

// TestProcessorInputContractV1 validates that the current DataContext serialisation
// (proxy → processor direction) has not drifted from the v1 golden fixture.
//
// A failure here is a DATA CONTRACT VIOLATION — not a test configuration issue.
// Read the error message for guidance on how to proceed.
func TestProcessorInputContractV1(t *testing.T) {
	// Given: a representative DataContext with all fields populated
	inputCtx := buildContractInputDataContext()

	// When: serialising as the proxy would before sending to a processor
	serialized, err := marshalProcessorInput(inputCtx)
	if err != nil {
		t.Fatalf("Failed to serialize DataContext: %v", err)
	}

	// Then: the result must match the golden fixture exactly
	fixturePath := filepath.Join(contractV1Dir, "processor_input.json")
	golden, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf(
			"Golden fixture not found at %s\nRun: go test ./internal/processor/ -run TestGenerateContractFixtures",
			fixturePath,
		)
	}

	if !jsonDeepEqual(t, serialized, golden) {
		pretty, _ := prettyFormatJSON(serialized)
		t.Fatalf("%s\nGolden (expected):\n%s\n\nActual (got):\n%s",
			contractViolationMsg, string(golden), string(pretty))
	}
}

// TestProcessorOutputContractV1 validates that the proxy can successfully parse the v1
// golden output fixture (processor → proxy direction) without errors, and that the
// unmarshalled DataContext exposes the expected top-level fields.
//
// A failure here means the proxy's DataContext no longer understands v1 processor output.
func TestProcessorOutputContractV1(t *testing.T) {
	// Given: the v1 golden output fixture
	fixturePath := filepath.Join(contractV1Dir, "processor_output.json")
	golden, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf(
			"Golden fixture not found at %s\nRun: go test ./internal/processor/ -run TestGenerateContractFixtures",
			fixturePath,
		)
	}

	// When: parsing as the proxy would after receiving output from a processor
	output, err := unmarshalProcessorOutput(golden)
	if err != nil {
		t.Fatalf("%s\nProxy failed to parse v1 output fixture: %v", contractViolationMsg, err)
	}

	// Then: key fields must be accessible
	if output.Version != CurrentDataContextVersion {
		t.Errorf("Expected version %q in parsed output, got %q", CurrentDataContextVersion, output.Version)
	}
	if output.Payload == nil {
		t.Fatal("Expected non-nil payload in parsed output fixture")
	}
	if output.Payload.Result == nil {
		t.Fatal("Expected non-nil result in parsed output payload")
	}
}

// jsonDeepEqual compares two JSON byte slices for structural equality.
// Field insertion order is irrelevant — only the logical structure matters.
func jsonDeepEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var aVal, bVal interface{}
	if err := json.Unmarshal(a, &aVal); err != nil {
		t.Fatalf("Failed to unmarshal actual JSON for comparison: %v", err)
	}
	if err := json.Unmarshal(b, &bVal); err != nil {
		t.Fatalf("Failed to unmarshal golden JSON for comparison: %v", err)
	}
	return reflect.DeepEqual(aVal, bVal)
}

// prettyFormatJSON formats a JSON byte slice with consistent indentation.
func prettyFormatJSON(raw []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return json.MarshalIndent(v, "", "  ")
}
