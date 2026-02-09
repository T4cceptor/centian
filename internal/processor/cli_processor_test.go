package processor

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestMarshalProcessorInput_StripsUnmarshalableRequestFields(t *testing.T) {
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test_tool",
			Arguments: json.RawMessage(`{"key":"value"}`),
		},
		Extra: &mcp.RequestExtra{
			CloseSSEStream: func(mcp.CloseSSEStreamArgs) {},
		},
	}

	input := &DataContext{
		Version: "1.0",
		Payload: &PayloadPart{
			Request: req,
		},
	}

	encoded, err := marshalProcessorInput(input)
	assert.NilError(t, err)

	var decoded map[string]any
	assert.NilError(t, json.Unmarshal(encoded, &decoded))

	payload, ok := decoded["payload"].(map[string]any)
	assert.Assert(t, ok)

	request, ok := payload["request"].(map[string]any)
	assert.Assert(t, ok)

	params, ok := request["Params"].(map[string]any)
	assert.Assert(t, ok)

	assert.Equal(t, params["name"], "test_tool")

	_, hasExtra := request["Extra"]
	assert.Assert(t, !hasExtra)
	_, hasSession := request["Session"]
	assert.Assert(t, !hasSession)
}
