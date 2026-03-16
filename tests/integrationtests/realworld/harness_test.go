package realworld

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/proxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	runRealworldIntegrationEnv = "CENTIAN_RUN_REALWORLD_INTEGRATION"
	defaultSessionTimeout      = 30 * time.Second
	defaultToolsWaitTimeout    = 20 * time.Second
)

type serverMode string

const (
	modeDirect  serverMode = "direct"
	modeProxied serverMode = "proxied"
)

type serverCommand struct {
	Command string
	Args    []string
	Env     map[string]string
}

type warmupCommand struct {
	Command           string
	Args              []string
	IncludeRuntimeEnv bool
	Timeout           time.Duration
}

type bootstrapConfig struct {
	Warmup             *warmupCommand
	RetryDirectStartup bool
}

type modeFixture struct {
	RootDir       string
	Roots         []*mcp.Root
	Env           map[string]string
	Normalization map[string]string
}

type fixtureBundle struct {
	Direct   modeFixture
	Proxied  modeFixture
	Shared   map[string]string
	Expected map[string]string
}

type resultNormalizer func(serverMode, any, *fixtureBundle) any

type serverManifest struct {
	Name           string
	GatewayID      string
	ServerID       string
	CommandEnvVar  string
	ArgsEnvVar     string
	DefaultCommand string
	DefaultArgs    []string
	ExpectedTools  []string
	BuildFixture   func(*testing.T) *fixtureBundle
	Normalize      resultNormalizer
	Bootstrap      *bootstrapConfig
}

type instrumentedSession struct {
	mode    serverMode
	client  *mcp.Client
	session *mcp.ClientSession
}

type connectionPair struct {
	Direct  *instrumentedSession
	Proxied *instrumentedSession
}

type realworldHarness struct {
	manifest       *serverManifest
	fixture        *fixtureBundle
	directCommand  serverCommand
	proxiedCommand serverCommand
	proxyURL       string
	warmupRan      bool
}

func newRealworldHarness(t *testing.T, manifest *serverManifest) *realworldHarness {
	t.Helper()

	requireRealworldEnabled(t)

	fixture := manifest.BuildFixture(t)
	directCommand := loadServerCommand(t, manifest, modeDirect, fixture)
	proxiedCommand := loadServerCommand(t, manifest, modeProxied, fixture)

	warmupRan := false
	if err := runManifestWarmup(t, manifest, directCommand); err != nil {
		t.Fatalf("failed to warm up %s server bootstrap: %v", manifest.Name, err)
	}
	if manifest.Bootstrap != nil && manifest.Bootstrap.Warmup != nil {
		warmupRan = true
	}

	proxyURL := startCentianProxyForRealworld(t, manifest, proxiedCommand)

	return &realworldHarness{
		manifest:       manifest,
		fixture:        fixture,
		directCommand:  directCommand,
		proxiedCommand: proxiedCommand,
		proxyURL:       proxyURL,
		warmupRan:      warmupRan,
	}
}

func requireRealworldEnabled(t *testing.T) {
	t.Helper()

	if os.Getenv(runRealworldIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run real-world integration tests", runRealworldIntegrationEnv)
	}
}

func loadServerCommand(t *testing.T, manifest *serverManifest, mode serverMode, fixture *fixtureBundle) serverCommand {
	t.Helper()

	command := strings.TrimSpace(os.Getenv(manifest.CommandEnvVar))
	if command == "" {
		command = manifest.DefaultCommand
	}

	argsEnv := strings.TrimSpace(os.Getenv(manifest.ArgsEnvVar))
	args := append([]string(nil), manifest.DefaultArgs...)
	if argsEnv != "" {
		args = strings.Fields(argsEnv)
	}

	if _, err := exec.LookPath(command); err != nil {
		t.Skipf("%s command %q is not available: %v", manifest.Name, command, err)
	}

	return serverCommand{
		Command: command,
		Args:    args,
		Env:     fixtureForMode(fixture, mode).Env,
	}
}

func (h *realworldHarness) connectPair(t *testing.T) *connectionPair {
	t.Helper()

	return &connectionPair{
		Direct:  h.connectDirect(t),
		Proxied: h.connectViaCentian(t),
	}
}

