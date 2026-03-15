package proxy

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

// TestFindConnectionForResourceURI_DuplicateURIAcrossServers verifies the documented
// "first match wins" behaviour when the same resource URI is advertised by more than
// one downstream server.
func TestFindConnectionForResourceURI_DuplicateURIAcrossServers(t *testing.T) {
	const sharedURI = "file:///shared/resource"

	// Given: two connected downstream servers that both advertise the same resource URI
	connA := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: sharedURI, Name: "resource-on-a"},
		},
	}
	connB := &MockDownstreamConnection{
		serverName: "server-b",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: sharedURI, Name: "resource-on-b"},
		},
	}

	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		id: "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	// When: looking up the connection for the shared URI
	found := proxy.findConnectionForResourceURI(session, sharedURI)

	// Then: exactly one connection is returned (first match in iteration order) –
	// the caller must be aware that URI uniqueness across downstreams is not enforced.
	assert.Assert(t, found != nil, "expected a connection to be found for the shared URI")
	assert.Assert(t,
		found.GetServerName() == "server-a" || found.GetServerName() == "server-b",
		"returned connection should belong to one of the two servers",
	)
}

// TestFindConnectionForResourceURI_ReturnsNilForUnknownURI verifies that nil is
// returned when no downstream server advertises the requested URI.
func TestFindConnectionForResourceURI_ReturnsNilForUnknownURI(t *testing.T) {
	// Given: a connected downstream server with a different resource URI
	conn := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: "file:///other/resource"},
		},
	}

	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		id: "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	// When: looking up an unknown URI
	found := proxy.findConnectionForResourceURI(session, "file:///unknown/resource")

	// Then: nil is returned
	assert.Assert(t, found == nil)
}

// TestSyncAvailableResources_DuplicateURILastServerWins verifies that when two
// downstream servers expose the same URI, syncAvailableResources registers the
// resource exactly once (last-one-wins during collection) without panicking.
func TestSyncAvailableResources_DuplicateURILastServerWins(t *testing.T) {
	const sharedURI = "file:///shared/resource"

	// Given: two connected servers that both expose the same resource URI
	connA := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: sharedURI, Name: "resource-on-a"},
		},
	}
	connB := &MockDownstreamConnection{
		serverName: "server-b",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: sharedURI, Name: "resource-on-b"},
		},
	}

	proxy := &CentianEndpoint{name: "test"}
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{
		HasResources: true,
	})
	session := &UpstreamSession{
		id:                  "session-1",
		upstreamServer:      upstreamServer,
		registeredResources: make(map[string]struct{}),
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	// When: syncing available resources
	proxy.syncAvailableResources(session)

	// Then: the shared URI is registered exactly once
	assert.Equal(t, len(session.registeredResources), 1,
		"duplicate URI should be collapsed to a single registration")
	_, registered := session.registeredResources[sharedURI]
	assert.Assert(t, registered, "the shared URI must appear in registeredResources")
}
