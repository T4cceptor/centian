package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	taskListTemplatesTool       = "centian.task_list_templates"
	taskRegisterTool            = "centian.task_register"
	taskCompleteOnboardingTool  = "centian.task_complete_onboarding"
	taskCompletePlanningTool    = "centian.task_complete_planning"
	taskStartStepTool           = "centian.task_start_step"
	taskCompleteStepTool        = "centian.task_complete_step"
	taskResumeTool              = "centian.task_resume"
	taskRestartTool             = "centian.task_restart"
	taskFailTool                = "centian.task_fail"
	pathModeRelativeToWorkspace = "relative_to_workspace"
)

type taskRegisterArgs struct {
	TemplateID string         `json:"templateId"`
	Parameters map[string]any `json:"parameters"`
}

type taskStepArgs struct {
	Step int `json:"step"`
}

type taskCompleteOnboardingArgs struct {
	Onboarding taskverification.OnboardingArtifact `json:"onboarding"`
}

type taskCompletePlanningArgs struct {
	Planning taskverification.PlanningArtifact `json:"planning"`
}

type taskFailArgs struct {
	Reason string `json:"reason"`
}

type taskToolHandler func(context.Context, *UpstreamSession, *mcp.CallToolRequest) (*mcp.CallToolResult, error)

func (p *CentianEndpoint) registerTaskVerificationTools(session *UpstreamSession, server *mcp.Server) {
	if session == nil || server == nil || p == nil || p.server == nil || p.server.TaskVerification == nil {
		return
	}
	if _, exists := session.registeredStaticTools[taskListTemplatesTool]; exists {
		return
	}

	server.AddTool(&mcp.Tool{
		Name:        taskListTemplatesTool,
		Description: taskToolDescription("List available task verification templates."),
		Annotations: taskReadOnlyAnnotations(),
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, p.wrapTaskToolHandler(session, taskListTemplatesTool, p.handleTaskListTemplatesTool))
	session.registeredStaticTools[taskListTemplatesTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskRegisterTool,
		Description: taskToolDescription("Register a task verification run from a template."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"templateId": map[string]any{"type": "string"},
				"parameters": map[string]any{"type": "object"},
			},
			"required": []string{"templateId", "parameters"},
		},
	}, p.wrapTaskToolHandler(session, taskRegisterTool, p.handleTaskRegisterTool))
	session.registeredStaticTools[taskRegisterTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskCompleteOnboardingTool,
		Description: taskToolDescription("Persist onboarding context and advance the task into planning."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: taskCompleteOnboardingSchema(),
	}, p.wrapTaskToolHandler(session, taskCompleteOnboardingTool, p.handleTaskCompleteOnboardingTool))
	session.registeredStaticTools[taskCompleteOnboardingTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskCompletePlanningTool,
		Description: taskToolDescription("Persist planning context, freeze the contract, and enter execution."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: taskCompletePlanningSchema(),
	}, p.wrapTaskToolHandler(session, taskCompletePlanningTool, p.handleTaskCompletePlanningTool))
	session.registeredStaticTools[taskCompletePlanningTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskStartStepTool,
		Description: taskToolDescription("Start a task step by running preconditions and capturing invariant baselines."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step": map[string]any{"type": "integer"},
			},
			"required": []string{"step"},
		},
	}, p.wrapTaskToolHandler(session, taskStartStepTool, p.handleTaskStartStepTool))
	session.registeredStaticTools[taskStartStepTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskCompleteStepTool,
		Description: taskToolDescription("Complete a task step by running postconditions and invariant checks."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step": map[string]any{"type": "integer"},
			},
			"required": []string{"step"},
		},
	}, p.wrapTaskToolHandler(session, taskCompleteStepTool, p.handleTaskCompleteStepTool))
	session.registeredStaticTools[taskCompleteStepTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskResumeTool,
		Description: taskToolDescription("Resume a timed-out task verification run without resetting workflow progress."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, p.wrapTaskToolHandler(session, taskResumeTool, p.handleTaskResumeTool))
	session.registeredStaticTools[taskResumeTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskRestartTool,
		Description: taskToolDescription("Restart the active task verification run and clear step state."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, p.wrapTaskToolHandler(session, taskRestartTool, p.handleTaskRestartTool))
	session.registeredStaticTools[taskRestartTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskFailTool,
		Description: taskToolDescription("Explicitly fail the active task verification run."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{"type": "string"},
			},
		},
	}, p.wrapTaskToolHandler(session, taskFailTool, p.handleTaskFailTool))
	session.registeredStaticTools[taskFailTool] = struct{}{}
}

