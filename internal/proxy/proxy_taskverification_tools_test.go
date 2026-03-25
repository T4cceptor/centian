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
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func newTaskToolTestProxy(t *testing.T, templateContent string) (*CentianEndpoint, *UpstreamSession) {
	return newTaskToolTestProxyWithEnabled(t, templateContent, true)
}

func newTaskToolTestProxyWithEnabled(t *testing.T, templateContent string, enabled bool) (*CentianEndpoint, *UpstreamSession) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	taskVerificationEnabled := enabled
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
				Proxy: &config.ProxySettings{
					Capabilities: &config.CapabilitiesSettings{
						TaskVerification: &config.TaskVerificationCapabilitySettings{
							Enabled: &taskVerificationEnabled,
						},
					},
				},
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
	err = endpoint.initEventProcessor()
	assert.NilError(t, err)
	return endpoint, session
}

func TestNewUpstreamServerDoesNotRegisterTaskVerificationToolsByDefault(t *testing.T) {
	_, session := newTaskToolTestProxyWithEnabled(t, basicTaskTemplate(), false)

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	assert.DeepEqual(t, listToolNames(t, clientSession), []string{})
}

func newPersistentTaskToolTestProxy(t *testing.T, templateContent string) (*CentianEndpoint, *UpstreamSession, *persistence.Store) {
	t.Helper()

	endpoint, session := newTaskToolTestProxy(t, templateContent)
	store, err := persistence.NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})
	endpoint.server.TaskVerification.EventStore = store
	endpoint.server.Logger.SetActionEventStore(store)
	return endpoint, session, store
}

func attachTaskToolDownstream(
	t *testing.T,
	endpoint *CentianEndpoint,
	session *UpstreamSession,
	toolName string,
) *MockDownstreamConnection {
	t.Helper()

	conn := &MockDownstreamConnection{
		serverName: "server-a",
		Status:     StatusConnected,
		cfg:        &config.MCPServerConfig{URL: "http://test"},
		tools: []*mcp.Tool{
			{
				Name:        toolName,
				Description: "test tool",
				InputSchema: map[string]any{"type": "object"},
			},
		},
		ResultToReturn: &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		},
	}
	session.downstreamConns["server-a"] = conn
	endpoint.syncAvailableTools(session)
	return conn
}

