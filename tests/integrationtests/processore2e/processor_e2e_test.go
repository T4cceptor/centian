//go:build e2e

// Package processore2e contains an interactive end-to-end harness that drives a
// real coding agent through an in-process Centian proxy and asserts that a
// builtin processor actually fired. It doubles as a live demo: after the
// assertions run it keeps the proxy open so the events can be inspected in the
// web UI, and only removes its temporary artifacts once the user stops it
// (Ctrl+C).
//
// Run it via the Makefile target:
//
//	make test-processor-e2e                       # uses claude
//	make test-processor-e2e CENTIAN_E2E_AGENT=codex
//
// v1 covers a single processor (secret_token_redactor) and a single task: the
// agent reads a file containing a fake secret through the proxy, and the
// redactor strips the secret from the response on the way back.
package processore2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/proxy"
)

const (
	runEnv   = "CENTIAN_RUN_PROCESSOR_E2E"
	agentEnv = "CENTIAN_E2E_AGENT"

	projectSlug = "e2e"
	gatewayName = "files"
	serverName  = "fs"

	// apiKeyValue is a fake-but-pattern-valid Anthropic key the redactor matches.
	apiKeyValue = "sk-ant-abc123DEADBEEFcafef00dbaadf00d1234567890"

	// configFileName is an innocuous service config file the agent reviews.
	configFileName = "staging-config.txt"
)

// TestProcessorE2E runs one real agent against an in-process proxy with the
// secret_token_redactor enabled, asserts the redaction took effect, then holds
// the server open for visual inspection until interrupted.
func TestProcessorE2E(t *testing.T) {
	// Given: the harness is explicitly requested and the required CLIs exist.
	if os.Getenv(runEnv) != "1" {
		t.Skipf("set %s=1 to run the interactive processor e2e/demo harness", runEnv)
	}
	agentName := strings.ToLower(strings.TrimSpace(os.Getenv(agentEnv)))
	if agentName == "" {
		agentName = common.AgentClaude
	}
	agentBinary := agentName
	if agentName == common.AgentCodexOllama {
		agentBinary = common.AgentCodex
	}
	if _, err := exec.LookPath(agentBinary); err != nil {
		t.Skipf("agent %q is not available on PATH: %v", agentBinary, err)
	}
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skipf("npx is not available on PATH: %v", err)
	}

	// Lay down temp artifacts. t.TempDir auto-cleans only after this function
	// returns, so blocking on Ctrl+C below keeps them alive for inspection and
	// removes them on close.
	// Serve files from the agent's own workspace so the target file is inside
	// the agent's allowed directory — agents refuse paths outside their cwd.
	workspace := t.TempDir()
	fsRoot := workspace
	configFile := filepath.Join(fsRoot, configFileName)
	configContent := strings.Join([]string{
		"# Billing service - staging configuration",
		"SERVICE_NAME=billing-api",
		"LOG_LEVEL=info",
		"MAX_RETRIES=3",
		"ANTHROPIC_API_KEY=" + apiKeyValue,
		"REQUEST_TIMEOUT_SECONDS=30",
		"",
	}, "\n")
	if err := os.WriteFile(configFile, []byte(configContent), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	artifactRoot := t.TempDir()
	eventDB := filepath.Join(t.TempDir(), "events.sqlite")

	// When: the proxy starts in-process and a real agent reads the file.
	cfg := buildE2EConfig(fsRoot, eventDB)
	server, err := proxy.NewCentianServerWithOptions(
		config.NewConfigProviderFromConfig(cfg),
		proxy.CentianServerOptions{
			LoggerFactory: func(string) (*logging.Logger, error) {
				return logging.NewDiscardLogger()
			},
		},
	)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	t.Cleanup(func() {
		for _, e := range server.Close() {
			t.Logf("close server: %v", e)
		}
	})
	if err := server.Setup(); err != nil {
		t.Fatalf("setup server: %v", err)
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(config.DefaultProxyHost, "0"))
	if err != nil {
		t.Fatalf("bind listener: %v", err)
	}
	server.Server.Addr = listener.Addr().String()
	serveErr := make(chan error, 1)
	go func() {
		if e := server.Server.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
			serveErr <- e
			return
		}
		serveErr <- nil
	}()

	addr := listener.Addr().String()
	mcpURL := fmt.Sprintf("http://%s/%s/mcp/%s/%s", addr, projectSlug, gatewayName, serverName)
	uiURL := fmt.Sprintf("http://%s/ui/%s/activity?range=live", addr, projectSlug)

	// Print the inspection details up front so they're easy to follow while the
	// agent run (which can take minutes) is in progress.
	t.Logf("Centian proxy is up.")
	t.Logf("  UI:                       %s", uiURL)
	t.Logf("  MCP:                      %s", mcpURL)
	t.Logf("  workspace / served files: %s", workspace)
	t.Logf("  config file:              %s", configFile)
	t.Logf("  agent artifacts:          %s", artifactRoot)
	t.Logf("  event store (sqlite):     %s", eventDB)
	t.Logf("Running agent now; events will populate the UI as it works...")

	project := server.Projects[projectSlug]
	if project == nil || project.PersistenceStore == nil {
		t.Fatalf("project %q event store is unavailable", projectSlug)
	}
	store := project.PersistenceStore

	prompt := fmt.Sprintf(
		"I'm reviewing the configuration for our billing service. Read the file at %q "+
			"and list every configuration setting it defines, one per line in `KEY = VALUE` "+
			"form, so I can see the current values.",
		configFile,
	)
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, runErr := agentrunner.Run(runCtx, &agentrunner.RunOptions{
		Execution:     agentrunner.AgentExecutionOptions{Agent: agentName},
		ArtifactRoot:  artifactRoot,
		WorkspacePath: workspace,
		MCPURL:        mcpURL,
		Prompt:        prompt,
		Timeout:       5 * time.Minute,
		Stdout:        &testLogWriter{t: t},
		Stderr:        &testLogWriter{t: t},
	})
	if runErr != nil {
		t.Logf("agent run error (continuing to soft-assert and hold open): %v", runErr)
	}

	// Then: collect soft assertions (don't abort, so the hold-open still runs).
	failures := assertRedaction(t, store, result)

	// Report, then hold the server open for inspection. Re-print the URL since
	// the agent output will have scrolled past the startup banner.
	t.Logf("Agent run complete. Inspect events at: %s", uiURL)
	t.Logf("Temp artifacts (removed on Ctrl+C) are under: %s", workspace)
	if len(failures) == 0 {
		t.Logf("ALL ASSERTIONS PASSED")
	} else {
		for _, f := range failures {
			t.Logf("ASSERTION FAILED: %s", f)
		}
	}
	t.Logf("Server held open for inspection. Press Ctrl+C to shut down and clean up temp artifacts.")

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-stopCtx.Done():
	case e := <-serveErr:
		if e != nil {
			t.Logf("server exited early: %v", e)
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Server.Shutdown(shutdownCtx)

	// Final verdict only after the user closes, so failures still leave the
	// server up for inspection.
	if len(failures) > 0 {
		t.Fatalf("%d assertion(s) failed:\n  - %s", len(failures), strings.Join(failures, "\n  - "))
	}
}

