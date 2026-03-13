package everything

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/proxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	runEverythingIntegrationEnv = "CENTIAN_RUN_EVERYTHING_INTEGRATION"
	everythingServerCmdEnv      = "CENTIAN_EVERYTHING_SERVER_CMD"
	everythingServerArgsEnv     = "CENTIAN_EVERYTHING_SERVER_ARGS"
	defaultSessionTimeout       = 30 * time.Second
	defaultToolsWaitTimeout     = 20 * time.Second
)

type everythingServerCommand struct {
	Command string
	Args    []string
}

type notificationRecorder struct {
	mu sync.Mutex

	logMessages         []*mcp.LoggingMessageParams
	progressMessages    []*mcp.ProgressNotificationParams
	resourceUpdates     []*mcp.ResourceUpdatedNotificationParams
	toolListChanged     int
	resourceListChanged int
	promptListChanged   int
	rootsListChanged    int
	elicitationComplete int
}

type instrumentedSession struct {
	mode     string
	client   *mcp.Client
	session  *mcp.ClientSession
	recorder *notificationRecorder
}

type connectionPair struct {
	Direct  *instrumentedSession
	Proxied *instrumentedSession
}

type everythingHarness struct {
	serverCommand everythingServerCommand
	proxyURL      string
}

func newEverythingHarness(t *testing.T) *everythingHarness {
	t.Helper()

	serverCommand := loadEverythingServerCommand(t)
	proxyURL := startCentianProxyForEverything(t, serverCommand)

	return &everythingHarness{
		serverCommand: serverCommand,
		proxyURL:      proxyURL,
	}
}

func (h *everythingHarness) connectDirect(t *testing.T) *instrumentedSession {
	t.Helper()

	command := exec.Command(h.serverCommand.Command, h.serverCommand.Args...)
	command.Env = os.Environ()

	return connectInstrumentedSession(
		t,
		"direct",
		&mcp.CommandTransport{Command: command},
	)
}

func (h *everythingHarness) connectViaCentian(t *testing.T) *instrumentedSession {
	t.Helper()

	transport := &mcp.StreamableClientTransport{
		Endpoint:   h.proxyURL,
		HTTPClient: &http.Client{Timeout: defaultSessionTimeout},
	}

	return connectInstrumentedSession(t, "proxied", transport)
}

func (h *everythingHarness) connectPair(t *testing.T) *connectionPair {
	t.Helper()

	return &connectionPair{
		Direct:  h.connectDirect(t),
		Proxied: h.connectViaCentian(t),
	}
}

func connectInstrumentedSession(
	t *testing.T,
	mode string,
	transport mcp.Transport,
) *instrumentedSession {
	t.Helper()

	recorder := &notificationRecorder{}
	client := newInstrumentedClient(recorder)

	ctx, cancel := context.WithTimeout(context.Background(), defaultSessionTimeout)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("failed to connect %s session: %v", mode, err)
	}
	t.Cleanup(func() {
		if closeErr := session.Close(); closeErr != nil {
			t.Fatalf("failed to close %s session: %v", mode, closeErr)
		}
	})

	return &instrumentedSession{
		mode:     mode,
		client:   client,
		session:  session,
		recorder: recorder,
	}
}

func newInstrumentedClient(recorder *notificationRecorder) *mcp.Client {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "centian-everything-test-client",
		Version: "1.0.0",
	}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			RootsV2:  &mcp.RootCapabilities{ListChanged: true},
			Sampling: &mcp.SamplingCapabilities{},
			Elicitation: &mcp.ElicitationCapabilities{
				Form: &mcp.FormElicitationCapabilities{},
				URL:  &mcp.URLElicitationCapabilities{},
			},
		},
		CreateMessageHandler: func(context.Context, *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "centian everything harness response"},
				Model:   "test-model",
				Role:    "assistant",
			}, nil
		},
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			recorder.recordLog(req.Params)
		},
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			recorder.recordProgress(req.Params)
		},
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			recorder.recordResourceUpdate(req.Params)
		},
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			recorder.incrementToolListChanged()
		},
		ResourceListChangedHandler: func(context.Context, *mcp.ResourceListChangedRequest) {
			recorder.incrementResourceListChanged()
		},
		PromptListChangedHandler: func(context.Context, *mcp.PromptListChangedRequest) {
			recorder.incrementPromptListChanged()
		},
		ElicitationCompleteHandler: func(context.Context, *mcp.ElicitationCompleteNotificationRequest) {
			recorder.incrementElicitationComplete()
		},
	})

	client.AddRoots(&mcp.Root{
		Name: "centian-workspace",
		URI:  "file:///Users/brb/_devspace/centian-cli",
	})

	return client
}

