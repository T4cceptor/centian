package proxy

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

type resourceUpdateRecorder struct {
	mu      sync.Mutex
	updates []*mcp.ResourceUpdatedNotificationParams
}

func (r *resourceUpdateRecorder) record(params *mcp.ResourceUpdatedNotificationParams) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, params)
}

func (r *resourceUpdateRecorder) snapshot() []*mcp.ResourceUpdatedNotificationParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*mcp.ResourceUpdatedNotificationParams(nil), r.updates...)
}

func connectResourceClient(t *testing.T, session *UpstreamSession) (*resourceUpdateRecorder, *mcp.ClientSession, func()) {
	t.Helper()

	session.upstreamServer = mcp.NewServer(&mcp.Implementation{Name: "server", Version: "1.0.0"}, &mcp.ServerOptions{
		HasResources: true,
		SubscribeHandler: func(context.Context, *mcp.SubscribeRequest) error {
			return nil
		},
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error {
			return nil
		},
		GetSessionID: func() string {
			return session.id
		},
	})
	session.upstreamServer.AddResource(&mcp.Resource{URI: "file:///resource", Name: "resource"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{}, nil
	})

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := session.upstreamServer.Connect(ctx, serverTransport, nil)
	assert.NilError(t, err)

	recorder := &resourceUpdateRecorder{}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0.0"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			recorder.record(req.Params)
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	assert.NilError(t, err)
	session.id = ""

	return recorder, clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func subscribeResource(t *testing.T, clientSession *mcp.ClientSession) {
	t.Helper()
	assert.NilError(t, clientSession.Subscribe(context.Background(), &mcp.SubscribeParams{URI: "file:///resource"}))
}

func waitForSingleResourceUpdate(t *testing.T, recorder *resourceUpdateRecorder) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := recorder.snapshot()
		if len(snapshot) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	snapshot := recorder.snapshot()
	t.Fatalf("expected 1 resource update, got %d", len(snapshot))
}

func TestNewPoolResourceUpdatedHandler(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	recorder, clientSession, cleanup := connectResourceClient(t, session)
	defer cleanup()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions:     map[string]*UpstreamSession{"session-1": session},
	}

	subscribeResource(t, clientSession)

	handler := proxy.newPoolResourceUpdatedHandler("pool-1", "server-a", nil)
	handler(context.Background(), nil)
	handler(context.Background(), &mcp.ResourceUpdatedNotificationRequest{})
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, len(recorder.snapshot()), 0)

	handler(context.Background(), &mcp.ResourceUpdatedNotificationRequest{
		Params: &mcp.ResourceUpdatedNotificationParams{URI: "file:///resource"},
	})
	waitForSingleResourceUpdate(t, recorder)
}

func TestForwardDownstreamResourceUpdated_SuppressesCollidedResourceURI(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	recorder, clientSession, cleanup := connectResourceClient(t, session)
	defer cleanup()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions:     map[string]*UpstreamSession{"session-1": session},
		resourceCollisions:   map[string][]string{"file:///resource": {"server-a", "server-b"}},
	}

	subscribeResource(t, clientSession)

	proxy.forwardDownstreamResourceUpdated(context.Background(), "pool-1", "server-a", nil, &mcp.ResourceUpdatedNotificationParams{
		URI: "file:///resource",
	})
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, len(recorder.snapshot()), 0)
}