func (h *realworldHarness) connectDirect(t *testing.T) *instrumentedSession {
	t.Helper()

	session, state, err := connectWithBootstrapRetry(
		func() (*instrumentedSession, string, error) {
			return connectDirectSession(t, h.directCommand, fixtureForMode(h.fixture, modeDirect).Roots)
		},
		func(session *instrumentedSession) {
			if session == nil || session.session == nil {
				return
			}
			_ = session.session.Close()
		},
		func(session *instrumentedSession) error {
			return probeSessionReady(session)
		},
		func() error {
			return runManifestWarmup(t, h.manifest, h.directCommand)
		},
		h.manifest.Bootstrap != nil && h.manifest.Bootstrap.RetryDirectStartup,
	)
	if err != nil {
		t.Fatalf(
			"failed to connect %s session: %v\ncommand: %q\nargs: %v\nwarmup ran: %t\nretry attempt: %t\nstderr: %s",
			modeDirect,
			err,
			h.directCommand.Command,
			h.directCommand.Args,
			h.warmupRan || state.RetryWarmupRan,
			state.Retried,
			state.Stderr,
		)
	}

	registerSessionCleanup(t, session)
	return session
}

func (h *realworldHarness) connectViaCentian(t *testing.T) *instrumentedSession {
	t.Helper()

	session, err := connectInstrumentedSession(
		t,
		modeProxied,
		fixtureForMode(h.fixture, modeProxied).Roots,
		&mcp.StreamableClientTransport{
			Endpoint:   h.proxyURL,
			HTTPClient: &http.Client{Timeout: defaultSessionTimeout},
		},
	)
	if err != nil {
		t.Fatalf("failed to connect %s session: %v", modeProxied, err)
	}
	registerSessionCleanup(t, session)
	return session
}

func connectInstrumentedSession(
	t *testing.T,
	mode serverMode,
	roots []*mcp.Root,
	transport mcp.Transport,
) (*instrumentedSession, error) {
	t.Helper()

	client := newClientWithRoots(roots)
	ctx, cancel := context.WithTimeout(context.Background(), defaultSessionTimeout)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}

	return &instrumentedSession{
		mode:    mode,
		client:  client,
		session: session,
	}, nil
}

type bootstrapRetryState struct {
	Retried        bool
	RetryWarmupRan bool
	Stderr         string
}

func connectDirectSession(t *testing.T, command serverCommand, roots []*mcp.Root) (*instrumentedSession, string, error) {
	t.Helper()

	cmd := exec.Command(command.Command, command.Args...)
	cmd.Env = mergeEnv(os.Environ(), command.Env)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	session, err := connectInstrumentedSession(
		t,
		modeDirect,
		roots,
		&mcp.CommandTransport{Command: cmd},
	)
	return session, strings.TrimSpace(stderr.String()), err
}

func registerSessionCleanup(t *testing.T, session *instrumentedSession) {
	t.Helper()

	t.Cleanup(func() {
		if closeErr := session.session.Close(); closeErr != nil {
			t.Fatalf("failed to close %s session: %v", session.mode, closeErr)
		}
	})
}

func probeSessionReady(session *instrumentedSession) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultToolsWaitTimeout)
	defer cancel()

	toolsResult, err := waitForTools(ctx, session.session)
	if err != nil {
		return err
	}
	if len(toolsResult.Tools) == 0 {
		return errors.New("no tools discovered before readiness probe timeout")
	}
	return nil
}

func connectWithBootstrapRetry(
	connect func() (*instrumentedSession, string, error),
	closeSession func(*instrumentedSession),
	verify func(*instrumentedSession) error,
	runWarmup func() error,
	retryEnabled bool,
) (*instrumentedSession, bootstrapRetryState, error) {
	session, stderr, err := connect()
	state := bootstrapRetryState{Stderr: stderr}
	if err == nil && verify != nil {
		err = verify(session)
	}
	if err == nil {
		return session, state, nil
	}

	if session != nil {
		closeSession(session)
	}
	if !retryEnabled || !isRetryableBootstrapError(err) {
		return nil, state, err
	}

	state.Retried = true
	if runWarmup != nil {
		if warmupErr := runWarmup(); warmupErr != nil {
			state.RetryWarmupRan = true
			return nil, state, fmt.Errorf("retry warmup failed: %w", warmupErr)
		}
		state.RetryWarmupRan = true
	}

	session, retryStderr, err := connect()
	if retryStderr != "" {
		state.Stderr = retryStderr
	}
	if err == nil && verify != nil {
		err = verify(session)
	}
	if err != nil {
		if session != nil {
			closeSession(session)
		}
		return nil, state, err
	}

	return session, state, nil
}