func taskToolDescription(base string) string {
	return base + " Use workspaceRoot as the project root, keep file paths relative to it, and treat nextAction as the workflow hint."
}

func taskReadOnlyAnnotations() *mcp.ToolAnnotations {
	openWorld := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
		OpenWorldHint:  &openWorld,
	}
}

func taskStateTransitionAnnotations() *mcp.ToolAnnotations {
	destructive := false
	openWorld := false
	return &mcp.ToolAnnotations{
		DestructiveHint: &destructive,
		OpenWorldHint:   &openWorld,
	}
}

func (p *CentianEndpoint) wrapTaskToolHandler(
	session *UpstreamSession,
	toolName string,
	handler taskToolHandler,
) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		requestID := getNewUUIDV7()
		ctx = withTaskActionRequestID(ctx, requestID)
		session.taskMu.Lock()
		p.maybeExpireActiveTaskLocked(session, requestID)
		invocationSnapshot := snapshotTaskRun(session.taskRun)
		if invocationSnapshot.Status == taskverification.TaskStatusActive {
			p.cancelTaskTimeoutLocked(session)
		}
		session.taskMu.Unlock()
		var (
			result *mcp.CallToolResult
			err    error
		)
		if invocationSnapshot.RunID == "" && !taskToolAllowedBeforeRegistration(toolName) {
			result = taskToolRegistrationRequiredResult(toolName, p.server.TaskVerification.WorkingDir)
		} else {
			result, err = handler(ctx, session, req)
		}
		p.logTaskToolCall(session, requestID, toolName, req, result, err)
		session.taskMu.Lock()
		switch {
		case session.taskRun == nil:
			p.cancelTaskTimeoutLocked(session)
		case session.taskRun.Status == taskverification.TaskStatusActive:
			p.refreshTaskActivityLocked(session)
		default:
			p.cancelTaskTimeoutLocked(session)
			if session.taskRun.Status != taskverification.TaskStatusTimedOut {
				session.taskRun.ExpiresAt = 0
			}
		}
		currentSnapshot := snapshotTaskRun(session.taskRun)
		addTaskTimingToToolResult(result, session.taskRun)
		session.taskMu.Unlock()
		if currentSnapshot.RunID != "" {
			invocationPhase := invocationSnapshot.Phase
			invocationNodeKind := invocationSnapshot.NodeKind
			if invocationSnapshot.RunID == "" {
				invocationPhase = currentSnapshot.Phase
				invocationNodeKind = currentSnapshot.NodeKind
			}
			p.recordTaskActionContext(currentSnapshot.RunID, requestID, invocationPhase, invocationNodeKind)
		}
		return result, err
	}
}

func taskToolAllowedBeforeRegistration(toolName string) bool {
	switch toolName {
	case taskListTemplatesTool, taskRegisterTool:
		return true
	default:
		return false
	}
}

func taskToolRegistrationRequiredResult(toolName, workingDir string) *mcp.CallToolResult {
	message := fmt.Sprintf(
		"tool %q is not allowed before registering a task; call centian.task_list_templates and centian.task_register first",
		toolName,
	)
	structured := map[string]any{
		"error":              message,
		"requestedTool":      toolName,
		"reason":             governanceDeniedRegistrationNeeded,
		"allowedBeforeRun":   []string{taskListTemplatesTool, taskRegisterTool},
		"registrationNeeded": true,
		"nextAction":         "Call centian.task_list_templates, then centian.task_register.",
	}
	addWorkspaceContext(structured, workingDir)
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
		StructuredContent: structured,
	}
}

