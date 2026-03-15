package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

type loggingNotificationRecorder struct {
	mu       sync.Mutex
	messages []*mcp.LoggingMessageParams
}

func (r *loggingNotificationRecorder) record(params *mcp.LoggingMessageParams) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, params)
}

func (r *loggingNotificationRecorder) snapshot() []*mcp.LoggingMessageParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*mcp.LoggingMessageParams(nil), r.messages...)
}

func newLoggingTestProxy() *CentianEndpoint {
	return &CentianEndpoint{
		name:             "gateway",
		endpoint:         "/mcp/gateway",
		server:           &CentianServer{Config: &config.GlobalConfig{Version: "1.0.0"}},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamSessionPool),
	}
}

func newLoggingTestSession(proxy *CentianEndpoint, sessionID string) *UpstreamSession {
	session := &UpstreamSession{
		id:                  sessionID,
		downstreamConns:     make(map[string]DownstreamConnectionInterface),
		registeredTools:     make(map[string]struct{}),
		registeredResources: make(map[string]struct{}),
		registeredPrompts:   make(map[string]struct{}),
	}
	session.upstreamServer = proxy.newUpstreamServer(session)
	proxy.upstreamSessions[sessionID] = session
	return session
}

func connectLoggingClient(
	t *testing.T,
	session *UpstreamSession,
	middleware ...mcp.Middleware,
) (*loggingNotificationRecorder, func()) {
	t.Helper()

	for _, mw := range middleware {
		session.upstreamServer.AddSendingMiddleware(mw)
	}

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := session.upstreamServer.Connect(ctx, serverTransport, nil)
	assert.NilError(t, err)

	recorder := &loggingNotificationRecorder{}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0.0"}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			recorder.record(req.Params)
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	assert.NilError(t, err)
	assert.NilError(t, clientSession.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}))
	// In-memory SDK sessions do not expose transport session IDs, so use the
	// existing empty-ID fallback when resolving the live session in tests.
	session.id = ""

	return recorder, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func waitForSingleLog(t *testing.T, recorder *loggingNotificationRecorder) []*mcp.LoggingMessageParams {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := recorder.snapshot()
		if len(snapshot) == 1 {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}

	snapshot := recorder.snapshot()
	t.Fatalf("expected 1 log message, got %d", len(snapshot))
	return nil
}

func failLoggingNotificationMiddleware(err error) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "notifications/message" {
				return nil, err
			}
			return next(ctx, method, req)
		}
	}
}

func TestForwardDownstreamLog_SingleSession(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	recorder, cleanup := connectLoggingClient(t, session)
	defer cleanup()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions:     map[string]*UpstreamSession{"session-1": session},
	}

	proxy.forwardDownstreamLog(context.Background(), "pool-1", "server-1", nil, &mcp.LoggingMessageParams{
		Level: "info",
		Data:  "hello",
	})

	messages := waitForSingleLog(t, recorder)
	assert.Equal(t, messages[0].Data.(string), "hello")
}

func TestForwardDownstreamLog_BroadcastsToSharedPool(t *testing.T) {
	proxy := newLoggingTestProxy()
	sessionA := newLoggingTestSession(proxy, "session-a")
	sessionB := newLoggingTestSession(proxy, "session-b")
	recorderA, cleanupA := connectLoggingClient(t, sessionA)
	defer cleanupA()
	recorderB, cleanupB := connectLoggingClient(t, sessionB)
	defer cleanupB()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions: map[string]*UpstreamSession{
			"session-a": sessionA,
			"session-b": sessionB,
		},
	}

	proxy.forwardDownstreamLog(context.Background(), "pool-1", "server-1", nil, &mcp.LoggingMessageParams{
		Level: "info",
		Data:  "broadcast",
	})

	assert.Equal(t, waitForSingleLog(t, recorderA)[0].Data.(string), "broadcast")
	assert.Equal(t, waitForSingleLog(t, recorderB)[0].Data.(string), "broadcast")
}

