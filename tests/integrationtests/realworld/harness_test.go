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

type inlineGatewayProvider struct{ file *config.GatewayFile }

func (p *inlineGatewayProvider) LoadGatewayFile() (*config.GatewayFile, error) { return p.file, nil }
func (p *inlineGatewayProvider) SaveGatewayFile(_ *config.GatewayFile) error   { return nil }

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
	manifest *serverManifest
	fixture  *fixtureBundle
	proxyURL string
}

func newRealworldHarness(t *testing.T, manifest *serverManifest) *realworldHarness {
	t.Helper()

	requireRealworldEnabled(t)

	fixture := manifest.BuildFixture(t)
	proxiedCommand := loadServerCommand(t, manifest, modeProxied, fixture)
	proxyURL := startCentianProxyForRealworld(t, manifest, proxiedCommand)

	return &realworldHarness{
		manifest: manifest,
		fixture:  fixture,
		proxyURL: proxyURL,
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

	command := loadServerCommand(t, h.manifest, modeDirect, h.fixture)
	cmd := exec.Command(command.Command, command.Args...)
	cmd.Env = mergeEnv(os.Environ(), command.Env)

	return connectInstrumentedSession(
		t,
		modeDirect,
		fixtureForMode(h.fixture, modeDirect).Roots,
		&mcp.CommandTransport{Command: cmd},
	)
}

func (h *realworldHarness) connectViaCentian(t *testing.T) *instrumentedSession {
	t.Helper()

	return connectInstrumentedSession(
		t,
		modeProxied,
		fixtureForMode(h.fixture, modeProxied).Roots,
		&mcp.StreamableClientTransport{
			Endpoint:   h.proxyURL,
			HTTPClient: &http.Client{Timeout: defaultSessionTimeout},
		},
	)
}

func connectInstrumentedSession(
	t *testing.T,
	mode serverMode,
	roots []*mcp.Root,
	transport mcp.Transport,
) *instrumentedSession {
	t.Helper()

	client := newClientWithRoots(roots)
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
		mode:    mode,
		client:  client,
		session: session,
	}
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

	serverConfig := &config.GlobalConfig{
		Name:        manifest.Name + " Integration Proxy",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Host:    "127.0.0.1",
			Port:    port,
			Timeout: int(defaultSessionTimeout.Seconds()),
		},
	}
	gf := &config.GatewayFile{
		Version: "1.0.0",
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

	server, err := proxy.NewCentianServer(serverConfig, &inlineGatewayProvider{file: gf})
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