func TestNewUpstreamServerRegistersTaskVerificationTools(t *testing.T) {
	_, session := newTaskToolTestProxy(t, basicTaskTemplate())

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	assert.DeepEqual(t, listToolNames(t, clientSession), []string{
		taskCompleteOnboardingTool,
		taskCompletePlanningTool,
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
	assert.Assert(t, registerStructured["taskRunId"] != "")
	assert.Equal(t, registerStructured["status"], string(taskverification.TaskStatusActive))
	assert.Equal(t, registerStructured["phase"], string(taskverification.TaskPhaseOnboarding))
	assert.Equal(t, registerStructured["currentNodeKind"], string(taskverification.WorkflowNodeKindOnboarding))
	assert.Equal(t, registerStructured["nextNodePath"], string(taskverification.TaskPhasePlanning))
	assert.Equal(t, registerStructured["approvalBlocked"], false)
	assert.DeepEqual(t, registerStructured["allowedTools"], []any{"shell__*", "filesystem__*"})
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
	assert.Equal(t, completeOnboardingStructured["currentNodeKind"], string(taskverification.WorkflowNodeKindPlanning))
	assert.Equal(t, completeOnboardingStructured["nextNodePath"], "execution.step_one")
	assert.Equal(t, completeOnboardingStructured["hasOnboarding"], true)
	assert.Equal(t, completeOnboardingStructured["onboardingSummary"], "Small test project with one shell validation path.")
	assert.DeepEqual(t, completeOnboardingStructured["allowedTools"], []any{"shell__*", "filesystem__*"})
	assert.Assert(t, completeOnboardingStructured["onboarding"] != nil)
	assert.Equal(t, completeOnboardingStructured["hasPlanning"], false)

	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskStartStepTool,
		Arguments: map[string]any{
			"step": 1,
		},
	})
	assert.ErrorContains(t, err, "step execution is only allowed in execution nodes")

	completePlanningResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{
				"selectedFiles":        []string{"tests/test_mathlib.py"},
				"testTarget":           "python -m pytest -q tests/test_mathlib.py",
				"lintCommand":          "ruff check .",
				"implementationTarget": "mathlib.add",
			},
		},
	})
	assert.NilError(t, err)
	completePlanningStructured := completePlanningResult.StructuredContent.(map[string]any)
	assert.Equal(t, completePlanningStructured["status"], string(taskverification.TaskStatusActive))
	assert.Equal(t, completePlanningStructured["phase"], "execution.step_one")
	assert.Equal(t, completePlanningStructured["currentNodeKind"], string(taskverification.WorkflowNodeKindExecution))
	assert.Equal(t, completePlanningStructured["approvalBlocked"], false)
	assert.Equal(t, completePlanningStructured["hasPlanning"], true)
	assert.DeepEqual(t, completePlanningStructured["allowedTools"], []any{"shell__*", "filesystem__*"})
	assert.Equal(t, completePlanningStructured["executionReady"], true)
	assert.Assert(t, completePlanningStructured["planningSummary"] != nil)
	assert.DeepEqual(t, completePlanningStructured["frozenContractSummary"], map[string]any{
		"selectedFiles":        []any{"tests/test_mathlib.py"},
		"testTarget":           "python -m pytest -q tests/test_mathlib.py",
		"implementationTarget": "mathlib.add",
		"lintCommand":          "ruff check .",
		"invariantCount":       float64(0),
	})
	assert.Assert(t, completePlanningStructured["steps"] != nil)

	startStepResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskStartStepTool,
		Arguments: map[string]any{
			"step": 1,
		},
	})
	assert.NilError(t, err)
	startStepStructured := startStepResult.StructuredContent.(map[string]any)
	assert.Equal(t, startStepStructured["phase"], "execution.step_one")
	assert.Equal(t, startStepStructured["stepStatus"], string(taskverification.StepStatusActive))
	assert.Equal(t, startStepStructured["summary"], "step 1 (step_one) started")
	_, hasFailureKind := startStepStructured["failureKind"]
	assert.Assert(t, !hasFailureKind)

	restartResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskRestartTool,
		Arguments: map[string]any{},
	})
	assert.NilError(t, err)
	restartStructured := restartResult.StructuredContent.(map[string]any)
	assert.Equal(t, restartStructured["status"], string(taskverification.TaskStatusActive))
	assert.Equal(t, restartStructured["phase"], string(taskverification.TaskPhaseOnboarding))
	assert.Equal(t, restartStructured["currentNodeKind"], string(taskverification.WorkflowNodeKindOnboarding))
	assert.Equal(t, restartStructured["hasOnboarding"], true)
	assert.Equal(t, restartStructured["hasPlanning"], false)
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

func TestTaskLifecycleEventsRecorded(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, basicTaskTemplate())

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
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
			"onboarding": map[string]any{"projectSummary": "Stored summary"},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{"testTarget": "pytest -q"},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskStartStepTool,
		Arguments: map[string]any{"step": 1},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskCompleteStepTool,
		Arguments: map[string]any{"step": 1},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskRestartTool,
		Arguments: map[string]any{},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskFailTool,
		Arguments: map[string]any{"reason": "stop"},
	})
	assert.NilError(t, err)

	events := endpoint.server.TaskVerification.TaskEvents()
	assert.Equal(t, len(events), 7)
	assert.DeepEqual(t, []taskverification.TaskEventType{
		events[0].EventType,
		events[1].EventType,
		events[2].EventType,
		events[3].EventType,
		events[4].EventType,
		events[5].EventType,
		events[6].EventType,
	}, []taskverification.TaskEventType{
		taskverification.TaskEventTypeRegistered,
		taskverification.TaskEventTypeOnboardingCompleted,
		taskverification.TaskEventTypePlanningCompleted,
		taskverification.TaskEventTypeStepStarted,
		taskverification.TaskEventTypeStepCompleted,
		taskverification.TaskEventTypeRestarted,
		taskverification.TaskEventTypeFailed,
	})
	for _, event := range events {
		assert.Assert(t, event.TaskRunID != "")
		assert.Equal(t, event.TemplateID, "task")
		assert.Equal(t, event.PrincipalID, "principal-1")
		assert.Assert(t, event.RelatedActionEventID != "")
	}
}

