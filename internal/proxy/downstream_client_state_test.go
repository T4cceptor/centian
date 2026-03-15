package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestFingerprintClientCapabilitiesDeterministic(t *testing.T) {
	capabilitiesA := &mcp.ClientCapabilities{
		Experimental: map[string]any{"b": "two", "a": "one"},
		RootsV2:      &mcp.RootCapabilities{ListChanged: true},
	}
	capabilitiesB := &mcp.ClientCapabilities{
		Experimental: map[string]any{"a": "one", "b": "two"},
		RootsV2:      &mcp.RootCapabilities{ListChanged: true},
	}

	assert.Equal(t, fingerprintClientCapabilities(capabilitiesA), fingerprintClientCapabilities(capabilitiesB))
}

func TestCloneClientCapabilitiesPreservesRootsV2(t *testing.T) {
	capabilities := &mcp.ClientCapabilities{
		RootsV2: &mcp.RootCapabilities{ListChanged: true},
	}

	cloned := cloneClientCapabilities(capabilities)

	assert.Assert(t, cloned != nil)
	assert.Assert(t, cloned.RootsV2 != nil)
	assert.Assert(t, cloned.RootsV2 != capabilities.RootsV2)
	assert.Equal(t, cloned.RootsV2.ListChanged, true)
}

func TestFingerprintRootsDeterministic(t *testing.T) {
	rootsA := []*mcp.Root{
		{Name: "b", URI: "file://b"},
		{Name: "a", URI: "file://a"},
	}
	rootsB := []*mcp.Root{
		{Name: "a", URI: "file://a"},
		{Name: "b", URI: "file://b"},
	}

	assert.Equal(t, fingerprintRoots(rootsA), fingerprintRoots(rootsB))
}

func TestBuildDownstreamSessionKeyIncludesCapabilitiesAndRoots(t *testing.T) {
	proxy := &CentianEndpoint{endpoint: "/mcp/gateway"}

	stateA := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, nil)
	stateB := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{
		Sampling: &mcp.SamplingCapabilities{},
	}, nil)
	stateC := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{
		{Name: "root", URI: "file://root"},
	})

	keyA := proxy.buildDownstreamSessionKey("identity", map[string]string{"Authorization": "one"}, stateA)
	keyB := proxy.buildDownstreamSessionKey("identity", map[string]string{"Authorization": "one"}, stateB)
	keyC := proxy.buildDownstreamSessionKey("identity", map[string]string{"Authorization": "one"}, stateC)

	assert.Assert(t, keyA != keyB)
	assert.Assert(t, keyA != keyC)
}

func TestInspectMCPRequestRestoresBody(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", strings.NewReader(body))

	observation := inspectMCPRequest(request)

	assert.Assert(t, observation.hasMethod("initialize"))
	assert.Equal(t, observation.sessionID, "")

	restoredBody, err := io.ReadAll(request.Body)
	assert.NilError(t, err)
	assert.Equal(t, string(restoredBody), body)
}
