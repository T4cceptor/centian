package processor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestNewCLIProcessorAndGetConfig(t *testing.T) {
	cfg := &config.ProcessorConfig{
		Name:    "demo",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "cat",
		},
	}

	p, err := NewCLIProcessor(cfg)
	assert.NilError(t, err)
	assert.Assert(t, p.WorkingDir != "")
	assert.Assert(t, p.GetConfig() == cfg)
}

func TestExtractCommandAndArgs(t *testing.T) {
	t.Run("success with args", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name: "demo",
			Config: map[string]interface{}{
				"command": "python3",
				"args":    []interface{}{"script.py", "--flag"},
			},
		}

		command, args, err := extractCommandAndArgs(cfg)
		assert.NilError(t, err)
		assert.Equal(t, command, "python3")
		assert.DeepEqual(t, args, []string{"script.py", "--flag"})
	})

	t.Run("success without args", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name: "demo",
			Config: map[string]interface{}{
				"command": "cat",
			},
		}

		command, args, err := extractCommandAndArgs(cfg)
		assert.NilError(t, err)
		assert.Equal(t, command, "cat")
		assert.Equal(t, len(args), 0)
	})

	t.Run("invalid command type", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name: "demo",
			Config: map[string]interface{}{
				"command": 123,
			},
		}

		_, _, err := extractCommandAndArgs(cfg)
		assert.Assert(t, err != nil)
	})

	t.Run("invalid args type", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name: "demo",
			Config: map[string]interface{}{
				"command": "cat",
				"args":    "not-an-array",
			},
		}

		_, _, err := extractCommandAndArgs(cfg)
		assert.Assert(t, err != nil)
	})

	t.Run("invalid args element type", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name: "demo",
			Config: map[string]interface{}{
				"command": "cat",
				"args":    []interface{}{"ok", 42},
			},
		}

		_, _, err := extractCommandAndArgs(cfg)
		assert.Assert(t, err != nil)
	})
}

func TestCloneRequestForDTO(t *testing.T) {
	assert.Assert(t, cloneRequestForDTO(nil) == nil)
	assert.Assert(t, cloneRequestForDTO(&mcp.CallToolRequest{}) == nil)

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "tool-a",
			Arguments: json.RawMessage(`{"a":1}`),
		},
	}

	dto := cloneRequestForDTO(req)
	assert.Assert(t, dto != nil)
	assert.Equal(t, dto.Params.Name, "tool-a")
	assert.Equal(t, string(dto.Params.Arguments), `{"a":1}`)

	req.Params.Arguments = json.RawMessage(`{"a":2}`)
	assert.Equal(t, string(dto.Params.Arguments), `{"a":1}`)
}

func TestMarshalProcessorInput_NilInput(t *testing.T) {
	encoded, err := marshalProcessorInput(nil)
	assert.NilError(t, err)

	var decoded map[string]any
	assert.NilError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, len(decoded), 0)
}

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

func TestMarshalProcessorInput_IncludesAuthPart(t *testing.T) {
	input := &DataContext{
		Version: "1.0",
		Auth: &common.AuthContext{
			Authenticated: true,
			PrincipalID:   "principal-1",
			PrincipalType: "api_key",
			KeyID:         "key_1",
			Gateway:       "default",
		},
	}

	encoded, err := marshalProcessorInput(input)
	assert.NilError(t, err)

	var decoded map[string]any
	assert.NilError(t, json.Unmarshal(encoded, &decoded))

	authPart, ok := decoded["auth"].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, authPart["principal_id"], "principal-1")
	assert.Equal(t, authPart["principal_type"], "api_key")
	assert.Equal(t, authPart["key_id"], "key_1")
}

func TestCLIProcessorProcess(t *testing.T) {
	makeContext := func() *DataContext {
		return &DataContext{
			Version: "1.0",
			Payload: &PayloadPart{
				Request: &mcp.CallToolRequest{
					Params: &mcp.CallToolParamsRaw{
						Name:      "tool-a",
						Arguments: json.RawMessage(`{"a":1}`),
					},
				},
			},
		}
	}

	t.Run("success roundtrip with cat", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "cat",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"command": "cat",
			},
		}

		p := &CLIProcessor{WorkingDir: t.TempDir(), config: cfg}
		out, err := p.Process(makeContext())
		assert.NilError(t, err)
		assert.Assert(t, out != nil)
		assert.Assert(t, out.Payload != nil)
		assert.Assert(t, out.Payload.Request != nil)
		assert.Assert(t, out.Payload.Request.Params != nil)
		assert.Equal(t, out.Payload.Request.Params.Name, "tool-a")
	})

	t.Run("invalid config command returns error", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "bad-cfg",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"command": 1,
			},
		}

		p := &CLIProcessor{WorkingDir: t.TempDir(), config: cfg}
		_, err := p.Process(makeContext())
		assert.Assert(t, err != nil)
	})

	t.Run("execution error includes stderr", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "exec-fail",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"command": "sh",
				"args":    []interface{}{"-c", "echo fail-msg 1>&2; exit 7"},
			},
		}

		p := &CLIProcessor{WorkingDir: t.TempDir(), config: cfg}
		_, err := p.Process(makeContext())
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "execution failed"))
		assert.Assert(t, strings.Contains(err.Error(), "stderr: fail-msg"))
	})

	t.Run("invalid json output returns error with stdout", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "invalid-json",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"command": "sh",
				"args":    []interface{}{"-c", "echo not-json"},
			},
		}

		p := &CLIProcessor{WorkingDir: t.TempDir(), config: cfg}
		_, err := p.Process(makeContext())
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "returned invalid JSON"))
		assert.Assert(t, strings.Contains(err.Error(), "stdout: not-json"))
	})

	t.Run("timeout returns timeout error", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "timeout",
			Type:    "cli",
			Enabled: true,
			Timeout: 1,
			Config: map[string]interface{}{
				"command": "sh",
				"args":    []interface{}{"-c", "sleep 2"},
			},
		}

		p := &CLIProcessor{WorkingDir: t.TempDir(), config: cfg}
		_, err := p.Process(makeContext())
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "timed out"))
	})
}
