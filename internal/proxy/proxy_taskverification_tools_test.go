package proxy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func newTaskToolTestProxy(t *testing.T, templateContent string) (*CentianEndpoint, *UpstreamSession) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	templateDir := t.TempDir()
	workingDir := t.TempDir()
	err := os.WriteFile(filepath.Join(templateDir, "task.yaml"), []byte(templateContent), 0o644)
	assert.NilError(t, err)

	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = logger.Close()
	})

	endpoint := &CentianEndpoint{
		name:     "tasks",
		endpoint: "/mcp/gateway/tasks",
		server: &CentianServer{
			Config: &config.GlobalConfig{
				Version: "1.0.0",
				Proxy:   &config.ProxySettings{},
			},
			Logger:           logger,
			TaskVerification: taskverification.NewService(templateDir, workingDir),
		},
		config:           &config.GatewayConfig{},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamSessionPool),
	}

	session := &UpstreamSession{
		id:                    "session-1",
		identityKey:           "principal-1",
		downstreamSessionKey:  "pool-1",
		downstreamConns:       make(map[string]DownstreamConnectionInterface),
		registeredTools:       make(map[string]struct{}),
		registeredStaticTools: make(map[string]struct{}),
	}
	session.upstreamServer = endpoint.newUpstreamServer(session)
	endpoint.upstreamSessions[session.id] = session
	return endpoint, session
}

func TestNewUpstreamServerRegistersTaskVerificationTools(t *testing.T) {
	_, session := newTaskToolTestProxy(t, basicTaskTemplate())

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	assert.DeepEqual(t, listToolNames(t, clientSession), []string{
		taskCompleteOnboardingTool,
		taskCompleteStepTool,
		taskFailTool,
		taskListTemplatesTool,
		taskRegisterTool,
		taskRestartTool,
		taskStartStepTool,
	})
}

func TestTaskToolFlowAndRestartFail(t *testing.T) {
	_, session := newTaskToolTestProxy(t, basicTaskTemplate())

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	registerResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)
	assert.Assert(t, registerResult != nil)
	registerStructured := registerResult.StructuredContent.(map[string]any)
	assert.Equal(t, registerStructured["instructions"], "Use Centian validation instead of rebuilding checks manually.")
	assert.Equal(t, registerStructured["status"], string(taskverification.TaskStatusActive))
	assert.Equal(t, registerStructured["phase"], string(taskverification.TaskPhaseOnboarding))
	assert.Equal(t, registerStructured["executionReady"], false)
	assert.Equal(t, registerStructured["hasOnboarding"], false)
	_, hasSteps := registerStructured["steps"]
	assert.Assert(t, !hasSteps)

	completeOnboardingResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompleteOnboardingTool,
		Arguments: map[string]any{
			"onboarding": map[string]any{
				"projectSummary": "Small test project with one shell validation path.",
				"artifactMap": []map[string]any{
					{
						"path":  "/workspace/project/tests",
						"kind":  "tests",
						"notes": "Primary test directory",
					},
				},
				"commonCommands": []map[string]any{
					{
						"command": "python -m pytest -q",
						"purpose": "Run targeted tests",
					},
				},
				"constraints":   []string{"Use Centian tools only"},
				"openQuestions": []string{"Which file should planning target?"},
			},
		},
	})
	assert.NilError(t, err)
	completeOnboardingStructured := completeOnboardingResult.StructuredContent.(map[string]any)
	assert.Equal(t, completeOnboardingStructured["status"], string(taskverification.TaskStatusActive))
	assert.Equal(t, completeOnboardingStructured["phase"], string(taskverification.TaskPhasePlanning))
	assert.Equal(t, completeOnboardingStructured["hasOnboarding"], true)
	assert.Equal(t, completeOnboardingStructured["onboardingSummary"], "Small test project with one shell validation path.")
	assert.Assert(t, completeOnboardingStructured["onboarding"] != nil)

	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskStartStepTool,
		Arguments: map[string]any{
			"step": 1,
		},
	})
	assert.ErrorContains(t, err, "step execution is only allowed in execution phase")

	restartResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskRestartTool,
		Arguments: map[string]any{},
	})
	assert.NilError(t, err)
	restartStructured := restartResult.StructuredContent.(map[string]any)
	assert.Equal(t, restartStructured["status"], string(taskverification.TaskStatusActive))
	assert.Equal(t, restartStructured["phase"], string(taskverification.TaskPhaseOnboarding))
	assert.Equal(t, restartStructured["hasOnboarding"], true)
	assert.Equal(t, restartStructured["onboardingSummary"], "Small test project with one shell validation path.")
	assert.Equal(t, restartStructured["onboardingSummary"], "Small test project with one shell validation path.")

	failResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskFailTool,
		Arguments: map[string]any{
			"reason": "stuck",
		},
	})
	assert.NilError(t, err)
	failStructured := failResult.StructuredContent.(map[string]any)
	assert.Equal(t, failStructured["status"], string(taskverification.TaskStatusFailed))
	assert.Equal(t, failStructured["explicitFailReason"], "stuck")
	assert.Equal(t, failStructured["phase"], string(taskverification.TaskPhaseOnboarding))
}