func isRetryableBootstrapError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "looking for beginning of value") ||
		strings.Contains(message, "unexpected EOF") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "exit status")
}

func runManifestWarmup(t *testing.T, manifest *serverManifest, runtime serverCommand) error {
	t.Helper()

	command, timeout, ok := resolveWarmupCommand(manifest, runtime)
	if !ok {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command.Command, command.Args...)
	cmd.Env = mergeEnv(os.Environ(), command.Env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"command=%q args=%v stdout=%q stderr=%q: %w",
			command.Command,
			command.Args,
			strings.TrimSpace(stdout.String()),
			strings.TrimSpace(stderr.String()),
			err,
		)
	}

	return nil
}

func resolveWarmupCommand(manifest *serverManifest, runtime serverCommand) (serverCommand, time.Duration, bool) {
	if manifest.Bootstrap == nil || manifest.Bootstrap.Warmup == nil {
		return serverCommand{}, 0, false
	}

	warmup := manifest.Bootstrap.Warmup
	command := serverCommand{
		Command: runtime.Command,
		Args:    append([]string(nil), warmup.Args...),
	}
	if warmup.Command != "" {
		command.Command = warmup.Command
	}
	if warmup.IncludeRuntimeEnv {
		command.Env = mapsClone(runtime.Env)
	}

	timeout := warmup.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return command, timeout, true
}

func mapsClone(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func newClientWithRoots(roots []*mcp.Root) *mcp.Client {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "centian-realworld-test-client",
		Version: "1.0.0",
	}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			RootsV2: &mcp.RootCapabilities{ListChanged: true},
		},
	})

	if len(roots) > 0 {
		client.AddRoots(roots...)
	}

	return client
}

