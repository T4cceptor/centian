package proxy

import (
	"net/http"
	"testing"

	"github.com/T4cceptor/centian/internal/identifiers"
	"gotest.tools/assert"
)

func TestGetNewUUIDV7(t *testing.T) {
	id := getNewUUIDV7()

	assert.Assert(t, identifiers.IsKind(id, identifiers.KindRequest))
}

func TestGetServerID(t *testing.T) {
	serverID := getServerID()

	assert.Assert(t, identifiers.IsKind(serverID, identifiers.KindServer))
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
		wantErr        bool
	}{
		{
			name:           "valid namespaced tool",
			rawName:        "server___query",
			expectedServer: "server",
			expectedTool:   "query",
		},
		{
			name:           "wrong server namespace",
			rawName:        "other___query",
			expectedServer: "server",
			wantErr:        true,
		},
		{
			name:    "malformed separator",
			rawName: "server___nested___query",
			wantErr: true,
		},
		{
			name:    "missing namespace",
			rawName: "query",
			wantErr: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			toolName, err := parseAggregatedToolName(testCase.rawName, testCase.expectedServer)
			if testCase.wantErr {
				assert.Assert(t, err != nil)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, toolName, testCase.expectedTool)
		})
	}
}

func TestGetAuthHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer upstream"},
		"X-API-Key":     []string{"api-key"},
		"X-Auth-Token":  []string{"auth-token"},
		"X-Other":       []string{"ignored"},
	}

	authHeaders := getAuthHeaders(headers, "X-API-Key")

	assert.Equal(t, authHeaders["Authorization"], "Bearer upstream")
	assert.Equal(t, authHeaders["X-Auth-Token"], "auth-token")
	assert.Assert(t, authHeaders["X-API-Key"] == "")
	assert.Assert(t, authHeaders["X-Other"] == "")
}

func TestRedactHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization":  []string{"Bearer upstream"},
		"X-Centian-Auth": []string{"centian-secret"},
		"X-Api-Key":      []string{"api-key"},
		"X-Auth-Token":   []string{"auth-token"},
		"X-Other":        []string{"visible"},
	}

	redacted := redactHeaders(headers)

	assert.DeepEqual(t, redacted["Authorization"], []string{"<redacted>"})
	assert.DeepEqual(t, redacted["X-Centian-Auth"], []string{"<redacted>"})
	assert.DeepEqual(t, redacted["X-Api-Key"], []string{"<redacted>"})
	assert.DeepEqual(t, redacted["X-Auth-Token"], []string{"<redacted>"})
	assert.DeepEqual(t, redacted["X-Other"], []string{"visible"})
	assert.DeepEqual(t, headers["Authorization"], []string{"Bearer upstream"})
}

func TestHeaderNameInList(t *testing.T) {
	assert.Assert(t, headerNameInList("Authorization", redactedAuthHeaders))
	assert.Assert(t, headerNameInList("authorization", redactedAuthHeaders))
	assert.Assert(t, !headerNameInList("X-Other", redactedAuthHeaders))
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
