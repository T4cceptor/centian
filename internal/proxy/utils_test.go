package proxy

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gotest.tools/assert"
)

func TestGetNewUUIDV7(t *testing.T) {
	// Given: a UUID generator
	// When: generating a new ID
	id := getNewUUIDV7()

	// Then: the ID is non-empty
	assert.Assert(t, id != "")
}

func TestGetNewUUIDV7_FallbackOnUUIDError(t *testing.T) {
	originalGenerator := newUUIDV7
	newUUIDV7 = func() (uuid.UUID, error) {
		return uuid.UUID{}, errors.New("forced failure")
	}
	t.Cleanup(func() {
		newUUIDV7 = originalGenerator
	})

	id := getNewUUIDV7()
	assert.Assert(t, strings.HasPrefix(id, "req_"))
}

func TestGetServerID(t *testing.T) {
	// Given: server names
	// When: generating server IDs
	withName := getServerID("my-server")
	defaultName := getServerID("")

	// Then: IDs contain expected prefixes
	assert.Assert(t, strings.HasPrefix(withName, "my-server_"))
	assert.Assert(t, strings.HasPrefix(defaultName, "centian_server_"))
}

func TestGetEndpointString(t *testing.T) {
	// Given: gateway and server names
	// When: building endpoint strings
	gatewayOnly, err := getEndpointString("gateway", "")
	assert.NilError(t, err)
	full, err := getEndpointString("gateway", "server")
	assert.NilError(t, err)

	// Then: endpoints are formed correctly
	assert.Equal(t, gatewayOnly, "/mcp/gateway")
	assert.Equal(t, full, "/mcp/gateway/server")

	// Given: invalid names
	_, err = getEndpointString("bad name", "server")
	assert.Assert(t, err != nil)
	_, err = getEndpointString("gateway", "bad name")
	assert.Assert(t, err != nil)
}

func TestParseAggregatedToolName(t *testing.T) {
	cases := []struct {
		name           string
		rawName        string
		expectedServer string
		expectedTool   string
		namespaced     bool
		namespace      string
		expectedErr    error
	}{
		{
			name:           "valid namespaced tool",
			rawName:        "server___query",
			expectedServer: "server",
			expectedTool:   "query",
			namespaced:     true,
			namespace:      "server",
		},
		{
			name:         "plain tool name",
			rawName:      "query",
			expectedTool: "query",
		},
		{
			name:           "wrong server namespace",
			rawName:        "other___query",
			expectedServer: "server",
			expectedTool:   "query",
			namespaced:     true,
			namespace:      "other",
			expectedErr:    ErrUnexpectedToolNamespace,
		},
		{
			name:         "malformed separator",
			rawName:      "server___nested___query",
			expectedTool: "server___nested___query",
			expectedErr:  ErrMalformedAggregatedToolName,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			parsed, err := ParseAggregatedToolName(testCase.rawName, testCase.expectedServer)
			if testCase.expectedErr != nil {
				assert.Assert(t, errors.Is(err, testCase.expectedErr))
			} else {
				assert.NilError(t, err)
			}
			assert.Equal(t, parsed.ToolName, testCase.expectedTool)
			assert.Equal(t, parsed.IsNamespaced, testCase.namespaced)
			assert.Equal(t, parsed.NamespaceServer, testCase.namespace)
		})
	}
}

func TestExtractAuthToken(t *testing.T) {
	// Given: header values
	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"Bearer token", "token"},
		{"token", "token"},
		{"bearer token", "token"},
	}

	for _, testCase := range cases {
		// When: extracting token
		result := extractAuthToken(testCase.input)

		// Then: token matches expectation
		assert.Equal(t, result, testCase.expected)
	}
}