func (p *CentianEndpoint) logTaskToolCall(
	session *UpstreamSession,
	requestID string,
	toolName string,
	req *mcp.CallToolRequest,
	result *mcp.CallToolResult,
	callErr error,
) {
	if p == nil || p.server == nil || p.server.Logger == nil || session == nil {
		return
	}

	meta := common.NewRequestMetaContext(string(common.HTTPTransport)).
		WithRequestID(requestID).
		WithSessionID(session.id).
		WithServerID(p.server.ServerID)
	meta.Success = callErr == nil && (result == nil || !result.IsError)
	meta.Direction = common.DirectionClientToServer
	meta.MessageType = common.MessageTypeRequest
	if callErr != nil {
		meta.Error = callErr.Error()
		meta.Status = 500
	} else {
		meta.Status = 200
	}

	entry := &common.LogEntry{
		BaseMcpEvent: meta.BaseMcpEvent,
		Routing: common.RoutingContext{
			Transport:  common.HTTPTransport,
			Gateway:    p.name,
			ServerName: "centian",
			Endpoint:   p.endpoint,
		},
	}
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]string)
	}
	if session.identityKey != "" {
		entry.Metadata["principal_id"] = session.identityKey
	}
	entry.WithToolRequest(toolName, toolName, taskToolArguments(req))
	entry.WithToolResult(taskToolResultJSON(result, callErr), callErr != nil || (result != nil && result.IsError))
	if err := p.server.Logger.LogMcpEvent(entry); err != nil {
		common.LogWarn("ProxyEndpoint[%s]: failed to log task tool call %s for session %s: %v", p.name, toolName, session.id, err)
	}
}

func taskToolArguments(req *mcp.CallToolRequest) json.RawMessage {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return json.RawMessage(`{}`)
	}
	return req.Params.Arguments
}

func taskToolResultJSON(result *mcp.CallToolResult, callErr error) json.RawMessage {
	if callErr != nil {
		payload, _ := json.Marshal(map[string]any{"error": callErr.Error()})
		return payload
	}
	if result == nil {
		return json.RawMessage(`null`)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		fallback, _ := json.Marshal(map[string]any{"error": err.Error()})
		return fallback
	}
	return payload
}

func taskCompleteOnboardingSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"onboarding": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"taskSummary": map[string]any{"type": "string"},
					"artifactMap": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"path":  map[string]any{"type": "string"},
								"kind":  map[string]any{"type": "string"},
								"notes": map[string]any{"type": "string"},
							},
							"required": []string{"path", "kind"},
						},
					},
					"commonCommands": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"command": map[string]any{"type": "string"},
								"purpose": map[string]any{"type": "string"},
							},
							"required": []string{"command", "purpose"},
						},
					},
					"constraints": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"openQuestions": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
				"required": []string{"taskSummary"},
			},
		},
		"required": []string{"onboarding"},
	}
}

func taskCompletePlanningSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"planning": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selectedFiles": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"testTarget":           map[string]any{"type": "string"},
					"lintCommand":          map[string]any{"type": "string"},
					"expectedFailure":      map[string]any{"type": "string"},
					"implementationTarget": map[string]any{"type": "string"},
					"invariants": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
			},
		},
		"required": []string{"planning"},
	}
}

func (p *CentianEndpoint) handleTaskListTemplatesTool(_ context.Context, _ *UpstreamSession, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	templates, err := p.server.TaskVerification.ListTemplates()
	if err != nil {
		return nil, err
	}

	lines := make([]string, 0, len(templates))
	structured := make([]map[string]any, 0, len(templates))
	for _, template := range templates {
		lines = append(lines, fmt.Sprintf("%s (%d steps)", template.ID, template.StepCount))
		structured = append(structured, map[string]any{
			"id":           template.ID,
			"name":         template.Name,
			"description":  template.Description,
			"instructions": template.Instructions,
			"parameters":   template.Parameters,
			"stepCount":    template.StepCount,
			"steps":        template.Steps,
		})
	}
	if len(lines) == 0 {
		lines = append(lines, "No task templates available.")
	}
	response := map[string]any{
		"templates":  structured,
		"nextAction": "Call centian.task_register with one templateId and its parameters.",
	}
	addWorkspaceContext(response, p.server.TaskVerification.WorkingDir)
	return toolResult(strings.Join(lines, "\n"), response), nil
}

