//go:build !windows

package demo

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const runDemoIntegrationEnv = "CENTIAN_RUN_DEMO_INTEGRATION"

func TestCentianDemoClaude(t *testing.T) {
	if os.Getenv(runDemoIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run demo integration tests", runDemoIntegrationEnv)
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Fatalf("claude is not available: %v", err)
	}
	if _, err := exec.LookPath("npx"); err != nil {
		t.Fatalf("npx is not available: %v", err)
	}

	root := t.TempDir()
	binary := filepath.Join(root, "centian")
	build := exec.Command("go", "build", "-o", binary, "./cmd/main.go")
	build.Dir = repoRoot(t)
	build.Env = append(os.Environ(), "GOCACHE=/tmp/centian-gocache")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build centian binary: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	demoRoot := filepath.Join(root, "demo")
	cmd := exec.Command(binary, "demo", "--agent", "claude", "--path", demoRoot)
	cmd.Dir = repoRoot(t)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run centian demo: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	for _, path := range []string{
		filepath.Join(demoRoot, "workspace"),
		filepath.Join(demoRoot, "templates", "python_tdd_workflow.yaml"),
		filepath.Join(demoRoot, "logs"),
		filepath.Join(demoRoot, "config.json"),
		filepath.Join(demoRoot, "prompt.md"),
		filepath.Join(demoRoot, "claude_mcp_config.json"),
		filepath.Join(demoRoot, "centian.pid"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected demo artifact %s: %v", path, err)
		}
	}

	pidBytes, err := os.ReadFile(filepath.Join(demoRoot, "centian.pid"))
	if err != nil {
		t.Fatalf("read centian.pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatalf("parse centian pid: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	})

	port := readPort(t, filepath.Join(demoRoot, "config.json"))
	baseURL := "http://127.0.0.1:" + port
	waitForHTTP(t, baseURL+"/api/task-runs")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("failed to locate repo root")
		}
		dir = parent
	}
}

func readPort(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var payload struct {
		Proxy struct {
			Port string `json:"port"`
		} `json:"proxy"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if payload.Proxy.Port == "" {
		t.Fatal("config port is empty")
	}
	return payload.Proxy.Port
}

func waitForHTTP(t *testing.T, url string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}
