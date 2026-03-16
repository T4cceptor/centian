package proxy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

type completionForwardConn struct {
	*MockDownstreamConnection
	result      *mcp.CompleteResult
	err         error
	capturedReq *mcp.CompleteRequest
}

func (c *completionForwardConn) Complete(_ context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	c.capturedReq = req
	return c.result, c.err
}

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

func TestForwardReadResource_ReturnsErrorWhenNoDownstreamOwnsURI(t *testing.T) {
	proxy := &CentianEndpoint{name: "test"}
	conn := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: "file:///other"},
		},
	}
	session := &UpstreamSession{
		id: "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	result, err := proxy.forwardReadResource(context.Background(), session, "server-b", "file:///missing")

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, `resource "file:///missing": no connection to server "server-b" found`)
}

func TestForwardReadResource_NormalizesDownstreamMethodError(t *testing.T) {
	conn := &MockDownstreamConnection{
		serverName:                "server-a",
		Status:                    StatusConnected,
		ReadResourceErrorToReturn: errors.New(`calling "resources/read": method not found`),
	}
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		id: "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	result, err := proxy.forwardReadResource(context.Background(), session, "server-a", "file:///resource")

	assert.Assert(t, result == nil)
	assert.Equal(t, err.Error(), "method not found")
}

func TestForwardSubscribe_ReturnsErrorWhenNoDownstreamOwnsURI(t *testing.T) {
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		id:              "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{},
	}

	err := proxy.forwardSubscribe(context.Background(), session, &mcp.SubscribeRequest{
		Params: &mcp.SubscribeParams{URI: "file:///missing"},
	})

	assert.ErrorContains(t, err, `no downstream connection found for resource URI "file:///missing"`)
}

func TestForwardUnsubscribe_ReturnsErrorWhenNoDownstreamOwnsURI(t *testing.T) {
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		id:              "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{},
	}

	err := proxy.forwardUnsubscribe(context.Background(), session, &mcp.UnsubscribeRequest{
		Params: &mcp.UnsubscribeParams{URI: "file:///missing"},
	})

	assert.ErrorContains(t, err, `no downstream connection found for resource URI "file:///missing"`)
}

func TestForwardSubscribe_NormalizesDownstreamMethodError(t *testing.T) {
	conn := &MockDownstreamConnection{
		serverName:             "server-a",
		Status:                 StatusConnected,
		resources:              []*mcp.Resource{{URI: "file:///resource"}},
		SubscribeErrorToReturn: errors.New(`calling "resources/subscribe": method not found`),
	}
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		id: "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	err := proxy.forwardSubscribe(context.Background(), session, &mcp.SubscribeRequest{
		Params: &mcp.SubscribeParams{URI: "file:///resource"},
	})

	assert.Equal(t, err.Error(), "method not found")
}

func TestForwardUnsubscribe_NormalizesDownstreamMethodError(t *testing.T) {
	conn := &MockDownstreamConnection{
		serverName:               "server-a",
		Status:                   StatusConnected,
		resources:                []*mcp.Resource{{URI: "file:///resource"}},
		UnsubscribeErrorToReturn: errors.New(`calling "resources/unsubscribe": method not found`),
	}
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		id: "session-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	err := proxy.forwardUnsubscribe(context.Background(), session, &mcp.UnsubscribeRequest{
		Params: &mcp.UnsubscribeParams{URI: "file:///resource"},
	})

	assert.Equal(t, err.Error(), "method not found")
}

func TestForwardCompletionUsesConnectedDownstream(t *testing.T) {
	conn := &completionForwardConn{
		MockDownstreamConnection: &MockDownstreamConnection{
			serverName: "server-a",
			Status:     StatusConnected,
		},
		result: &mcp.CompleteResult{
			Completion: mcp.CompletionResultDetails{Values: []string{"one", "two"}},
		},
	}
	proxy := &CentianEndpoint{name: "test"}
	req := &mcp.CompleteRequest{
		Params: &mcp.CompleteParams{
			Argument: mcp.CompleteParamsArgument{Name: "path", Value: "sr"},
		},
	}
	session := &UpstreamSession{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	result, err := proxy.forwardCompletion(context.Background(), session, req)

	assert.NilError(t, err)
	assert.DeepEqual(t, result.Completion.Values, []string{"one", "two"})
	assert.Assert(t, conn.capturedReq == req)
}

func TestForwardCompletionNormalizesDownstreamMethodError(t *testing.T) {
	conn := &completionForwardConn{
		MockDownstreamConnection: &MockDownstreamConnection{
			serverName: "server-a",
			Status:     StatusConnected,
		},
		err: errors.New(`calling "completion/complete": method not found`),
	}
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	result, err := proxy.forwardCompletion(context.Background(), session, &mcp.CompleteRequest{
		Params: &mcp.CompleteParams{},
	})

	assert.Assert(t, result == nil)
	assert.Equal(t, err.Error(), "method not found")
}

func TestForwardCompletionReturnsErrorWithoutConnectedDownstream(t *testing.T) {
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": &MockDownstreamConnection{serverName: "server-a", Status: StatusFailed},
		},
	}

	result, err := proxy.forwardCompletion(context.Background(), session, &mcp.CompleteRequest{
		Params: &mcp.CompleteParams{},
	})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "no downstream connection available")
}