func (p *CentianEndpoint) handleTaskRegisterTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskRegisterArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.TemplateID) == "" {
		return nil, fmt.Errorf("templateId is required")
	}

	parameters := make(map[string]string, len(args.Parameters))
	for key, value := range args.Parameters {
		parameters[key] = fmt.Sprint(value)
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	if session.taskRun != nil && session.taskRun.Status == taskverification.TaskStatusActive {
		return nil, fmt.Errorf("an active task is already registered for this session")
	}

	run, err := p.server.TaskVerification.RegisterTask(args.TemplateID, parameters)
	if err != nil {
		return nil, err
	}
	session.taskRun = run
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(run)
	p.recordTaskEvent(session, run, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeRegistered, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), map[string]any{
		"draftParameters": run.DraftParameters,
	})

	structured := runStructuredContent(run, p.server.TaskVerification.WorkingDir)
	stepCount := len(run.SelectedTemplate.CompiledWorkflow.WorkflowSteps)
	structured["stepCount"] = stepCount
	return toolResult(fmt.Sprintf("Registered task %s with %d declared step(s).", run.TemplateID, stepCount), structured), nil
}

func (p *CentianEndpoint) handleTaskCompleteOnboardingTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskCompleteOnboardingArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.CompleteOnboarding(session.taskRun, &args.Onboarding); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeOnboardingCompleted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), map[string]any{
			"error": err.Error(),
		})
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeOnboardingCompleted, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), map[string]any{
		"taskSummary": args.Onboarding.TaskSummary,
	})
	structured := runStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)
	if session.taskRun.Onboarding != nil {
		structured["onboarding"] = session.taskRun.Onboarding
	}
	return toolResult("Task onboarding completed; task moved to planning.", structured), nil
}

func (p *CentianEndpoint) handleTaskCompletePlanningTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskCompletePlanningArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.CompletePlanning(session.taskRun, &args.Planning); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypePlanningCompleted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), map[string]any{
			"error": err.Error(),
		})
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypePlanningCompleted, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), map[string]any{
		"planning": args.Planning,
	})
	if node, exists := session.taskRun.CurrentNode(); exists && node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeApprovalWaitEntered, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), nil)
	}
	structured := runStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)
	if session.taskRun.Planning != nil {
		structured["planning"] = session.taskRun.Planning
	}
	return toolResult(fmt.Sprintf("Task planning completed; task moved to %s.", session.taskRun.Phase), structured), nil
}

func (p *CentianEndpoint) handleTaskStartStepTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskStepArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	result, err := p.server.TaskVerification.StartStep(ctx, session.taskRun, args.Step)
	if err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeStepStarted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), map[string]any{
			"step":  args.Step,
			"error": err.Error(),
		})
		return nil, err
	}
	outcome := taskverification.TaskEventOutcomeSucceeded
	if !result.Passed {
		outcome = taskverification.TaskEventOutcomeFailed
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeStepStarted, outcome, taskActionRequestIDFromContext(ctx), stepEventPayload(result))
	return stepToolResult(result, session.taskRun, p.server.TaskVerification.WorkingDir, taskStartStepTool), nil
}

func (p *CentianEndpoint) handleTaskCompleteStepTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskStepArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	result, err := p.server.TaskVerification.CompleteStep(ctx, session.taskRun, args.Step)
	if err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeStepCompleted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), map[string]any{
			"step":  args.Step,
			"error": err.Error(),
		})
		return nil, err
	}
	outcome := taskverification.TaskEventOutcomeSucceeded
	if !result.Passed {
		outcome = taskverification.TaskEventOutcomeFailed
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeStepCompleted, outcome, taskActionRequestIDFromContext(ctx), stepEventPayload(result))
	if result.Passed {
		if node, exists := session.taskRun.CurrentNode(); exists && node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
			p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeApprovalWaitEntered, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), nil)
		}
	}
	return stepToolResult(result, session.taskRun, p.server.TaskVerification.WorkingDir, taskCompleteStepTool), nil
}

func (p *CentianEndpoint) handleTaskResumeTool(ctx context.Context, session *UpstreamSession, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)
	if err := p.server.TaskVerification.ResumeTask(session.taskRun); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeResumed, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), map[string]any{
			"error": err.Error(),
		})
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeResumed, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), nil)
	return toolResult("Task resumed.", runStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)), nil
}