func startCentianProxyForRealworld(t *testing.T, manifest *serverManifest, downstream serverCommand) string {
	t.Helper()

	authDisabled := false
	port := allocateFreePort(t)

	globalConfig := &config.GlobalConfig{
		Name:        manifest.Name + " Integration Proxy",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Host:    "127.0.0.1",
			Port:    port,
			Timeout: int(defaultSessionTimeout.Seconds()),
		},
		Gateways: map[string]*config.GatewayConfig{
			manifest.GatewayID: {
				MCPServers: map[string]*config.MCPServerConfig{
					manifest.ServerID: {
						Command: downstream.Command,
						Args:    downstream.Args,
						Env:     downstream.Env,
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

	return fmt.Sprintf("http://127.0.0.1:%s/mcp/%s/%s", port, manifest.GatewayID, manifest.ServerID)
}

func runServerComparison(
	t *testing.T,
	manifest *serverManifest,
	testName string,
	testFn func(context.Context, *testing.T, *connectionPair, *fixtureBundle),
) {
	t.Helper()

	t.Run(testName, func(t *testing.T) {
		harness := newRealworldHarness(t, manifest)
		pair := harness.connectPair(t)

		ctx, cancel := context.WithTimeout(context.Background(), defaultSessionTimeout)
		defer cancel()

		testFn(ctx, t, pair, harness.fixture)
	})
}

func assertToolCatalogParity(
	ctx context.Context,
	t *testing.T,
	manifest *serverManifest,
	pair *connectionPair,
) {
	t.Helper()

	directTools, err := waitForTools(ctx, pair.Direct.session)
	if err != nil {
		t.Fatalf("failed to list direct tools: %v", err)
	}
	proxiedTools, err := waitForTools(ctx, pair.Proxied.session)
	if err != nil {
		t.Fatalf("failed to list proxied tools: %v", err)
	}

	directNames := toolNames(directTools.Tools)
	proxiedNames := toolNames(proxiedTools.Tools)
	slices.Sort(directNames)
	slices.Sort(proxiedNames)

	expected := append([]string(nil), manifest.ExpectedTools...)
	slices.Sort(expected)

	if !slices.Equal(expected, directNames) || !slices.Equal(expected, proxiedNames) {
		t.Fatalf(
			"unexpected tool catalog for %s\nexpected: %v\ndirect: %v\nproxied: %v",
			manifest.Name,
			expected,
			directNames,
			proxiedNames,
		)
	}

	directToolMap := toolMap(directTools.Tools)
	proxiedToolMap := toolMap(proxiedTools.Tools)
	for _, name := range expected {
		directTool := directToolMap[name]
		proxiedTool := proxiedToolMap[name]
		if proxiedTool == nil {
			t.Fatalf("missing proxied tool metadata for %q", name)
		}
		if !jsonEqual(t, directTool, proxiedTool) {
			t.Fatalf(
				"tool metadata mismatch for %q\ndirect:\n%s\nproxied:\n%s",
				name,
				prettyJSON(t, directTool),
				prettyJSON(t, proxiedTool),
			)
		}
	}
}

func assertToolCallParity(
	ctx context.Context,
	t *testing.T,
	manifest *serverManifest,
	fixture *fixtureBundle,
	pair *connectionPair,
	toolName string,
	directArgs map[string]any,
	proxiedArgs map[string]any,
) {
	t.Helper()

	directTools, err := waitForTools(ctx, pair.Direct.session)
	if err != nil {
		t.Fatalf("failed to list direct tools before calling %q: %v", toolName, err)
	}
	proxiedTools, err := waitForTools(ctx, pair.Proxied.session)
	if err != nil {
		t.Fatalf("failed to list proxied tools before calling %q: %v", toolName, err)
	}
	if _, ok := toolMap(directTools.Tools)[toolName]; !ok {
		t.Fatalf("direct tool catalog does not contain %q", toolName)
	}
	if _, ok := toolMap(proxiedTools.Tools)[toolName]; !ok {
		t.Fatalf("proxied tool catalog does not contain %q", toolName)
	}

	directResult, directErr := pair.Direct.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: directArgs,
	})
	proxiedResult, proxiedErr := pair.Proxied.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: proxiedArgs,
	})

	assertCallOutcomeParity(t, manifest, fixture, directResult, directErr, proxiedResult, proxiedErr)
}

func assertCallOutcomeParity(
	t *testing.T,
	manifest *serverManifest,
	fixture *fixtureBundle,
	directResult *mcp.CallToolResult,
	directErr error,
	proxiedResult *mcp.CallToolResult,
	proxiedErr error,
) {
	t.Helper()

	if !sameErrorMessage(directErr, proxiedErr) {
		t.Fatalf(
			"call error mismatch\ndirect error: %v\nproxied error: %v\ndirect result:\n%s\nproxied result:\n%s",
			directErr,
			proxiedErr,
			prettyJSON(t, directResult),
			prettyJSON(t, proxiedResult),
		)
	}

	if !compareNormalizedResults(t, manifest, fixture, directResult, proxiedResult) {
		t.Fatalf(
			"call result mismatch\ndirect:\n%s\nproxied:\n%s",
			prettyJSON(t, normalizeResultForComparison(t, manifest, modeDirect, fixture, directResult)),
			prettyJSON(t, normalizeResultForComparison(t, manifest, modeProxied, fixture, proxiedResult)),
		)
	}
}

func compareNormalizedResults(
	t *testing.T,
	manifest *serverManifest,
	fixture *fixtureBundle,
	directResult *mcp.CallToolResult,
	proxiedResult *mcp.CallToolResult,
) bool {
	t.Helper()

	directNormalized := normalizeResultForComparison(t, manifest, modeDirect, fixture, directResult)
	proxiedNormalized := normalizeResultForComparison(t, manifest, modeProxied, fixture, proxiedResult)
	return jsonEqual(t, directNormalized, proxiedNormalized)
}

func normalizeResultForComparison(
	t *testing.T,
	manifest *serverManifest,
	mode serverMode,
	fixture *fixtureBundle,
	value any,
) any {
	t.Helper()

	if value == nil {
		return nil
	}

	if manifest.Normalize == nil {
		return value
	}

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to marshal normalized value: %v", err)
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed to unmarshal normalized value: %v", err)
	}

	return manifest.Normalize(mode, decoded, fixture)
}