// assertRedaction checks the three v1 guarantees: the agent's answer is redacted,
// an action event recorded the read, and a governance annotation was emitted.
func assertRedaction(t *testing.T, store *persistence.Store, result *agentrunner.RunResult) []string {
	t.Helper()
	var failures []string

	// (a) The agent's answer must be redacted, not contain the raw secret.
	if result == nil {
		failures = append(failures, "no agent result returned")
	} else {
		out, readErr := os.ReadFile(result.StdoutPath)
		if readErr != nil {
			failures = append(failures, "read agent stdout log: "+readErr.Error())
		} else {
			text := string(out)
			if strings.Contains(text, apiKeyValue) {
				failures = append(failures, "raw secret present in agent output (not redacted)")
			}
			if !strings.Contains(text, "[REDACTED_") {
				failures = append(failures, "no [REDACTED_*] marker found in agent output")
			}
		}
	}

	// (b) An action event must exist for the read tool call.
	events, err := store.ActionEvents()
	if err != nil {
		failures = append(failures, "ActionEvents: "+err.Error())
	} else {
		hasRead := false
		for _, e := range events {
			if e.ToolName == "read_text_file" || e.ToolName == "read_file" {
				hasRead = true
				break
			}
		}
		if !hasRead {
			failures = append(failures, "no action event recorded for a read tool call")
		}
	}

	// (c) A governance annotation from the redactor must exist.
	page, err := store.ListEvents(context.Background(), &persistence.EventListFilter{
		WithGovernanceEvent: true,
		Limit:               100,
	})
	if err != nil {
		failures = append(failures, "ListEvents: "+err.Error())
	} else {
		govOK := false
		for _, item := range page.Items {
			for _, a := range item.Annotations {
				if a.Processor == config.BuiltinSecretTokenRedactor && a.Action == "redacted" && a.Category == "security" {
					govOK = true
					break
				}
			}
		}
		if !govOK {
			failures = append(failures, "no governance annotation (secret_token_redactor/redacted/security)")
		}
	}

	return failures
}

// buildE2EConfig builds a project-based config with one gateway pointing at the
// filesystem MCP server and the secret_token_redactor attached at the gateway
// level (the only level instantiated into the processing chain).
func buildE2EConfig(fsRoot, eventDBPath string) *config.GlobalConfig {
	authDisabled := false
	enabled := true

	return &config.GlobalConfig{
		Name:    "Centian Processor E2E",
		Version: "1.0.0",
		Proxy: &config.ProxySettings{
			Host:    config.DefaultProxyHost,
			Port:    "0",
			Timeout: 60,
		},
		Projects: map[string]*config.ProjectConfig{
			projectSlug: {
				Slug:        projectSlug,
				Description: "Interactive processor e2e/demo project.",
				AuthEnabled: &authDisabled,
				Capabilities: &config.CapabilitiesSettings{
					UI: &config.UICapabilitySettings{Enabled: &enabled},
					EventStorage: &config.EventStorageCapabilitySettings{
						Enabled: &enabled,
						Driver:  config.DefaultEventStorageDriver,
						Path:    eventDBPath,
					},
				},
				Gateways: map[string]*config.GatewayConfig{
					gatewayName: {
						MCPServers: map[string]*config.MCPServerConfig{
							serverName: {
								Name:    serverName,
								Command: "npx",
								Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", fsRoot},
							},
						},
						Processors: []*config.ProcessorConfig{
							{
								Name:    config.BuiltinSecretTokenRedactor,
								Type:    string(config.BuiltinProcessor),
								Enabled: true,
								Parts:   []string{"payload", "annotations"},
								Config: map[string]interface{}{
									"processor": config.BuiltinSecretTokenRedactor,
									"mode":      config.BuiltinRedactionModeRedact,
									"scope":     config.BuiltinRedactionScopeResponse,
								},
							},
						},
					},
				},
				Metadata: map[string]interface{}{"name": "Centian Processor E2E"},
			},
		},
	}
}

// testLogWriter streams agent stdout/stderr into the test log.
type testLogWriter struct{ t *testing.T }

func (w *testLogWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