func (p *CentianEndpoint) handleTaskRestartTool(ctx context.Context, session *UpstreamSession, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.RestartTask(session.taskRun); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeRestarted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), map[string]any{
			"error": err.Error(),
		})
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeRestarted, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), nil)
	return toolResult("Task restarted.", runStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)), nil
}

func (p *CentianEndpoint) handleTaskFailTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskFailArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.FailTask(session.taskRun, args.Reason); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeFailed, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), map[string]any{
			"reason": args.Reason,
			"error":  err.Error(),
		})
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeFailed, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), map[string]any{
		"reason": args.Reason,
	})
	message := "Task failed."
	if strings.TrimSpace(args.Reason) != "" {
		message = fmt.Sprintf("Task failed: %s", strings.TrimSpace(args.Reason))
	}
	return toolResult(message, runStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)), nil
}

func decodeToolArguments(req *mcp.CallToolRequest, target any) error {
	if target == nil {
		return fmt.Errorf("target is required")
	}
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return nil
	}
	if err := json.Unmarshal(req.Params.Arguments, target); err != nil {
		return fmt.Errorf("failed to parse tool arguments: %w", err)
	}
	return nil
}

func toolResult(message string, structured map[string]any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: message}},
		StructuredContent: structured,
	}
}

func stepToolResult(result *taskverification.StepResult, run *taskverification.RunState, workingDir, actionTool string) *mcp.CallToolResult {
	structured := runStructuredContent(run, workingDir)
	structured["passed"] = result.Passed
	structured["message"] = result.Message
	structured["step"] = result.Step
	structured["stepId"] = result.StepID
	structured["status"] = string(result.Status)
	structured["phase"] = string(result.Phase)
	structured["stepStatus"] = string(result.StepStatus)
	if result.Summary != "" {
		structured["summary"] = result.Summary
	}
	if result.FailureKind != "" {
		structured["failureKind"] = string(result.FailureKind)
	}
	if result.FailurePhase != "" {
		structured["failurePhase"] = string(result.FailurePhase)
	}
	if result.FailedCheckID != "" {
		structured["failedCheckId"] = result.FailedCheckID
	}
	if result.FailedInvariantID != "" {
		structured["failedInvariantId"] = result.FailedInvariantID
	}
	if result.ExitCode != nil {
		structured["exitCode"] = *result.ExitCode
	}
	if result.StdoutSnippet != "" {
		structured["stdoutSnippet"] = result.StdoutSnippet
	}
	if result.StderrSnippet != "" {
		structured["stderrSnippet"] = result.StderrSnippet
	}
	structured["nextAction"] = nextActionForStepResult(result, run, actionTool)
	return toolResult(result.Message, structured)
}

func runStructuredContent(run *taskverification.RunState, workingDir string) map[string]any {
	if run == nil {
		structured := map[string]any{}
		addWorkspaceContext(structured, workingDir)
		return structured
	}

	structured := map[string]any{
		"taskRunId":          run.RunID,
		"templateId":         run.TemplateID,
		"templateName":       run.SelectedTemplate.Task.Name,
		"description":        run.SelectedTemplate.Task.Description,
		"instructions":       run.SelectedTemplate.Task.Instructions,
		"status":             string(run.Status),
		"phase":              string(run.Phase),
		"draftParameters":    run.DraftParameters,
		"hasOnboarding":      run.Onboarding != nil,
		"hasPlanning":        run.Planning != nil,
		"executionReady":     run.WorkflowReady,
		"stepCount":          len(run.SelectedTemplate.CompiledWorkflow.WorkflowSteps),
		"lastFailureMessage": run.LastFailureMessage,
		"explicitFailReason": run.ExplicitFailReason,
	}
	addTaskTimingFields(structured, run)
	addWorkspaceContext(structured, workingDir)
	addTaskContracts(structured)
	addCurrentNodeContext(structured, run)
	addPlanningNodeContext(structured, run)
	addArtifactSummaries(structured, run)
	structured["nextAction"] = nextActionForRun(run)

	if !run.WorkflowReady || run.RunnableTemplate == nil {
		return structured
	}

	structured["steps"] = workflowStepsSummary(run)
	return structured
}

