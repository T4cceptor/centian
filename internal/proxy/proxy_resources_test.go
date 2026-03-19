package proxy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
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

func captureInternalLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var console bytes.Buffer
	assert.NilError(t, common.InitInternalLogger(common.LoggerOptions{
		Level:         "info",
		Output:        "console",
		ConsoleWriter: &console,
	}))
	t.Cleanup(func() {
		assert.NilError(t, common.CloseLogger())
	})

	return &console
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

func TestSyncAvailableResources_AggregatedDuplicateURIIsOmitted(t *testing.T) {
	const sharedURI = "file:///shared/resource"
	const uniqueURI = "file:///unique/resource"
	logs := captureInternalLogs(t)

	connA := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: sharedURI, Name: "resource-on-a"},
			{URI: uniqueURI, Name: "resource-on-a-only"},
		},
	}
	connB := &MockDownstreamConnection{
		serverName: "server-b",
		Status:     StatusConnected,
		resources: []*mcp.Resource{
			{URI: sharedURI, Name: "resource-on-b"},
		},
	}

	proxy := &CentianEndpoint{
		name:              "test",
		isAggregatedProxy: true,
		downstreamPools:   make(map[string]*DownstreamSessionPool),
	}
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{
		HasResources: true,
	})
	pool := &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		resourceCollisions:   make(map[string][]string),
	}
	proxy.downstreamPools["pool-1"] = pool
	session := &UpstreamSession{
		id:                   "session-1",
		upstreamServer:       upstreamServer,
		registeredResources:  make(map[string]struct{}),
		downstreamSessionKey: "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	proxy.syncAvailableResources(session)

	assert.Equal(t, len(session.registeredResources), 1,
		"only the non-colliding resource should be registered")
	_, sharedRegistered := session.registeredResources[sharedURI]
	assert.Assert(t, !sharedRegistered, "the colliding URI must be omitted")
	_, uniqueRegistered := session.registeredResources[uniqueURI]
	assert.Assert(t, uniqueRegistered, "the unique URI should remain available")
	assert.DeepEqual(t, pool.resourceCollisions[sharedURI], []string{"server-a", "server-b"})
	assert.Assert(t, strings.Contains(logs.String(), `aggregated resource URI "`+sharedURI+`" collides across downstreams [server-a, server-b]`))
}

func TestSyncAvailableResources_NonAggregatedDuplicateURIRemainsRegistered(t *testing.T) {
	const sharedURI = "file:///shared/resource"

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

	proxy.syncAvailableResources(session)

	assert.Equal(t, len(session.registeredResources), 1)
	_, registered := session.registeredResources[sharedURI]
	assert.Assert(t, registered)
}

func TestSyncAvailableResources_RegistersSharedURIWhenCollisionResolves(t *testing.T) {
	const sharedURI = "file:///shared/resource"
	logs := captureInternalLogs(t)

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

	proxy := &CentianEndpoint{
		name:              "test",
		isAggregatedProxy: true,
		downstreamPools:   make(map[string]*DownstreamSessionPool),
	}
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{
		HasResources: true,
	})
	pool := &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		resourceCollisions:   make(map[string][]string),
	}
	proxy.downstreamPools["pool-1"] = pool
	session := &UpstreamSession{
		id:                   "session-1",
		upstreamServer:       upstreamServer,
		registeredResources:  make(map[string]struct{}),
		downstreamSessionKey: "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	proxy.syncAvailableResources(session)
	_, registeredWhileColliding := session.registeredResources[sharedURI]
	assert.Assert(t, !registeredWhileColliding)

	connB.resources = nil
	proxy.syncAvailableResources(session)

	_, registeredAfterResolution := session.registeredResources[sharedURI]
	assert.Assert(t, registeredAfterResolution, "the surviving resource should be re-registered after the collision resolves")
	_, stillColliding := pool.resourceCollisions[sharedURI]
	assert.Assert(t, !stillColliding, "collision tracking should clear once only one downstream remains")
	assert.Assert(t, strings.Contains(logs.String(), `aggregated resource URI "`+sharedURI+`" collision resolved`))
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

func TestForwardSubscribe_ReturnsCollisionErrorForHiddenURI(t *testing.T) {
	const sharedURI = "file:///shared/resource"

	connA := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resources:  []*mcp.Resource{{URI: sharedURI}},
	}
	connB := &MockDownstreamConnection{
		serverName: "server-b",
		Status:     StatusConnected,
		resources:  []*mcp.Resource{{URI: sharedURI}},
	}
	proxy := &CentianEndpoint{
		name:              "test",
		isAggregatedProxy: true,
		downstreamPools: map[string]*DownstreamSessionPool{
			"pool-1": {
				downstreamSessionKey: "pool-1",
				resourceCollisions: map[string][]string{
					sharedURI: {"server-a", "server-b"},
				},
			},
		},
	}
	session := &UpstreamSession{
		id:                   "session-1",
		downstreamSessionKey: "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	err := proxy.forwardSubscribe(context.Background(), session, &mcp.SubscribeRequest{
		Params: &mcp.SubscribeParams{URI: sharedURI},
	})

	assert.ErrorContains(t, err, `resource URI "file:///shared/resource" is hidden because multiple downstreams expose it: server-a, server-b`)
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