func TestTaskCompletePlanningEmitsApprovalWaitEvent(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding:
    tools_allowed: ["shell__*"]
  planning:
    tools_allowed: ["shell__*"]
    required_outputs: ["testTarget"]
    next: "waiting_for_approval.review_plan"
  execution:
    - id: "review_plan"
      kind: "waiting_for_approval"
      tools_allowed: ["shell__*"]
      next: "execution.step_one"
    - id: "step_one"
      tools_allowed: ["shell__*"]
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
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
			"onboarding": map[string]any{"projectSummary": "Stored summary"},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{"testTarget": "pytest -q"},
		},
	})
	assert.NilError(t, err)

	events := endpoint.server.TaskVerification.TaskEvents()
	assert.Equal(t, events[len(events)-2].EventType, taskverification.TaskEventTypePlanningCompleted)
	assert.Equal(t, events[len(events)-1].EventType, taskverification.TaskEventTypeApprovalWaitEntered)
	assert.Equal(t, events[len(events)-1].PhasePath, taskverification.TaskPhase("waiting_for_approval.review_plan"))
}

func TestActionEventTaskContextCreatedForBuiltInTaskTools(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, basicTaskTemplate())

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
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
			"onboarding": map[string]any{"projectSummary": "Stored summary"},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{"testTarget": "pytest -q"},
		},
	})
	assert.NilError(t, err)

	contexts := endpoint.server.TaskVerification.ActionEventTaskContexts()
	assert.Equal(t, len(contexts), 3)
	assert.Equal(t, contexts[0].PhasePath, taskverification.TaskPhaseOnboarding)
	assert.Equal(t, contexts[1].PhasePath, taskverification.TaskPhasePlanning)
	assert.Equal(t, contexts[2].PhasePath, taskverification.TaskPhase("execution.step_one"))
	for _, context := range contexts {
		assert.Assert(t, context.ActionEventID != "")
		assert.Assert(t, context.TaskRunID != "")
	}
}

func TestProxiedActionCreatesTaskContextOnlyWhenActiveTaskRunExists(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, basicTaskTemplate())
	attachTaskToolDownstream(t, endpoint, session, "shell__exec")

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "shell__exec",
		Arguments: map[string]any{"command": "pwd"},
	})
	assert.NilError(t, err)
	assert.Equal(t, len(endpoint.server.TaskVerification.ActionEventTaskContexts()), 0)

	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)
	before := len(endpoint.server.TaskVerification.ActionEventTaskContexts())

	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "shell__exec",
		Arguments: map[string]any{"command": "pwd"},
	})
	assert.NilError(t, err)

	contexts := endpoint.server.TaskVerification.ActionEventTaskContexts()
	assert.Equal(t, len(contexts), before+1)
	assert.Equal(t, contexts[len(contexts)-1].PhasePath, taskverification.TaskPhaseOnboarding)
}

func TestTaskToolCallsPersistToSQLiteActionAndTaskStores(t *testing.T) {
	_, session, store := newPersistentTaskToolTestProxy(t, basicTaskTemplate())

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: taskListTemplatesTool})
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
			"onboarding": map[string]any{"projectSummary": "Stored summary"},
		},
	})
	assert.NilError(t, err)

	actionEvents := store.ActionEvents()
	assert.Assert(t, len(actionEvents) >= 3)
	assert.Equal(t, actionEvents[0].ToolName, taskListTemplatesTool)
	assert.Equal(t, actionEvents[1].ToolName, taskRegisterTool)
	assert.Equal(t, actionEvents[1].PrincipalID, "principal-1")

	taskEvents := store.TaskEvents()
	assert.Equal(t, len(taskEvents), 2)
	assert.Equal(t, taskEvents[0].EventType, taskverification.TaskEventTypeRegistered)
	assert.Equal(t, taskEvents[1].EventType, taskverification.TaskEventTypeOnboardingCompleted)

	contexts := store.ActionEventTaskContexts()
	assert.Equal(t, len(contexts), 2)
	assert.Equal(t, contexts[0].ActionEventID, actionEvents[1].ID)
	assert.Equal(t, contexts[0].TaskRunID, taskEvents[0].TaskRunID)
	assert.Equal(t, contexts[1].PhasePath, taskverification.TaskPhasePlanning)
}