func addWorkspaceContext(structured map[string]any, workingDir string) {
	if strings.TrimSpace(workingDir) == "" {
		return
	}
	structured["workspaceRoot"] = workingDir
	structured["pathMode"] = pathModeRelativeToWorkspace
	structured["commandWorkingDirectory"] = workingDir
}

func addTaskContracts(structured map[string]any) {
	structured["onboardingContract"] = onboardingContract()
	structured["planningContract"] = planningContract()
	structured["shellCommandHint"] = "For compound shell commands or directory changes, use bash -lc '...'."
}

func nextActionForRun(run *taskverification.RunState) string {
	if run == nil {
		return ""
	}
	switch run.Status {
	case taskverification.TaskStatusCompleted:
		return "Task is complete; stop task work."
	case taskverification.TaskStatusFailed:
		return "Restart the task or register a new task run."
	case taskverification.TaskStatusTimedOut:
		return "Call centian.task_resume or centian.task_restart."
	}

	node, exists := run.CurrentNode()
	if !exists {
		if run.Phase == taskverification.TaskPhaseInitialization {
			return "Call centian.task_list_templates, then centian.task_register."
		}
		return ""
	}

	switch node.Kind {
	case taskverification.WorkflowNodeKindOnboarding:
		return "Call centian.task_complete_onboarding to freeze the onboarding context."
	case taskverification.WorkflowNodeKindPlanning:
		return "Call centian.task_complete_planning to freeze the execution contract."
	case taskverification.WorkflowNodeKindWaitingForApproval:
		return "Wait for approval before continuing task work."
	case taskverification.WorkflowNodeKindScaffolding, taskverification.WorkflowNodeKindExecution:
		stepNumber, active := activeOrCurrentStep(run)
		if stepNumber == 0 {
			return ""
		}
		if active {
			return fmt.Sprintf("Do the step work in workspaceRoot, then call centian.task_complete_step for step %d.", stepNumber)
		}
		return fmt.Sprintf("Call centian.task_start_step for step %d.", stepNumber)
	default:
		return ""
	}
}

func nextActionForStepResult(result *taskverification.StepResult, run *taskverification.RunState, actionTool string) string {
	if result == nil {
		return nextActionForRun(run)
	}
	if result.Passed {
		return nextActionForRun(run)
	}
	retryTool := actionTool
	if retryTool == "" {
		retryTool = taskCompleteStepTool
	}
	return fmt.Sprintf("Fix the failed check in workspaceRoot, then retry %s for step %d.", retryTool, result.Step)
}

func activeOrCurrentStep(run *taskverification.RunState) (int, bool) {
	if run == nil || run.RunnableTemplate == nil || run.RunnableTemplate.CompiledWorkflow == nil {
		return 0, false
	}
	for idx := range run.Steps {
		if run.Steps[idx].Status == taskverification.StepStatusActive {
			return idx + 1, true
		}
	}
	for idx := range run.RunnableTemplate.CompiledWorkflow.WorkflowSteps {
		if run.RunnableTemplate.CompiledWorkflow.WorkflowSteps[idx].Path == run.Phase {
			return idx + 1, false
		}
	}
	return 0, false
}

func addCurrentNodeContext(structured map[string]any, run *taskverification.RunState) {
	node, exists := run.CurrentNode()
	if !exists {
		return
	}
	structured["currentNodeKind"] = string(node.Kind)
	structured["approvalBlocked"] = node.Kind == taskverification.WorkflowNodeKindWaitingForApproval
	structured["allowedTools"] = append([]string{}, node.AllowedTools...)
	if len(node.EditableFields) > 0 {
		structured["planningEditableFields"] = append([]string{}, node.EditableFields...)
	}
	if len(node.RequiredPlanningOutputs) > 0 {
		structured["planningRequiredOutputs"] = append([]string{}, node.RequiredPlanningOutputs...)
	}
	if node.NextPath != "" {
		structured["nextNodePath"] = string(node.NextPath)
	}
}