func TestSyncAvailableResourceTemplates_RegistersTemplates(t *testing.T) {
	const templateURI = "file:///items/{id}"

	conn := &MockDownstreamConnection{
		serverName:        "server-a",
		Status:            StatusConnected,
		resourceTemplates: []*mcp.ResourceTemplate{{URITemplate: templateURI, Name: "item-template"}},
	}
	proxy := &CentianEndpoint{name: "test"}
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{
		HasResources: true,
	})
	session := &UpstreamSession{
		id:                          "session-1",
		upstreamServer:              upstreamServer,
		registeredResources:         make(map[string]struct{}),
		registeredResourceTemplates: make(map[string]struct{}),
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	proxy.syncAvailableResourceTemplates(session)

	_, registered := session.registeredResourceTemplates[templateURI]
	assert.Assert(t, registered)
}

func TestRefreshDownstreamResources_SyncsTemplatesAndResources(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	session.downstreamSessionKey = "pool-1"
	conn := &MockDownstreamConnection{
		serverName:        "server-a",
		Status:            StatusConnected,
		resources:         []*mcp.Resource{{URI: "file:///resource"}},
		resourceTemplates: []*mcp.ResourceTemplate{{URITemplate: "file:///resource/{id}", Name: "template"}},
	}
	session.downstreamConns = map[string]DownstreamConnectionInterface{"server-a": conn}
	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
		upstreamSessions: map[string]*UpstreamSession{"session-1": session},
	}

	proxy.refreshDownstreamResources(context.Background(), "pool-1", "server-a", conn, &mcp.ResourceListChangedRequest{
		Params: &mcp.ResourceListChangedParams{},
	})

	_, resourceRegistered := session.registeredResources["file:///resource"]
	_, templateRegistered := session.registeredResourceTemplates["file:///resource/{id}"]
	assert.Assert(t, resourceRegistered)
	assert.Assert(t, templateRegistered)
}

func TestForwardDownstreamResourceUpdated_BroadcastsToSubscribedSessions(t *testing.T) {
	proxy := newLoggingTestProxy()
	sessionA := newLoggingTestSession(proxy, "session-a")
	sessionB := newLoggingTestSession(proxy, "session-b")
	recorderA, clientSessionA, cleanupA := connectResourceClient(t, sessionA)
	defer cleanupA()
	recorderB, clientSessionB, cleanupB := connectResourceClient(t, sessionB)
	defer cleanupB()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions: map[string]*UpstreamSession{
			"session-a": sessionA,
			"session-b": sessionB,
		},
	}

	subscribeResource(t, clientSessionA, "file:///resource")
	subscribeResource(t, clientSessionB, "file:///resource")

	proxy.forwardDownstreamResourceUpdated(context.Background(), "pool-1", "server-a", nil, &mcp.ResourceUpdatedNotificationParams{
		URI: "file:///resource",
	})

	waitForSingleResourceUpdate(t, recorderA)
	waitForSingleResourceUpdate(t, recorderB)
}

func TestForwardDownstreamResourceUpdated_StopsAfterUnsubscribe(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	recorder, clientSession, cleanup := connectResourceClient(t, session)
	defer cleanup()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions:     map[string]*UpstreamSession{"session-1": session},
	}

	subscribeResource(t, clientSession, "file:///resource")
	proxy.forwardDownstreamResourceUpdated(context.Background(), "pool-1", "server-a", nil, &mcp.ResourceUpdatedNotificationParams{
		URI: "file:///resource",
	})
	waitForSingleResourceUpdate(t, recorder)

	assert.NilError(t, clientSession.Unsubscribe(context.Background(), &mcp.UnsubscribeParams{URI: "file:///resource"}))
	time.Sleep(50 * time.Millisecond)
	proxy.forwardDownstreamResourceUpdated(context.Background(), "pool-1", "server-a", nil, &mcp.ResourceUpdatedNotificationParams{
		URI: "file:///resource",
	})
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, len(recorder.snapshot()), 1)
}