func fixtureForMode(fixture *fixtureBundle, mode serverMode) modeFixture {
	if mode == modeDirect {
		return fixture.Direct
	}
	return fixture.Proxied
}

func modePath(fixture modeFixture, rel string) string {
	if rel == "" {
		return fixture.RootDir
	}
	return filepath.Join(fixture.RootDir, filepath.FromSlash(rel))
}

func fileURI(path string) string {
	return (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}).String()
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}

	values := make(map[string]string, len(base)+len(extra))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range extra {
		values[key] = value
	}

	merged := make([]string, 0, len(values))
	for key, value := range values {
		merged = append(merged, key+"="+value)
	}
	slices.Sort(merged)
	return merged
}

func waitForTools(ctx context.Context, session *mcp.ClientSession) (*mcp.ListToolsResult, error) {
	deadline := time.Now().Add(defaultToolsWaitTimeout)
	var lastErr error

	for time.Now().Before(deadline) {
		toolsResult, err := session.ListTools(ctx, nil)
		if err == nil && len(toolsResult.Tools) > 0 {
			return toolsResult, nil
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return &mcp.ListToolsResult{}, nil
}

func waitForCallResult(
	ctx context.Context,
	session *mcp.ClientSession,
	toolName string,
	args map[string]any,
	accept func(*mcp.CallToolResult, error) bool,
) (*mcp.CallToolResult, error) {
	deadline := time.Now().Add(10 * time.Second)
	var lastResult *mcp.CallToolResult
	var lastErr error

	for time.Now().Before(deadline) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: args})
		lastResult = result
		lastErr = err
		if accept(result, err) {
			return result, err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return lastResult, lastErr
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

func prettyJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal json: %v", err)
	}

	return string(data)
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()

	aJSON, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("failed to marshal first value: %v", err)
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("failed to marshal second value: %v", err)
	}

	return bytes.Equal(aJSON, bJSON)
}

func sameErrorMessage(left, right error) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.Error() == right.Error()
	}
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

func copyTree(t *testing.T, srcDir, dstDir string) {
	t.Helper()

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("failed to create destination dir %s: %v", dstDir, err)
	}

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		if err := copyFile(path, target, info.Mode()); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to copy fixture tree from %s to %s: %v", srcDir, dstDir, err)
	}
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func replaceStrings(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)

		normalized := make(map[string]any, len(typed))
		for _, key := range keys {
			normalized[key] = replaceStrings(typed[key], replacements)
		}
		return normalized
	case []any:
		items := make([]any, len(typed))
		for i := range typed {
			items[i] = replaceStrings(typed[i], replacements)
		}
		return items
	case string:
		replaced := typed
		keys := make([]string, 0, len(replacements))
		for key := range replacements {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			replaced = strings.ReplaceAll(replaced, key, replacements[key])
		}
		return replaced
	default:
		return typed
	}
}

func TestResolveWarmupCommandUsesRuntimeEnvByDefault(t *testing.T) {
	manifest := &serverManifest{
		Bootstrap: &bootstrapConfig{
			Warmup: &warmupCommand{
				Args:              []string{"tool", "--help"},
				IncludeRuntimeEnv: true,
				Timeout:           3 * time.Second,
			},
		},
	}
	runtime := serverCommand{
		Command: "uvx",
		Args:    []string{"mcp-server-fetch"},
		Env:     map[string]string{"FETCH_TOKEN": "secret"},
	}

	command, timeout, ok := resolveWarmupCommand(manifest, runtime)

	if !ok {
		t.Fatalf("expected warmup command to resolve")
	}
	if command.Command != "uvx" {
		t.Fatalf("expected runtime command to be reused, got %q", command.Command)
	}
	if !slices.Equal(command.Args, []string{"tool", "--help"}) {
		t.Fatalf("unexpected warmup args: %v", command.Args)
	}
	if timeout != 3*time.Second {
		t.Fatalf("unexpected warmup timeout: %v", timeout)
	}
	if !mapsEqual(command.Env, runtime.Env) {
		t.Fatalf("expected runtime env to be forwarded, got %v", command.Env)
	}
	command.Env["FETCH_TOKEN"] = "changed"
	if runtime.Env["FETCH_TOKEN"] != "secret" {
		t.Fatalf("expected warmup env map to be cloned")
	}
}