func TestForwardUnsubscribe_ReturnsCollisionErrorForHiddenURI(t *testing.T) {
	const sharedURI = "file:///shared/resource"

	connA := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resources:  []*mcp.Resource{{URI: sharedURI}},
	}
	connB := &MockDownstreamConnection{
		serverName: "server-b",
		Status:     StatusConnected,
		resources:  []*mcp.Resource{{URI: sharedURI}},
	}
	proxy := &CentianEndpoint{
		name:              "test",
		isAggregatedProxy: true,
		downstreamPools: map[string]*DownstreamSessionPool{
			"pool-1": {
				downstreamSessionKey: "pool-1",
				resourceCollisions: map[string][]string{
					sharedURI: {"server-a", "server-b"},
				},
			},
		},
	}
	session := &UpstreamSession{
		id:                   "session-1",
		downstreamSessionKey: "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	err := proxy.forwardUnsubscribe(context.Background(), session, &mcp.UnsubscribeRequest{
		Params: &mcp.UnsubscribeParams{URI: sharedURI},
	})

	assert.ErrorContains(t, err, `resource URI "file:///shared/resource" is hidden because multiple downstreams expose it: server-a, server-b`)
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

func TestSyncAvailableResourceTemplates_AggregatedDuplicateTemplateIsOmitted(t *testing.T) {
	const sharedTemplateURI = "file:///items/{id}"
	const uniqueTemplateURI = "file:///other/{id}"
	logs := captureInternalLogs(t)

	connA := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		resourceTemplates: []*mcp.ResourceTemplate{
			{URITemplate: sharedTemplateURI, Name: "shared-on-a"},
			{URITemplate: uniqueTemplateURI, Name: "unique-on-a"},
		},
	}
	connB := &MockDownstreamConnection{
		serverName: "server-b",
		Status:     StatusConnected,
		resourceTemplates: []*mcp.ResourceTemplate{
			{URITemplate: sharedTemplateURI, Name: "shared-on-b"},
		},
	}
	proxy := &CentianEndpoint{
		name:              "test",
		isAggregatedProxy: true,
		downstreamPools:   make(map[string]*DownstreamSessionPool),
	}
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{
		HasResources: true,
	})
	pool := &DownstreamSessionPool{
		downstreamSessionKey:       "pool-1",
		resourceTemplateCollisions: make(map[string][]string),
	}
	proxy.downstreamPools["pool-1"] = pool
	session := &UpstreamSession{
		id:                          "session-1",
		upstreamServer:              upstreamServer,
		registeredResources:         make(map[string]struct{}),
		registeredResourceTemplates: make(map[string]struct{}),
		downstreamSessionKey:        "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	proxy.syncAvailableResourceTemplates(session)

	_, sharedRegistered := session.registeredResourceTemplates[sharedTemplateURI]
	assert.Assert(t, !sharedRegistered, "colliding templates should be omitted")
	_, uniqueRegistered := session.registeredResourceTemplates[uniqueTemplateURI]
	assert.Assert(t, uniqueRegistered, "unique templates should remain available")
	assert.DeepEqual(t, pool.resourceTemplateCollisions[sharedTemplateURI], []string{"server-a", "server-b"})
	assert.Assert(t, strings.Contains(logs.String(), `aggregated resource template "`+sharedTemplateURI+`" collides across downstreams [server-a, server-b]`))
}

func TestSyncAvailableResourceTemplates_RegistersTemplateWhenCollisionResolves(t *testing.T) {
	const sharedTemplateURI = "file:///items/{id}"
	logs := captureInternalLogs(t)

	connA := &MockDownstreamConnection{
		serverName:        "server-a",
		Status:            StatusConnected,
		resourceTemplates: []*mcp.ResourceTemplate{{URITemplate: sharedTemplateURI, Name: "shared-on-a"}},
	}
	connB := &MockDownstreamConnection{
		serverName:        "server-b",
		Status:            StatusConnected,
		resourceTemplates: []*mcp.ResourceTemplate{{URITemplate: sharedTemplateURI, Name: "shared-on-b"}},
	}
	proxy := &CentianEndpoint{
		name:              "test",
		isAggregatedProxy: true,
		downstreamPools:   make(map[string]*DownstreamSessionPool),
	}
	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ServerOptions{
		HasResources: true,
	})
	pool := &DownstreamSessionPool{
		downstreamSessionKey:       "pool-1",
		resourceTemplateCollisions: make(map[string][]string),
	}
	proxy.downstreamPools["pool-1"] = pool
	session := &UpstreamSession{
		id:                          "session-1",
		upstreamServer:              upstreamServer,
		registeredResources:         make(map[string]struct{}),
		registeredResourceTemplates: make(map[string]struct{}),
		downstreamSessionKey:        "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": connA,
			"server-b": connB,
		},
	}

	proxy.syncAvailableResourceTemplates(session)
	_, registeredWhileColliding := session.registeredResourceTemplates[sharedTemplateURI]
	assert.Assert(t, !registeredWhileColliding)

	connB.resourceTemplates = nil
	proxy.syncAvailableResourceTemplates(session)

	_, registeredAfterResolution := session.registeredResourceTemplates[sharedTemplateURI]
	assert.Assert(t, registeredAfterResolution, "the surviving template should be re-registered after the collision resolves")
	_, stillColliding := pool.resourceTemplateCollisions[sharedTemplateURI]
	assert.Assert(t, !stillColliding, "template collision tracking should clear once only one downstream remains")
	assert.Assert(t, strings.Contains(logs.String(), `aggregated resource template "`+sharedTemplateURI+`" collision resolved`))
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

	subscribeResource(t, clientSessionA)
	subscribeResource(t, clientSessionB)

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

	subscribeResource(t, clientSession)
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
