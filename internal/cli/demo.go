package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/proxy"
	"github.com/urfave/cli/v3"
)

const (
	demoProjectSlug = "demo"
	demoProjectName = "Centian Demonstration"
	demoRunID       = "tr_1742947200123_itopsdemo1"
)

var demoOpenBrowser = openBrowserURL

// DemoCommand launches a self-contained Centian demo workspace.
var DemoCommand = &cli.Command{
	Name:  "demo",
	Usage: "Start an in-memory Centian demonstration server",
	Description: `Start a diskless Centian demo server, seed the bundled IT Ops
scenario into an in-memory event database, and open the task detail view for
post-hoc inspection.

Deprecated --file, --agent, and --path flows are no longer available from this
command and will likely be moved or removed in a future release.

Examples:
  centian demo
Deprecated:
  centian demo --file ./demo_scenario.json
  centian demo --agent claude
  centian demo --agent gemini
  centian demo --agent codex --model gpt-5.4-mini
  centian demo --agent codex-ollama --codex-config ~/.codex/config.toml --profile my-local-oss
  centian demo --agent claude --path ./my-demo
`,
	Action: handleDemoCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "agent",
			Aliases: []string{"a"},
			Usage:   "Deprecated: agent to run instead of the static IT Ops demo (v1 supports: claude, gemini, codex, codex-ollama)",
		},
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Deprecated: synthetic demo scenario JSON file to seed immediately",
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Usage:   "Deprecated: disk-backed demo workspace path is no longer supported",
		},
		&cli.StringFlag{
			Name:    "model",
			Aliases: []string{"m"},
			Usage:   singleModelFlagUsage(),
		},
		&cli.StringFlag{
			Name:  "profile",
			Usage: "Codex Ollama profile from the supplied Codex config (" + codexOllamaProfileHelp + ")",
		},
		&cli.StringFlag{
			Name:  "codex-config",
			Usage: "Base Codex config to copy and patch for codex or codex-ollama runs",
		},
	},
}

// handleDemoCommand resolves demo inputs and runs one local demo session.
func handleDemoCommand(ctx context.Context, cmd *cli.Command) error {
	if err := rejectDeprecatedDemoFlags(cmd); err != nil {
		return err
	}
	return runInMemoryDemo(ctx, os.Stdout, os.Stderr)
}

func rejectDeprecatedDemoFlags(cmd *cli.Command) error {
	for _, flagName := range []string{"agent", "file", "path", "model", "profile", "codex-config"} {
		if strings.TrimSpace(cmd.String(flagName)) == "" {
			continue
		}
		return fmt.Errorf("centian demo --%s is deprecated and is not supported by the in-memory demo", flagName)
	}
	return nil
}

func runInMemoryDemo(ctx context.Context, stdout, stderr io.Writer) error {
	server, err := newInMemoryDemoServer(ctx)
	if err != nil {
		return err
	}
	cleanupServer := true
	defer func() {
		if cleanupServer {
			closeDemoServer(server, stderr)
		}
	}()

	shutdown, err := serveInMemoryDemo(ctx, server, stdout, stderr)
	if shutdown {
		cleanupServer = false
	}
	return err
}