func TestTaskRegistrationIsIsolatedPerSession(t *testing.T) {
	endpoint, sessionA := newTaskToolTestProxy(t, basicTaskTemplate())
	sessionB := &UpstreamSession{
		id:                    "session-2",
		identityKey:           "principal-2",
		downstreamSessionKey:  "pool-2",
		downstreamConns:       make(map[string]DownstreamConnectionInterface),
		registeredTools:       make(map[string]struct{}),
		registeredStaticTools: make(map[string]struct{}),
	}
	sessionB.upstreamServer = endpoint.newUpstreamServer(sessionB)
	endpoint.upstreamSessions[sessionB.id] = sessionB

	clientA, cleanupA := connectUpstreamTestClient(t, sessionA, &mcp.ClientOptions{})
	defer cleanupA()
	clientB, cleanupB := connectUpstreamTestClient(t, sessionB, &mcp.ClientOptions{})
	defer cleanupB()

	_, err := clientA.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)
	_, err = clientB.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)

	assert.Assert(t, sessionA.taskRun != nil)
	assert.Assert(t, sessionB.taskRun != nil)
	assert.Assert(t, sessionA.taskRun != sessionB.taskRun)
}

func TestTaskToolCallsAreWrittenToRequestLog(t *testing.T) {
	// Given: an upstream session with task verification tools enabled.
	endpoint, session := newTaskToolTestProxy(t, basicTaskTemplate())
	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	// When: built-in task tools are called through the upstream MCP surface.
	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskListTemplatesTool,
		Arguments: map[string]any{},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompleteOnboardingTool,
		Arguments: map[string]any{
			"onboarding": map[string]any{
				"projectSummary": "Stored summary",
			},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskStartStepTool,
		Arguments: map[string]any{
			"step": 1,
		},
	})
	assert.ErrorContains(t, err, "step execution is only allowed in execution phase")

	// Then: the request log contains the built-in task tool calls.
	entries := readTaskToolLogEntries(t, endpoint.server.Logger.GetLogPath())
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.ToolCall != nil {
			names = append(names, entry.ToolCall.Name)
		}
	}
	assert.DeepEqual(t, names, []string{
		taskListTemplatesTool,
		taskRegisterTool,
		taskCompleteOnboardingTool,
		taskStartStepTool,
	})
}

func basicTaskTemplate() string {
	return `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
  instructions: "Use Centian validation instead of rebuilding checks manually."
steps:
  - id: "step_one"
    instructions: "Start the step before using complete_step."
    checks:
      - id: "check_one"
        command: "printf 'ok'"
        pre_conditions:
          - type: stdout_contains
            value: "ok"
        post_conditions:
          - type: stdout_contains
            value: "ok"
`
}

func readTaskToolLogEntries(t *testing.T, path string) []common.LogEntry {
	t.Helper()

	data, err := os.ReadFile(path)
	assert.NilError(t, err)

	lines := bytesSplitLines(data)
	entries := make([]common.LogEntry, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var entry common.LogEntry
		err := json.Unmarshal(line, &entry)
		assert.NilError(t, err)
		entries = append(entries, entry)
	}
	return entries
}

func bytesSplitLines(data []byte) [][]byte {
	lines := make([][]byte, 0)
	start := 0
	for idx, b := range data {
		if b != '\n' {
			continue
		}
		lines = append(lines, data[start:idx])
		start = idx + 1
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
