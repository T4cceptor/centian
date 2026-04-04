//go:build !windows

package taskverification

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/benchmarks"
)

const runBenchmarkSmokeEnv = "CENTIAN_RUN_TASKVERIFICATION_BENCHMARK_SMOKE"
const benchmarkAgentEnv = "CENTIAN_TASKVERIFICATION_BENCHMARK_AGENT"
const benchmarkCaseEnv = "CENTIAN_TASKVERIFICATION_BENCHMARK_CASE"

func TestTaskVerificationBenchmarkSmoke(t *testing.T) {
	if os.Getenv(runBenchmarkSmokeEnv) != "1" {
		t.Skipf("set %s=1 to run taskverification benchmark smoke tests", runBenchmarkSmokeEnv)
	}

	agent := strings.TrimSpace(os.Getenv(benchmarkAgentEnv))
	if agent == "" {
		agent = "codex"
	}
	caseID := strings.TrimSpace(os.Getenv(benchmarkCaseEnv))
	if caseID == "" {
		caseID = "assertion_failure_red"
	}

	if _, err := exec.LookPath(agent); err != nil {
		t.Fatalf("%s is not available: %v", agent, err)
	}
	if _, err := exec.LookPath("npx"); err != nil {
		t.Fatalf("npx is not available: %v", err)
	}

	root := t.TempDir()
	binary := filepath.Join(root, "centian")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = repoRoot(t)
	build.Env = append(os.Environ(), "GOCACHE=/tmp/centian-gocache")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build centian binary: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	outputRoot := filepath.Join(root, "benchmark-output")
	suitePath := filepath.Join(repoRoot(t), "tests", "integrationtests", "taskverification", "benchmarks", "simple_tdd_v1")
	cmd := exec.Command(
		binary,
		"benchmark",
		"run",
		"--suite", suitePath,
		"--agent", agent,
		"--case", caseID,
		"--repeat", "1",
		"--output-root", outputRoot,
	)
	cmd.Dir = repoRoot(t)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run benchmark smoke: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	matches, err := filepath.Glob(filepath.Join(outputRoot, "simple_tdd_v1", "*", "session.json"))
	if err != nil {
		t.Fatalf("glob session manifest: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one session manifest, got %d", len(matches))
	}

	sessionPath := matches[0]
	sessionBytes, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session manifest: %v", err)
	}
	var session benchmarks.SessionManifest
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		t.Fatalf("parse session manifest: %v", err)
	}
	if session.Status != "completed" {
		t.Fatalf("expected completed session, got %s", session.Status)
	}
	if len(session.Runs) != 1 {
		t.Fatalf("expected one run entry, got %d", len(session.Runs))
	}

	runPath := filepath.Join(filepath.Dir(sessionPath), session.Runs[0].RelativeRunDir, "run.json")
	runBytes, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read run manifest: %v", err)
	}
	var run benchmarks.RunManifest
	if err := json.Unmarshal(runBytes, &run); err != nil {
		t.Fatalf("parse run manifest: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("expected completed run, got %s", run.Status)
	}
	if run.LatestTaskRunID == "" || len(run.LinkedTaskRunIDs) == 0 {
		t.Fatalf("expected linked task runs in manifest: %+v", run)
	}
	if _, err := os.Stat(run.ArtifactPaths.TaskRunsSnapshot); err != nil {
		t.Fatalf("expected task runs snapshot: %v", err)
	}
	if _, err := os.Stat(run.ArtifactPaths.TaskRunEventsDir); err != nil {
		t.Fatalf("expected task run events dir: %v", err)
	}
	if _, err := os.Stat(run.ArtifactPaths.RequestLogPath); err != nil {
		t.Fatalf("expected request log path: %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := benchmarks.FindRepoRoot(".")
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	return root
}
