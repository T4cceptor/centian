package everything

import (
	"context"
	"encoding/json"
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
	defaultDirectTerminateWait  = 250 * time.Millisecond
	defaultToolsWaitTimeout     = 20 * time.Second
	defaultShutdownTimeout      = 10 * time.Second
	defaultEverythingServerCmd  = "npx"
	defaultEverythingServerArgs = "-y @modelcontextprotocol/server-everything"
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

type notificationSnapshot struct {
	LogCount               int
	ProgressCount          int
	ResourceUpdateCount    int
	ResourceUpdateURIs     []string
	ToolListChangedCount   int
	ResourceListChanged    int
	PromptListChangedCount int
	RootsListChangedCount  int
	ElicitationComplete    int
}

type phase3Classification string

const (
	classificationMatch                phase3Classification = "match"
	classificationProxyDivergence      phase3Classification = "proxy_divergence"
	classificationUnsupportedInCentian phase3Classification = "unsupported_in_centian"
)

type phase3Outcome struct {
	Name           string
	Classification phase3Classification
	Summary        string
	DirectDetails  string
	ProxiedDetails string
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
		&mcp.CommandTransport{
			Command:           command,
			TerminateDuration: defaultDirectTerminateWait,
		},
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
			if mode == "direct" && isExpectedDirectCloseError(closeErr) {
				return
			}
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

func isExpectedDirectCloseError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
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

func (r *notificationRecorder) snapshot() notificationSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	resourceUpdateURIs := make([]string, 0, len(r.resourceUpdates))
	for _, update := range r.resourceUpdates {
		if update == nil {
			continue
		}
		resourceUpdateURIs = append(resourceUpdateURIs, update.URI)
	}

	return notificationSnapshot{
		LogCount:               len(r.logMessages),
		ProgressCount:          len(r.progressMessages),
		ResourceUpdateCount:    len(r.resourceUpdates),
		ResourceUpdateURIs:     resourceUpdateURIs,
		ToolListChangedCount:   r.toolListChanged,
		ResourceListChanged:    r.resourceListChanged,
		PromptListChangedCount: r.promptListChanged,
		RootsListChangedCount:  r.rootsListChanged,
		ElicitationComplete:    r.elicitationComplete,
	}
}

func loadEverythingServerCommand(t *testing.T) everythingServerCommand {
	t.Helper()

	if os.Getenv(runEverythingIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run everything integration tests", runEverythingIntegrationEnv)
	}

	command := strings.TrimSpace(os.Getenv(everythingServerCmdEnv))
	if command == "" {
		command = defaultEverythingServerCmd
	}

	argsEnv := strings.TrimSpace(os.Getenv(everythingServerArgsEnv))
	if argsEnv == "" {
		argsEnv = defaultEverythingServerArgs
	}

	args := strings.Fields(argsEnv)
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
			Host:            "127.0.0.1",
			Port:            port,
			Timeout:         int(defaultSessionTimeout.Seconds()),
			EnableTestTools: false,
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

	server, err := proxy.NewCentianServer(globalConfig)
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
		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
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

func mustFindTool(t *testing.T, tools []*mcp.Tool, name string) {
	t.Helper()

	for _, tool := range tools {
		if tool.Name == name {
			return
		}
	}

	t.Fatalf("expected tool %q to exist", name)
}

func mustCallTool(
	t *testing.T,
	ctx context.Context,
	session *mcp.ClientSession,
	name string,
	args map[string]any,
) *mcp.CallToolResult {
	t.Helper()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("failed to call tool %q: %v", name, err)
	}
	return result
}

func mustCallToolWithParams(
	t *testing.T,
	ctx context.Context,
	session *mcp.ClientSession,
	params *mcp.CallToolParams,
) *mcp.CallToolResult {
	t.Helper()

	result, err := session.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("failed to call tool %q: %v", params.Name, err)
	}
	return result
}

func prettyJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal json: %v", err)
	}

	return string(data)
}

func reportPhase3Outcome(t *testing.T, outcome *phase3Outcome) {
	t.Helper()

	if outcome.Classification == classificationMatch {
		t.Logf("%s: %s", outcome.Classification, outcome.Summary)
		return
	}

	t.Fatalf(
		"phase 3 probe %q classified as %s\nsummary: %s\ndirect:\n%s\nproxied:\n%s",
		outcome.Name,
		outcome.Classification,
		outcome.Summary,
		outcome.DirectDetails,
		outcome.ProxiedDetails,
	)
}