func TestResolveWarmupCommandSkippedWithoutConfig(t *testing.T) {
	command, timeout, ok := resolveWarmupCommand(&serverManifest{}, serverCommand{Command: "uvx"})
	if ok {
		t.Fatalf("expected warmup to be skipped, got command=%+v timeout=%v", command, timeout)
	}
}

func TestConnectWithBootstrapRetryDoesNotRetryWhenDisabled(t *testing.T) {
	connectCalls := 0
	warmupCalls := 0

	_, state, err := connectWithBootstrapRetry(
		func() (*instrumentedSession, string, error) {
			connectCalls++
			return nil, "stderr", errors.New("invalid character 'a' looking for beginning of value")
		},
		func(*instrumentedSession) {},
		nil,
		func() error {
			warmupCalls++
			return nil
		},
		false,
	)

	if err == nil {
		t.Fatalf("expected bootstrap failure")
	}
	if connectCalls != 1 {
		t.Fatalf("expected one connect attempt, got %d", connectCalls)
	}
	if warmupCalls != 0 {
		t.Fatalf("expected no retry warmup, got %d", warmupCalls)
	}
	if state.Retried {
		t.Fatalf("expected retry to remain disabled")
	}
}

func TestConnectWithBootstrapRetryRetriesOnceAfterWarmup(t *testing.T) {
	connectCalls := 0
	warmupCalls := 0
	closeCalls := 0

	_, state, err := connectWithBootstrapRetry(
		func() (*instrumentedSession, string, error) {
			connectCalls++
			if connectCalls == 1 {
				return nil, "first stderr", errors.New("invalid character 'a' looking for beginning of value")
			}
			return &instrumentedSession{mode: modeDirect}, "second stderr", nil
		},
		func(*instrumentedSession) {
			closeCalls++
		},
		func(*instrumentedSession) error { return nil },
		func() error {
			warmupCalls++
			return nil
		},
		true,
	)

	if err != nil {
		t.Fatalf("expected retry to recover, got %v", err)
	}
	if connectCalls != 2 {
		t.Fatalf("expected two connect attempts, got %d", connectCalls)
	}
	if warmupCalls != 1 {
		t.Fatalf("expected one retry warmup, got %d", warmupCalls)
	}
	if closeCalls != 0 {
		t.Fatalf("expected no session close on connect failure, got %d", closeCalls)
	}
	if !state.Retried || !state.RetryWarmupRan {
		t.Fatalf("expected retry state to record warmup and retry, got %+v", state)
	}
	if state.Stderr != "second stderr" {
		t.Fatalf("expected final stderr to be retained, got %q", state.Stderr)
	}
}

func TestConnectWithBootstrapRetryDoesNotRetryNonBootstrapErrors(t *testing.T) {
	connectCalls := 0
	warmupCalls := 0
	closeCalls := 0

	_, state, err := connectWithBootstrapRetry(
		func() (*instrumentedSession, string, error) {
			connectCalls++
			return &instrumentedSession{mode: modeDirect}, "stderr", nil
		},
		func(*instrumentedSession) {
			closeCalls++
		},
		func(*instrumentedSession) error {
			return errors.New("tool schema mismatch")
		},
		func() error {
			warmupCalls++
			return nil
		},
		true,
	)

	if err == nil {
		t.Fatalf("expected verification failure")
	}
	if connectCalls != 1 {
		t.Fatalf("expected one connect attempt, got %d", connectCalls)
	}
	if closeCalls != 1 {
		t.Fatalf("expected failed verified session to be closed once, got %d", closeCalls)
	}
	if warmupCalls != 0 {
		t.Fatalf("expected no retry warmup for non-bootstrap error, got %d", warmupCalls)
	}
	if state.Retried {
		t.Fatalf("expected no retry for non-bootstrap error")
	}
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}

	for key, leftValue := range left {
		if right[key] != leftValue {
			return false
		}
	}
	return true
}
