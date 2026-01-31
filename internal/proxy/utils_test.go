package proxy

import (
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestGetNewUUIDV7(t *testing.T) {
	// Given: a UUID generator
	// When: generating a new ID
	id := getNewUUIDV7()

	// Then: the ID is non-empty
	assert.Assert(t, id != "")
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
