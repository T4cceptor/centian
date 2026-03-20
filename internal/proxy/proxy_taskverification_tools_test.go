package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func newTaskToolTestProxy(t *testing.T, templateContent string) (*CentianEndpoint, *UpstreamSession) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
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

	startResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskStartStepTool,
		Arguments: map[string]any{
			"step": 1,
		},
	})
	assert.NilError(t, err)
	startStructured := startResult.StructuredContent.(map[string]any)
	assert.Equal(t, startStructured["passed"], true)

	completeResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompleteStepTool,
		Arguments: map[string]any{
			"step": 1,
		},
	})
	assert.NilError(t, err)
	completeStructured := completeResult.StructuredContent.(map[string]any)
	assert.Equal(t, completeStructured["passed"], true)
	assert.Equal(t, completeStructured["status"], string(taskverification.TaskStatusCompleted))

	restartResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskRestartTool,
		Arguments: map[string]any{},
	})
	assert.NilError(t, err)
	restartStructured := restartResult.StructuredContent.(map[string]any)
	assert.Equal(t, restartStructured["status"], string(taskverification.TaskStatusRegistered))

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

func basicTaskTemplate() string {
	return `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
steps:
  - id: "step_one"
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