func newInMemoryDemoServer(ctx context.Context) (*proxy.CentianServer, error) {
	server, err := proxy.NewCentianServerWithOptions(newDemoConfig(), proxy.CentianServerOptions{
		LoggerFactory: func(string) (*logging.Logger, error) {
			return logging.NewDiscardLogger()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create demo server: %w", err)
	}
	if err := server.Setup(); err != nil {
		closeDemoServer(server, nil)
		return nil, fmt.Errorf("setup demo server: %w", err)
	}
	project := server.Projects[demoProjectSlug]
	if project == nil || project.PersistenceStore == nil {
		closeDemoServer(server, nil)
		return nil, fmt.Errorf("demo project event store is unavailable")
	}
	if _, err := agentrunner.StartSyntheticDemoRunWithOptions(ctx, project.PersistenceStore, "it_ops", agentrunner.SyntheticDemoRunOptions{RunID: demoRunID}); err != nil {
		closeDemoServer(server, nil)
		return nil, fmt.Errorf("seed demo run: %w", err)
	}
	return server, nil
}

func serveInMemoryDemo(ctx context.Context, server *proxy.CentianServer, stdout, stderr io.Writer) (bool, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(config.DefaultProxyHost, "0"))
	if err != nil {
		return false, fmt.Errorf("bind demo server: %w", err)
	}
	server.Server.Addr = listener.Addr().String()
	errCh := make(chan error, 1)
	go func() {
		if err := server.Server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	uiURL := fmt.Sprintf("http://%s/ui/%s/tasks/%s", listener.Addr().String(), demoProjectSlug, demoRunID)
	if err := demoOpenBrowser(uiURL); err != nil && stderr != nil {
		_, _ = fmt.Fprintf(stderr, "warning: open browser: %v\n", err)
	}
	if stdout != nil {
		_, _ = fmt.Fprintf(stdout, "Demo ready. UI: %s\n", uiURL)
		_, _ = fmt.Fprintln(stdout, "Press Ctrl+C to stop")
	}

	stopCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-stopCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Server.Shutdown(shutdownCtx); err != nil {
			return false, fmt.Errorf("shutdown demo server: %w", err)
		}
		<-errCh
		return true, nil
	case err := <-errCh:
		if err != nil {
			return false, fmt.Errorf("demo server: %w", err)
		}
		return false, nil
	}
}

func closeDemoServer(server *proxy.CentianServer, stderr io.Writer) {
	for _, err := range server.Close() {
		if stderr != nil {
			_, _ = fmt.Fprintf(stderr, "warning: close demo server: %v\n", err)
		}
	}
}

func newDemoConfig() *config.GlobalConfig {
	authDisabled := false
	uiEnabled := true
	eventStorageEnabled := true
	taskVerificationEnabled := true
	return &config.GlobalConfig{
		Name:    "Centian Demonstration",
		Version: "1.0.0",
		Proxy: &config.ProxySettings{
			Host:    config.DefaultProxyHost,
			Port:    "0",
			Timeout: 30,
		},
		Projects: map[string]*config.ProjectConfig{
			demoProjectSlug: {
				Slug:        demoProjectSlug,
				Description: "Bundled in-memory Centian demonstration project.",
				AuthEnabled: &authDisabled,
				Capabilities: &config.CapabilitiesSettings{
					UI: &config.UICapabilitySettings{
						Enabled: &uiEnabled,
					},
					EventStorage: &config.EventStorageCapabilitySettings{
						Enabled: &eventStorageEnabled,
						Driver:  config.DefaultEventStorageDriver,
						Path:    ":memory:",
					},
					TaskVerification: &config.TaskVerificationCapabilitySettings{
						Enabled: &taskVerificationEnabled,
					},
				},
				Gateways: map[string]*config.GatewayConfig{},
				Metadata: map[string]interface{}{
					"name": demoProjectName,
				},
			},
		},
	}
}

//nolint:gosec // The demo opens a Centian-generated local URL in the user's browser.
func openBrowserURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func demoScenarioFileFromFlags(cmd *cli.Command) (string, error) {
	for _, flagName := range []string{"model", "profile", "codex-config"} {
		if strings.TrimSpace(cmd.String(flagName)) != "" {
			return "", fmt.Errorf("--%s can only be used with --agent", flagName)
		}
	}
	return resolveOptionalPath(cmd.String("file"))
}

// demoExecutionFromFlags converts demo agent flags into one normalized execution config.
func demoExecutionFromFlags(cmd *cli.Command) (agentrunner.AgentExecutionOptions, error) {
	agent := strings.TrimSpace(cmd.String("agent"))
	execution := agentrunner.AgentExecutionOptions{
		Agent:   agent,
		Profile: strings.TrimSpace(cmd.String("profile")),
	}
	if model := strings.TrimSpace(cmd.String("model")); model != "" {
		if strings.EqualFold(agent, agentrunner.AgentCodexOllama) {
			return agentrunner.AgentExecutionOptions{}, fmt.Errorf("--model is not supported for codex-ollama; use --profile")
		}
		execution.Model = normalizeCLIModel(agent, model)
	}
	codexConfigPath, err := resolveOptionalPath(cmd.String("codex-config"))
	if err != nil {
		return agentrunner.AgentExecutionOptions{}, err
	}
	execution.CodexConfigPath = codexConfigPath
	if strings.EqualFold(agent, agentrunner.AgentCodexOllama) {
		if codexConfigPath == "" {
			return agentrunner.AgentExecutionOptions{}, fmt.Errorf("codex-ollama requires --codex-config")
		}
		if execution.Profile == "" {
			return agentrunner.AgentExecutionOptions{}, fmt.Errorf("codex-ollama requires --profile")
		}
	} else if execution.Profile != "" {
		return agentrunner.AgentExecutionOptions{}, fmt.Errorf("--profile can only be used with --agent codex-ollama")
	}
	return agentrunner.NormalizeExecutionOptions(execution)
}

// promptDemoShutdown optionally stops the demo Centian process after the agent run ends.
func promptDemoShutdown(input io.Reader, output io.Writer, result *agentrunner.DemoResult) error {
	if result == nil || result.PID <= 0 {
		return nil
	}
	_, _ = fmt.Fprint(output, "Shut down the Centian server now? (Y/n): ")
	if !shouldShutdownDemo(input) {
		return nil
	}
	process, err := os.FindProcess(result.PID)
	if err != nil {
		return fmt.Errorf("resolve centian process: %w", err)
	}
	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("shut down centian server: %w", err)
	}
	return nil
}

// shouldShutdownDemo interprets an interactive shutdown prompt response.
func shouldShutdownDemo(input io.Reader) bool {
	if input == nil {
		return true
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return true
	}
	switch value := normalizePromptAnswer(line); value {
	case "", "y", "yes":
		return true
	default:
		return false
	}
}

// normalizePromptAnswer trims and lowercases a yes/no prompt response.
func normalizePromptAnswer(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// resolveDemoRoot returns the explicit demo path or the default workspace location.
func resolveDemoRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Abs(filepath.Join(cwd, ".centian", "demo"))
}

// resolveOptionalPath absolutizes a non-empty path flag and leaves empty values unset.
func resolveOptionalPath(flagValue string) (string, error) {
	value := strings.TrimSpace(flagValue)
	if value == "" {
		return "", nil
	}
	return filepath.Abs(value)
}