func TestProxiedToolCallsPersistToSQLiteActionStoreAndContext(t *testing.T) {
	endpoint, session, store := newPersistentTaskToolTestProxy(t, basicTaskTemplate())
	attachTaskToolDownstream(t, endpoint, session, "shell__exec")

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "shell__exec",
		Arguments: map[string]any{"command": "pwd"},
	})
	assert.NilError(t, err)

	actionEvents := store.ActionEvents()
	assert.Assert(t, len(actionEvents) >= 1)
	foundShell := false
	knownActionIDs := make(map[string]struct{}, len(actionEvents))
	for _, actionEvent := range actionEvents {
		knownActionIDs[actionEvent.ID] = struct{}{}
		if actionEvent.ToolName == "shell__exec" {
			foundShell = true
			assert.Assert(t, actionEvent.OriginalToolName != "")
			assert.Equal(t, actionEvent.PrincipalID, "principal-1")
		}
	}
	assert.Assert(t, foundShell)

	contexts := store.ActionEventTaskContexts()
	assert.Assert(t, len(contexts) >= 2)
	_, exists := knownActionIDs[contexts[len(contexts)-1].ActionEventID]
	assert.Assert(t, exists)
	assert.Equal(t, contexts[len(contexts)-1].PhasePath, taskverification.TaskPhaseOnboarding)
}

func TestTaskCompletePlanningCanEnterApprovalWait(t *testing.T) {
	_, session := newTaskToolTestProxy(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding:
    tools_allowed: ["shell__*"]
  planning:
    tools_allowed: ["shell__*"]
    required_outputs: ["testTarget"]
    next: "waiting_for_approval.review_plan"
  execution:
    - id: "review_plan"
      kind: "waiting_for_approval"
      tools_allowed: ["shell__*"]
      next: "execution.step_one"
    - id: "step_one"
      tools_allowed: ["shell__*"]
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
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

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{
				"testTarget": "pytest -q",
			},
		},
	})
	assert.NilError(t, err)

	structured := result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["phase"], "waiting_for_approval.review_plan")
	assert.Equal(t, structured["currentNodeKind"], string(taskverification.WorkflowNodeKindWaitingForApproval))
	assert.Equal(t, structured["approvalBlocked"], true)
	assert.DeepEqual(t, structured["allowedTools"], []any{"shell__*"})
	assert.Equal(t, structured["nextNodePath"], "execution.step_one")

	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskStartStepTool,
		Arguments: map[string]any{
			"step": 1,
		},
	})
	assert.ErrorContains(t, err, "step execution is only allowed in execution nodes")
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