func (r *notificationRecorder) recordLog(params *mcp.LoggingMessageParams) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logMessages = append(r.logMessages, params)
}

func (r *notificationRecorder) recordProgress(params *mcp.ProgressNotificationParams) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.progressMessages = append(r.progressMessages, params)
}

func (r *notificationRecorder) recordResourceUpdate(params *mcp.ResourceUpdatedNotificationParams) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.resourceUpdates = append(r.resourceUpdates, params)
}

func (r *notificationRecorder) incrementToolListChanged() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.toolListChanged++
}

func (r *notificationRecorder) incrementResourceListChanged() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.resourceListChanged++
}

func (r *notificationRecorder) incrementPromptListChanged() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.promptListChanged++
}

func (r *notificationRecorder) incrementElicitationComplete() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.elicitationComplete++
}

func loadEverythingServerCommand(t *testing.T) everythingServerCommand {
	t.Helper()

	if os.Getenv(runEverythingIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run everything integration tests", runEverythingIntegrationEnv)
	}

	command := strings.TrimSpace(os.Getenv(everythingServerCmdEnv))
	if command == "" {
		t.Skipf("set %s to the everything server command", everythingServerCmdEnv)
	}

	args := strings.Fields(os.Getenv(everythingServerArgsEnv))
	return everythingServerCommand{
		Command: command,
		Args:    args,
	}
}

func startCentianProxyForEverything(t *testing.T, downstream everythingServerCommand) string {
	t.Helper()

	authDisabled := false
	port := allocateFreePort(t)

	globalConfig := &config.GlobalConfig{
		Name:        "Everything Integration Proxy",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Host:    "127.0.0.1",
			Port:    port,
			Timeout: int(defaultSessionTimeout.Seconds()),
		},
		Gateways: map[string]*config.GatewayConfig{
			"everything": {
				MCPServers: map[string]*config.MCPServerConfig{
					"everything": {
						Command: downstream.Command,
						Args:    downstream.Args,
					},
				},
			},
		},
	}

	server, err := proxy.NewCentianProxy(globalConfig)
	if err != nil {
		t.Fatalf("failed to create centian proxy: %v", err)
	}
	if err := server.Setup(); err != nil {
		t.Fatalf("failed to setup centian proxy: %v", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		if err := server.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	waitForProxyListener(t, server.Server.Addr)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("failed to shutdown centian proxy: %v", err)
		}

		select {
		case err := <-serveErr:
			if err != nil {
				t.Fatalf("centian proxy server failed: %v", err)
			}
		default:
		}
	})

	return fmt.Sprintf("http://127.0.0.1:%s/mcp/everything/everything", port)
}

func allocateFreePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type: %T", listener.Addr())
	}

	return strconv.Itoa(tcpAddr.Port)
}

func waitForProxyListener(t *testing.T, address string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("proxy listener %s did not become ready in time", address)
}

func waitForTools(
	ctx context.Context,
	session *mcp.ClientSession,
	timeout time.Duration,
	interval time.Duration,
) (*mcp.ListToolsResult, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		toolsResult, err := session.ListTools(ctx, nil)
		if err == nil && len(toolsResult.Tools) > 0 {
			return toolsResult, nil
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(interval)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return &mcp.ListToolsResult{}, nil
}

func runCapabilityComparison(
	t *testing.T,
	name string,
	testFn func(context.Context, *testing.T, *connectionPair),
) {
	t.Helper()

	t.Run(name, func(t *testing.T) {
		// Given: a real everything server and a Centian proxy configured against it.
		harness := newEverythingHarness(t)

		// When: connecting to the server directly and through Centian.
		pair := harness.connectPair(t)

		ctx, cancel := context.WithTimeout(context.Background(), defaultSessionTimeout)
		defer cancel()

		// Then: execute the concrete comparison probe.
		testFn(ctx, t, pair)
	})
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func toolMap(tools []*mcp.Tool) map[string]*mcp.Tool {
	result := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		result[tool.Name] = tool
	}
	return result
}