func addPlanningNodeContext(structured map[string]any, run *taskverification.RunState) {
	if run.SelectedTemplate.CompiledWorkflow == nil {
		return
	}
	planningNode, exists := run.SelectedTemplate.CompiledWorkflow.Nodes[run.SelectedTemplate.CompiledWorkflow.PlanningPath]
	if !exists {
		return
	}
	if _, present := structured["planningEditableFields"]; !present && len(planningNode.EditableFields) > 0 {
		structured["planningEditableFields"] = append([]string{}, planningNode.EditableFields...)
	}
	if _, present := structured["planningRequiredOutputs"]; !present && len(planningNode.RequiredPlanningOutputs) > 0 {
		structured["planningRequiredOutputs"] = append([]string{}, planningNode.RequiredPlanningOutputs...)
	}
}

func addArtifactSummaries(structured map[string]any, run *taskverification.RunState) {
	if run.Onboarding != nil {
		structured["taskSummary"] = run.Onboarding.TaskSummary
	}
	if run.Planning == nil {
		return
	}
	structured["planningSummary"] = map[string]any{
		"selectedFiles":        run.Planning.SelectedFiles,
		"testTarget":           run.Planning.TestTarget,
		"lintCommand":          run.Planning.LintCommand,
		"implementationTarget": run.Planning.ImplementationTarget,
	}
	structured["frozenContractSummary"] = frozenContractSummary(run)
}

func workflowStepsSummary(run *taskverification.RunState) []map[string]any {
	steps := make([]map[string]any, 0, len(run.Steps))
	for index, step := range run.Steps {
		templateStep := run.RunnableTemplate.CompiledWorkflow.WorkflowSteps[index]
		steps = append(steps, map[string]any{
			"step":         index + 1,
			"id":           step.ID,
			"path":         string(step.Path),
			"name":         templateStep.Name,
			"description":  templateStep.Description,
			"instructions": templateStep.Instructions,
			"status":       step.Status,
		})
	}
	sort.Slice(steps, func(i, j int) bool {
		return steps[i]["step"].(int) < steps[j]["step"].(int)
	})
	return steps
}

func onboardingContract() map[string]any {
	return map[string]any{
		"requiredFields": []string{"taskSummary"},
		"artifactMapItem": map[string]any{
			"requiredFields": []string{"path", "kind"},
			"optionalFields": []string{"notes"},
		},
		"commonCommandsItem": map[string]any{
			"requiredFields": []string{"command", "purpose"},
		},
		"optionalFields": []string{"artifactMap", "commonCommands", "constraints", "openQuestions"},
	}
}

func addTaskTimingFields(structured map[string]any, run *taskverification.RunState) {
	if structured == nil || run == nil {
		return
	}
	if run.LastActivityAt > 0 {
		structured["lastActivityAtUnixMilli"] = run.LastActivityAt
	}
	if run.ExpiresAt > 0 {
		structured["expiresAtUnixMilli"] = run.ExpiresAt
	}
}

func addTaskTimingToToolResult(result *mcp.CallToolResult, run *taskverification.RunState) {
	if result == nil {
		return
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		return
	}
	addTaskTimingFields(structured, run)
}

func planningContract() map[string]any {
	return map[string]any{
		"supportedFields": []string{
			"selectedFiles",
			"testTarget",
			"lintCommand",
			"expectedFailure",
			"implementationTarget",
			"invariants",
		},
	}
}

func frozenContractSummary(run *taskverification.RunState) map[string]any {
	summary := map[string]any{
		"selectedFiles":        []string{},
		"testTarget":           "",
		"implementationTarget": "",
		"invariantCount":       0,
	}
	if run == nil || run.Planning == nil {
		return summary
	}

	summary["selectedFiles"] = append([]string{}, run.Planning.SelectedFiles...)
	summary["testTarget"] = run.Planning.TestTarget
	summary["implementationTarget"] = run.Planning.ImplementationTarget
	if strings.TrimSpace(run.Planning.LintCommand) != "" {
		summary["lintCommand"] = run.Planning.LintCommand
	}
	if run.RunnableTemplate != nil && run.RunnableTemplate.CompiledWorkflow != nil {
		invariantCount := 0
		for idx := range run.RunnableTemplate.CompiledWorkflow.WorkflowSteps {
			step := &run.RunnableTemplate.CompiledWorkflow.WorkflowSteps[idx]
			invariantCount += len(step.Invariants)
		}
		summary["invariantCount"] = invariantCount
	}
	return summary
}