func TestForwardDownstreamLog_SkipsStaleSessions(t *testing.T) {
	proxy := newLoggingTestProxy()
	liveSession := newLoggingTestSession(proxy, "session-live")
	staleSession := newLoggingTestSession(proxy, "session-stale")
	recorder, cleanup := connectLoggingClient(t, liveSession)
	defer cleanup()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions: map[string]*UpstreamSession{
			"session-live":  liveSession,
			"session-stale": staleSession,
		},
	}

	proxy.forwardDownstreamLog(context.Background(), "pool-1", "server-1", nil, &mcp.LoggingMessageParams{
		Level: "info",
		Data:  "live-only",
	})

	messages := waitForSingleLog(t, recorder)
	assert.Equal(t, messages[0].Data.(string), "live-only")
	assert.Assert(t, proxy.currentUpstreamServerSession(staleSession) == nil)
}

func TestForwardDownstreamLog_ContinuesAfterRecipientError(t *testing.T) {
	proxy := newLoggingTestProxy()
	failingSession := newLoggingTestSession(proxy, "session-bad")
	liveSession := newLoggingTestSession(proxy, "session-good")
	_, cleanupBad := connectLoggingClient(t, failingSession, failLoggingNotificationMiddleware(errors.New("send failed")))
	defer cleanupBad()
	recorder, cleanupGood := connectLoggingClient(t, liveSession)
	defer cleanupGood()

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		upstreamSessions: map[string]*UpstreamSession{
			"session-bad":  failingSession,
			"session-good": liveSession,
		},
	}

	proxy.forwardDownstreamLog(context.Background(), "pool-1", "server-1", nil, &mcp.LoggingMessageParams{
		Level: "info",
		Data:  "still-delivered",
	})

	messages := waitForSingleLog(t, recorder)
	assert.Equal(t, messages[0].Data.(string), "still-delivered")
}

func TestSyncUpstreamLoggingLevel_UsesMostVerbosePoolLevel(t *testing.T) {
	proxy := newLoggingTestProxy()
	conn := &MockDownstreamConnection{serverName: "server-1", Status: StatusConnected}
	sessionInfo := newLoggingTestSession(proxy, "session-info")
	sessionError := newLoggingTestSession(proxy, "session-error")
	sessionInfo.downstreamSessionKey = "pool-1"
	sessionError.downstreamSessionKey = "pool-1"
	sessionError.logLevel = "error"

	proxy.downstreamPools["pool-1"] = &DownstreamSessionPool{
		downstreamSessionKey: "pool-1",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-1": conn,
		},
		upstreamSessions: map[string]*UpstreamSession{
			"session-info":  sessionInfo,
			"session-error": sessionError,
		},
	}

	proxy.syncUpstreamLoggingLevel("session-info", "info")

	conn.mu.RLock()
	defer conn.mu.RUnlock()
	assert.DeepEqual(t, conn.CapturedLogLevels, []mcp.LoggingLevel{"info"})
}

func TestProxyHTTP_SetLoggingLevelPropagatesToDownstreamPool(t *testing.T) {
	mockConn := &MockDownstreamConnection{
		serverName: "server-1",
		Status:     StatusConnected,
		tools: []*mcp.Tool{
			{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
		},
	}
	proxy := &CentianEndpoint{
		name:             "server-1",
		endpoint:         "/mcp/server-1",
		config:           &config.GatewayConfig{MCPServers: map[string]*config.MCPServerConfig{"server-1": {Command: "node"}}},
		server:           &CentianServer{Config: &config.GlobalConfig{Version: "1.0.0"}},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamSessionPool),
		connectionFactory: func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
			return mockConn
		},
	}

	mux := http.NewServeMux()
	RegisterEndpoint(proxy, mux, nil)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL + "/mcp/server-1",
	}, nil)
	assert.NilError(t, err)
	defer func() { _ = session.Close() }()

	assert.NilError(t, session.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "info"}))
	waitForCondition(t, time.Second, func() bool {
		mockConn.mu.RLock()
		defer mockConn.mu.RUnlock()
		return len(mockConn.CapturedLogLevels) == 1 && mockConn.CapturedLogLevels[0] == "info"
	})
}