func TestTaskCompleteStepReturnsStructuredDiagnosticsOnFailure(t *testing.T) {
	_, session := newTaskToolTestProxy(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding:
    tools_allowed: ["shell__*", "filesystem__*"]
  planning:
    tools_allowed: ["shell__*", "filesystem__*"]
    required_outputs: ["testTarget"]
  execution:
    - id: "step_one"
      tools_allowed: ["shell__*", "filesystem__*"]
      checks:
        - id: "check_one"
          command: "printf 'unexpected output for verification'"
          pre_conditions:
            - type: stdout_contains
              value: "unexpected"
          post_conditions:
            - type: stdout_contains
              value: "missing"
`)

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
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
			"onboarding": map[string]any{"projectSummary": "Stored summary"},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{"testTarget": "pytest -q"},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskStartStepTool,
		Arguments: map[string]any{"step": 1},
	})
	assert.NilError(t, err)

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskCompleteStepTool,
		Arguments: map[string]any{"step": 1},
	})
	assert.NilError(t, err)
	assert.Assert(t, result.IsError == false)
	structured := result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["passed"], false)
	assert.Equal(t, structured["failureKind"], string(taskverification.StepFailureKindCheck))
	assert.Equal(t, structured["failurePhase"], string(taskverification.StepFailurePhasePostcondition))
	assert.Equal(t, structured["failedCheckId"], "check_one")
	assert.Assert(t, structured["stdoutSnippet"] != nil)
	assert.Assert(t, structured["summary"] != nil)
	assert.Assert(t, structured["frozenContractSummary"] != nil)
}

func TestWorkflowNodeToolGovernanceAllowsMatchingTool(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, basicTaskTemplate())
	downstream := attachTaskToolDownstream(t, endpoint, session, "shell__exec")

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "shell__exec",
		Arguments: map[string]any{"command": "pwd"},
	})
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Assert(t, !result.IsError)
	assert.Equal(t, downstream.CapturedToolName, "shell__exec")
}

func TestWorkflowNodeToolGovernanceDeniesUnmatchedTool(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, basicTaskTemplate())
	downstream := attachTaskToolDownstream(t, endpoint, session, "database__query")

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "database__query",
		Arguments: map[string]any{"sql": "select 1"},
	})
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Assert(t, result.IsError)
	structured := result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["reason"], governanceDeniedNoPatternMatch)
	assert.Equal(t, structured["phase"], string(taskverification.TaskPhaseOnboarding))
	assert.Equal(t, structured["currentNodeKind"], string(taskverification.WorkflowNodeKindOnboarding))
	assert.Equal(t, structured["requestedTool"], "database__query")
	assert.DeepEqual(t, structured["allowedTools"], []any{"shell__*", "filesystem__*"})
	assert.Assert(t, downstream.CapturedRequest == nil)
}

func TestWorkflowNodeToolGovernanceDeniesWithoutAllowlist(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_outputs: ["testTarget"]
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)
	downstream := attachTaskToolDownstream(t, endpoint, session, "shell__exec")

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "shell__exec",
		Arguments: map[string]any{"command": "pwd"},
	})
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Assert(t, result.IsError)
	structured := result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["reason"], governanceDeniedNoAllowlist)
	assert.DeepEqual(t, structured["allowedTools"], []any{})
	assert.Assert(t, downstream.CapturedRequest == nil)
}

func TestWorkflowNodeToolGovernanceMatchesAggregatedCanonicalToolName(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, basicTaskTemplate())
	endpoint.isAggregatedProxy = true
	downstream := attachTaskToolDownstream(t, endpoint, session, "shell__exec")

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskRegisterTool,
		Arguments: map[string]any{
			"templateId": "task",
			"parameters": map[string]any{},
		},
	})
	assert.NilError(t, err)

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "server-a___shell__exec",
		Arguments: map[string]any{"command": "pwd"},
	})
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Assert(t, !result.IsError)
	assert.Equal(t, downstream.CapturedToolName, "shell__exec")
}

func TestWorkflowNodeToolGovernanceBlocksWaitingForApprovalButKeepsTaskToolsCallable(t *testing.T) {
	endpoint, session := newTaskToolTestProxy(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding:
    tools_allowed: ["shell__*"]
  planning:
    tools_allowed: ["shell__*"]
    required_outputs: ["testTarget"]
    next: "waiting_for_approval.review_plan"
  execution:
    - id: "review_plan"
      kind: "waiting_for_approval"
      tools_allowed: ["shell__*"]
      next: "execution.step_one"
    - id: "step_one"
      tools_allowed: ["shell__*"]
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)
	downstream := attachTaskToolDownstream(t, endpoint, session, "shell__exec")

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
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
			"onboarding": map[string]any{"projectSummary": "Stored summary"},
		},
	})
	assert.NilError(t, err)
	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{"testTarget": "pytest -q"},
		},
	})
	assert.NilError(t, err)

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "shell__exec",
		Arguments: map[string]any{"command": "pwd"},
	})
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Assert(t, result.IsError)
	structured := result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["reason"], governanceDeniedWaitingForApproval)
	assert.Equal(t, structured["phase"], "waiting_for_approval.review_plan")
	assert.Assert(t, downstream.CapturedRequest == nil)

	restartResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      taskRestartTool,
		Arguments: map[string]any{},
	})
	assert.NilError(t, err)
	assert.Assert(t, restartResult != nil)
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
		Name: taskCompletePlanningTool,
		Arguments: map[string]any{
			"planning": map[string]any{
				"testTarget": "pytest -q",
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
	assert.NilError(t, err)

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
		taskCompletePlanningTool,
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
workflow:
  onboarding:
    tools_allowed: ["shell__*", "filesystem__*"]
  planning:
    tools_allowed: ["shell__*", "filesystem__*"]
    required_outputs: ["testTarget"]
  execution:
    - id: "step_one"
      tools_allowed: ["shell__*", "filesystem__*"]
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
