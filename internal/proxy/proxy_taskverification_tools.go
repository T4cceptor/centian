package proxy

import (
	"context"
	"encoding/json"
	"errors"
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
	TemplateID       string `json:"templateId"`
	TaskDescription  string `json:"task_description,omitempty"`
	TaskToolMetadata        // shared optional metadata for Centian task tools.
}

type taskStepArgs struct {
	Step int `json:"step"`
	TaskToolMetadata
}

type taskCompleteOnboardingArgs struct {
	Onboarding taskverification.OnboardingArtifact `json:"onboarding"`
	TaskToolMetadata
}

type taskCompletePlanningArgs struct {
	Planning taskverification.PlanningArtifact `json:"planning"`
	TaskToolMetadata
}

type taskFailArgs struct {
	Reason string `json:"reason"`
	TaskToolMetadata
}

type taskMetadataArgs struct {
	TaskToolMetadata
}

// TaskToolMetadata contains optional metadata accepted by all Centian task tools.
type TaskToolMetadata struct {
	Annotations []any `json:"annotations,omitempty"`
}

type taskToolHandler func(context.Context, *UpstreamSession, *mcp.CallToolRequest) (*mcp.CallToolResult, error)

func (p *CentianEndpoint) registerTaskVerificationTools(session *UpstreamSession, server *mcp.Server) {
	if session == nil || server == nil || p == nil || p.server == nil || p.server.TaskVerification == nil {
		return
	}
	if _, exists := session.registeredStaticTools[taskListTemplatesTool]; exists {
		return
	}

	addTaskTool := func(tool *mcp.Tool, name string, handler taskToolHandler) {
		applyConfiguredToolHintOverrides(tool, p.config)
		server.AddTool(tool, p.wrapTaskToolHandler(session, name, handler))
		session.registeredStaticTools[name] = struct{}{}
	}

	addTaskTool(&mcp.Tool{
		Name:        taskListTemplatesTool,
		Description: taskToolDescription("List available task verification templates."),
		Annotations: taskReadOnlyAnnotations(),
		InputSchema: withTaskToolAnnotationsSchema(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
	}, taskListTemplatesTool, p.handleTaskListTemplatesTool)

	addTaskTool(&mcp.Tool{
		Name:        taskRegisterTool,
		Description: taskToolDescription("Register a task verification run from a template."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: withTaskToolAnnotationsSchema(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"templateId": map[string]any{"type": "string"},
				"task_description": map[string]any{
					"type":        "string",
					"description": "Human-facing description of what this specific task run is trying to achieve.",
				},
			},
			"required": []string{"templateId"},
		}),
	}, taskRegisterTool, p.handleTaskRegisterTool)

	addTaskTool(&mcp.Tool{
		Name:        taskCompleteOnboardingTool,
		Description: taskToolDescription("Persist onboarding context and advance the task into planning."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: taskCompleteOnboardingSchema(),
	}, taskCompleteOnboardingTool, p.handleTaskCompleteOnboardingTool)

	addTaskTool(&mcp.Tool{
		Name:        taskCompletePlanningTool,
		Description: taskToolDescription("Persist planning context, freeze the execution contract, and enter execution. planning.parameters must contain every required planning parameter before execution can begin, and Centian enforces that contract."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: taskCompletePlanningSchema(),
	}, taskCompletePlanningTool, p.handleTaskCompletePlanningTool)

	addTaskTool(&mcp.Tool{
		Name:        taskStartStepTool,
		Description: taskToolDescription("Start a task step by running preconditions and capturing invariant baselines."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: withTaskToolAnnotationsSchema(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step": map[string]any{"type": "integer"},
			},
			"required": []string{"step"},
		}),
	}, taskStartStepTool, p.handleTaskStartStepTool)

	addTaskTool(&mcp.Tool{
		Name:        taskCompleteStepTool,
		Description: taskToolDescription("Complete a task step by running postconditions and invariant checks."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: withTaskToolAnnotationsSchema(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step": map[string]any{"type": "integer"},
			},
			"required": []string{"step"},
		}),
	}, taskCompleteStepTool, p.handleTaskCompleteStepTool)

	addTaskTool(&mcp.Tool{
		Name:        taskResumeTool,
		Description: taskToolDescription("Resume a timed-out task verification run without resetting workflow progress."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: withTaskToolAnnotationsSchema(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
	}, taskResumeTool, p.handleTaskResumeTool)

	addTaskTool(&mcp.Tool{
		Name:        taskRestartTool,
		Description: taskToolDescription("Restart the active task verification run and clear step state."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: withTaskToolAnnotationsSchema(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
	}, taskRestartTool, p.handleTaskRestartTool)

	addTaskTool(&mcp.Tool{
		Name:        taskFailTool,
		Description: taskToolDescription("Explicitly fail the active task verification run."),
		Annotations: taskStateTransitionAnnotations(),
		InputSchema: withTaskToolAnnotationsSchema(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{"type": "string"},
			},
		}),
	}, taskFailTool, p.handleTaskFailTool)
}

func taskToolDescription(base string) string {
	return base + " Use workspaceRoot as the project root, keep file paths relative to it, and treat nextAction as the workflow hint."
}

func withTaskToolAnnotationsSchema(schema map[string]any) map[string]any {
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		schema["properties"] = properties
	}
	properties["annotations"] = map[string]any{
		"type":        "array",
		"description": "Optional annotations that provide additional context for this Centian tool call. These are persisted on related task lifecycle events when one is emitted.",
		"items": map[string]any{
			"oneOf": []any{
				map[string]any{"type": "string"},
				map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
		},
	}
	return schema
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
	return withTaskToolAnnotationsSchema(map[string]any{
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
	})
}

func taskCompletePlanningSchema() map[string]any {
	return withTaskToolAnnotationsSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"planning": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"planSummary": map[string]any{"type": "string"},
					"selectedFiles": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"parameters": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "string"},
					},
					"invariants": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
				"required": []string{"planSummary", "parameters"},
			},
		},
		"required": []string{"planning"},
	})
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
			"id":          template.ID,
			"name":        template.Name,
			"description": template.Description,
			"stepCount":   template.StepCount,
		})
	}
	if len(lines) == 0 {
		lines = append(lines, "No task templates available.")
	}
	response := map[string]any{
		"templates":  structured,
		"nextAction": "Call centian.task_register with one templateId.",
	}
	addWorkspaceContext(response, p.server.TaskVerification.WorkingDir)
	return toolResult(strings.Join(lines, "\n"), response), nil
}

func (p *CentianEndpoint) handleTaskRegisterTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		raw := map[string]json.RawMessage{}
		if err := json.Unmarshal(req.Params.Arguments, &raw); err == nil {
			if _, exists := raw["parameters"]; exists {
				return nil, fmt.Errorf("task_register no longer accepts parameters; provide them in task_complete_planning under planning.parameters")
			}
		}
	}
	args := taskRegisterArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)
	if strings.TrimSpace(args.TaskDescription) == "" {
		args.TaskDescription = taskRegisterDescriptionFromRequest(req)
	}
	if strings.TrimSpace(args.TemplateID) == "" {
		return nil, fmt.Errorf("templateId is required")
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	if session.taskRun != nil && session.taskRun.Status == taskverification.TaskStatusActive {
		return nil, fmt.Errorf("an active task is already registered for this session")
	}

	run, err := p.server.TaskVerification.RegisterTaskWithDescription(ctx, args.TemplateID, args.TaskDescription)
	if err != nil {
		return nil, err
	}
	session.taskRun = run
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(run)
	registerPayload := taskToolEventPayload(args.TaskToolMetadata, nil)
	if taskDescription := strings.TrimSpace(args.TaskDescription); taskDescription != "" {
		if registerPayload == nil {
			registerPayload = map[string]any{}
		}
		registerPayload["taskDescription"] = taskDescription
	}
	p.recordTaskEvent(session, run, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeRegistered, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), registerPayload)

	structured := lifecycleStructuredContent(run, p.server.TaskVerification.WorkingDir)
	stepCount := len(run.SelectedTemplate.CompiledWorkflow.WorkflowSteps)
	return toolResult(fmt.Sprintf("Registered task %s with %d declared step(s).", run.TemplateID, stepCount), structured), nil
}

func (p *CentianEndpoint) handleTaskCompleteOnboardingTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskCompleteOnboardingArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.CompleteOnboarding(ctx, session.taskRun, &args.Onboarding); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeOnboardingCompleted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
			"error": err.Error(),
		}))
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeOnboardingCompleted, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
		"taskSummary": args.Onboarding.TaskSummary,
	}))
	structured := onboardingStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)
	return toolResult("Task onboarding completed; task moved to planning.", structured), nil
}

func (p *CentianEndpoint) handleTaskCompletePlanningTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskCompletePlanningArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)
	normalizePlanningArtifactArguments(req, &args.Planning)

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.CompletePlanning(ctx, session.taskRun, &args.Planning); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypePlanningCompleted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
			"error": err.Error(),
		}))
		var validationErr *taskverification.PlanningValidationError
		if errors.As(err, &validationErr) {
			return planningValidationToolResult(validationErr, session.taskRun, p.server.TaskVerification.WorkingDir), nil
		}
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypePlanningCompleted, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
		"planning": args.Planning,
	}))
	if node, exists := session.taskRun.CurrentNode(); exists && node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeApprovalWaitEntered, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, nil))
	}
	structured := planningCompletionStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)
	return toolResult(fmt.Sprintf("Task planning completed; task moved to %s.", session.taskRun.Phase), structured), nil
}

func normalizePlanningArtifactArguments(req *mcp.CallToolRequest, artifact *taskverification.PlanningArtifact) {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 || artifact == nil {
		return
	}
	rawArgs := map[string]json.RawMessage{}
	if err := json.Unmarshal(req.Params.Arguments, &rawArgs); err != nil {
		return
	}
	rawPlanning, exists := rawArgs["planning"]
	if !exists {
		return
	}
	planningMap := map[string]json.RawMessage{}
	if err := json.Unmarshal(rawPlanning, &planningMap); err != nil {
		return
	}
	if artifact.Parameters == nil {
		artifact.Parameters = map[string]string{}
	}
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "testCommand", "testCommand")
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "testTarget", "testTarget")
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "testFile", "testFile")
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "testName", "testName")
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "lintCommand", "lintCommand")
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "expectedError", "expectedError")
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "expectedFailure", "expectedError")
	copyLegacyPlanningParameter(planningMap, artifact.Parameters, "implementationTarget", "implementationTarget")
}

func copyLegacyPlanningParameter(raw map[string]json.RawMessage, parameters map[string]string, sourceKey, targetKey string) {
	if _, exists := parameters[targetKey]; exists {
		return
	}
	value, exists := raw[sourceKey]
	if !exists {
		return
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" {
		return
	}
	parameters[targetKey] = decoded
}

func taskRegisterDescriptionFromRequest(req *mcp.CallToolRequest) string {
	rawArgs := rawTaskToolArguments(req)
	for _, key := range []string{"task_description", "taskDescription", "prompt"} {
		raw, exists := rawArgs[key]
		if !exists {
			continue
		}
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err == nil {
			if trimmed := strings.TrimSpace(decoded); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func taskToolMetadataFromRequest(req *mcp.CallToolRequest) TaskToolMetadata {
	rawArgs := rawTaskToolArguments(req)
	raw, exists := rawArgs["annotations"]
	if !exists {
		return TaskToolMetadata{}
	}
	var annotations []any
	if err := json.Unmarshal(raw, &annotations); err != nil {
		return TaskToolMetadata{}
	}
	return TaskToolMetadata{Annotations: annotations}
}

func rawTaskToolArguments(req *mcp.CallToolRequest) map[string]json.RawMessage {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return nil
	}
	rawArgs := map[string]json.RawMessage{}
	if err := json.Unmarshal(req.Params.Arguments, &rawArgs); err != nil {
		return nil
	}
	return rawArgs
}

func taskToolEventPayload(metadata TaskToolMetadata, payload map[string]any) map[string]any {
	annotations := append([]any{}, metadata.Annotations...)
	if generated, ok := centianTaskGovernanceAnnotation(payload); ok && !hasGovernanceEventAnnotation(annotations) {
		annotations = append(annotations, generated)
	}
	if len(annotations) == 0 {
		return payload
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["annotations"] = annotations
	return payload
}

func centianTaskGovernanceAnnotation(payload map[string]any) (map[string]any, bool) {
	if payload == nil || !isGovernanceTaskFailurePayload(payload) {
		return nil, false
	}
	return map[string]any{
		"type":      "governance_events",
		"processor": "centian",
		"action":    "stopped",
		"category":  "compliance",
		"severity":  centianTaskGovernanceSeverity(payload),
		"message":   centianTaskGovernanceMessage(payload),
	}, true
}

func isGovernanceTaskFailurePayload(payload map[string]any) bool {
	if passed, ok := payload["passed"].(bool); ok && !passed {
		return true
	}
	if value, ok := payload["failureKind"].(string); ok && strings.TrimSpace(value) != "" {
		return true
	}
	if value, ok := payload["failedCheckId"].(string); ok && strings.TrimSpace(value) != "" {
		return true
	}
	if value, ok := payload["failedInvariantId"].(string); ok && strings.TrimSpace(value) != "" {
		return true
	}
	if value, ok := payload["error"].(string); ok && strings.TrimSpace(value) != "" {
		return true
	}
	return false
}

func centianTaskGovernanceMessage(payload map[string]any) string {
	if value, ok := payload["failedCheckDescription"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	for _, key := range []string{"summary", "error", "message"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if value, ok := payload["failedCheckId"].(string); ok && strings.TrimSpace(value) != "" {
		return fmt.Sprintf("expected `%s`", strings.TrimSpace(value))
	}
	if value, ok := payload["failedInvariantId"].(string); ok && strings.TrimSpace(value) != "" {
		return fmt.Sprintf("expected invariant `%s`", strings.TrimSpace(value))
	}
	return "process requirement failed"
}

func centianTaskGovernanceSeverity(payload map[string]any) string {
	if value, ok := payload["failureKind"].(string); ok && strings.TrimSpace(value) == "check" {
		return "medium"
	}
	if _, ok := payload["failedCheckId"].(string); ok {
		return "medium"
	}
	return "high"
}

func hasGovernanceEventAnnotation(annotations []any) bool {
	for _, annotation := range annotations {
		values, ok := annotation.(map[string]any)
		if !ok {
			continue
		}
		if values["type"] == "governance_events" {
			return true
		}
	}
	return false
}

func (p *CentianEndpoint) handleTaskStartStepTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskStepArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	result, err := p.server.TaskVerification.StartStep(ctx, session.taskRun, args.Step)
	if err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeStepStarted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
			"step":  args.Step,
			"error": err.Error(),
		}))
		return nil, err
	}
	outcome := taskverification.TaskEventOutcomeSucceeded
	if !result.Passed {
		outcome = taskverification.TaskEventOutcomeFailed
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeStepStarted, outcome, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, stepEventPayload(result)))
	return stepToolResult(result, session.taskRun, p.server.TaskVerification.WorkingDir), nil
}

func (p *CentianEndpoint) handleTaskCompleteStepTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskStepArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	result, err := p.server.TaskVerification.CompleteStep(ctx, session.taskRun, args.Step)
	if err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeStepCompleted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
			"step":  args.Step,
			"error": err.Error(),
		}))
		return nil, err
	}
	outcome := taskverification.TaskEventOutcomeSucceeded
	if !result.Passed {
		outcome = taskverification.TaskEventOutcomeFailed
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeStepCompleted, outcome, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, stepEventPayload(result)))
	if result.Passed {
		if node, exists := session.taskRun.CurrentNode(); exists && node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
			p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeApprovalWaitEntered, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, nil))
		}
	}
	return stepToolResult(result, session.taskRun, p.server.TaskVerification.WorkingDir), nil
}

func (p *CentianEndpoint) handleTaskResumeTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskMetadataArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)
	if err := p.server.TaskVerification.ResumeTask(ctx, session.taskRun); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeResumed, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
			"error": err.Error(),
		}))
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeResumed, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, nil))
	return toolResult("Task resumed.", lifecycleStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)), nil
}

func (p *CentianEndpoint) handleTaskRestartTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskMetadataArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.RestartTask(ctx, session.taskRun); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeRestarted, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
			"error": err.Error(),
		}))
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeRestarted, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, nil))
	return toolResult("Task restarted.", lifecycleStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)), nil
}

func (p *CentianEndpoint) handleTaskFailTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskFailArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}
	args.TaskToolMetadata = taskToolMetadataFromRequest(req)

	session.taskMu.Lock()
	defer session.taskMu.Unlock()
	sourcePhase, sourceNodeKind := taskPhaseSnapshot(session.taskRun)

	if err := p.server.TaskVerification.FailTask(ctx, session.taskRun, args.Reason); err != nil {
		p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, sourcePhase, sourceNodeKind, taskverification.TaskEventTypeFailed, taskverification.TaskEventOutcomeFailed, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
			"reason": args.Reason,
			"error":  err.Error(),
		}))
		return nil, err
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(session.taskRun)
	p.recordTaskEvent(session, session.taskRun, sourcePhase, sourceNodeKind, resultingPhase, resultingNodeKind, taskverification.TaskEventTypeFailed, taskverification.TaskEventOutcomeSucceeded, taskActionRequestIDFromContext(ctx), taskToolEventPayload(args.TaskToolMetadata, map[string]any{
		"reason": args.Reason,
	}))
	message := "Task failed."
	if strings.TrimSpace(args.Reason) != "" {
		message = fmt.Sprintf("Task failed: %s", strings.TrimSpace(args.Reason))
	}
	return toolResult(message, lifecycleStructuredContent(session.taskRun, p.server.TaskVerification.WorkingDir)), nil
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

func stepToolResult(result *taskverification.StepResult, run *taskverification.RunState, workingDir string) *mcp.CallToolResult {
	structured := stepStructuredContent(run, workingDir)
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
	if result.FailedCheckDescription != "" {
		structured["failedCheckDescription"] = result.FailedCheckDescription
	}
	if result.FailedInvariantID != "" {
		structured["failedInvariantId"] = result.FailedInvariantID
	}
	if result.Retryable {
		structured["retryable"] = result.Retryable
	}
	if result.RestartRequired {
		structured["restartRequired"] = result.RestartRequired
	}
	if len(result.RecoveryActions) > 0 {
		structured["recoveryActions"] = recoveryActionsStructuredContent(result.RecoveryActions)
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
	structured["nextAction"] = nextActionForStepResult(result, run)
	return toolResult(result.Message, structured)
}

func lifecycleStructuredContent(run *taskverification.RunState, workingDir string) map[string]any {
	if run == nil {
		structured := map[string]any{}
		addWorkspaceContext(structured, workingDir)
		return structured
	}

	structured := map[string]any{
		"taskRunId":      run.RunID,
		"templateId":     run.TemplateID,
		"templateName":   run.SelectedTemplate.Task.Name,
		"status":         string(run.Status),
		"phase":          string(run.Phase),
		"hasOnboarding":  run.Onboarding != nil,
		"hasPlanning":    run.Planning != nil,
		"executionReady": run.WorkflowReady,
	}
	if strings.TrimSpace(run.TaskDescription) != "" {
		structured["taskDescription"] = run.TaskDescription
	}
	if strings.TrimSpace(run.ExplicitFailReason) != "" {
		structured["explicitFailReason"] = run.ExplicitFailReason
	}
	addTaskTimingFields(structured, run)
	addWorkspaceContext(structured, workingDir)
	addCurrentNodeContext(structured, run)
	structured["nextAction"] = nextActionForRun(run)
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

func onboardingStructuredContent(run *taskverification.RunState, workingDir string) map[string]any {
	structured := lifecycleStructuredContent(run, workingDir)
	if run == nil {
		return structured
	}
	structured["requiredInputNames"] = run.SelectedTemplate.RequiredParameterNames()
	structured["requiredPlanningParameters"] = requiredPlanningParameters(run.SelectedTemplate.ParameterDefinitions())
	addPlanningNodeContext(structured, run)
	return structured
}

func planningCompletionStructuredContent(run *taskverification.RunState, workingDir string) map[string]any {
	structured := lifecycleStructuredContent(run, workingDir)
	if run == nil {
		return structured
	}
	structured["planningSummary"] = planningSummary(run)
	structured["frozenContractSummary"] = frozenContractSummary(run)
	if run.WorkflowReady && run.RunnableTemplate != nil {
		structured["steps"] = workflowStepsSummary(run)
		addStepContract(structured, run)
	}
	return structured
}

func stepStructuredContent(run *taskverification.RunState, workingDir string) map[string]any {
	if run == nil {
		structured := map[string]any{}
		addWorkspaceContext(structured, workingDir)
		return structured
	}
	structured := map[string]any{
		"taskRunId":    run.RunID,
		"templateId":   run.TemplateID,
		"templateName": run.SelectedTemplate.Task.Name,
		"status":       string(run.Status),
		"phase":        string(run.Phase),
	}
	if strings.TrimSpace(run.TaskDescription) != "" {
		structured["taskDescription"] = run.TaskDescription
	}
	addTaskTimingFields(structured, run)
	addWorkspaceContext(structured, workingDir)
	addCurrentNodeContext(structured, run)
	addStepContract(structured, run)
	return structured
}

func recoveryActionsStructuredContent(actions []taskverification.RecoveryAction) []map[string]any {
	if len(actions) == 0 {
		return []map[string]any{}
	}
	structured := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		entry := map[string]any{
			"kind":    action.Kind,
			"summary": action.Summary,
		}
		if action.Tool != "" {
			entry["tool"] = action.Tool
		}
		if len(action.Arguments) > 0 {
			entry["arguments"] = action.Arguments
		}
		structured = append(structured, entry)
	}
	return structured
}

func planningSummary(run *taskverification.RunState) map[string]any {
	summary := map[string]any{
		"planSummary":   "",
		"selectedFiles": []string{},
		"parameters":    map[string]string{},
	}
	if run == nil || run.Planning == nil {
		return summary
	}
	summary["planSummary"] = run.Planning.PlanSummary
	summary["selectedFiles"] = append([]string{}, run.Planning.SelectedFiles...)
	summary["parameters"] = cloneStringMap(run.Planning.Parameters)
	return summary
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
		return "Call centian.task_complete_planning with planning.parameters containing every required planning parameter. Execution cannot begin until the full planning contract is provided, and Centian enforces it."
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

func nextActionForStepResult(result *taskverification.StepResult, run *taskverification.RunState) string {
	if result == nil {
		return nextActionForRun(run)
	}
	if result.Passed {
		return nextActionForRun(run)
	}
	if len(result.RecoveryActions) > 0 && strings.TrimSpace(result.RecoveryActions[0].Summary) != "" {
		return result.RecoveryActions[0].Summary
	}
	return "Inspect the step failure and recover before retrying."
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
}

func addPlanningNodeContext(structured map[string]any, run *taskverification.RunState) {
	if run == nil || run.SelectedTemplate.CompiledWorkflow == nil {
		return
	}
	planningNode, exists := run.SelectedTemplate.CompiledWorkflow.Nodes[run.SelectedTemplate.CompiledWorkflow.PlanningPath]
	if !exists {
		return
	}
	if _, present := structured["planningEditableFields"]; !present && len(planningNode.EditableFields) > 0 {
		structured["planningEditableFields"] = append([]string{}, planningNode.EditableFields...)
	}
	if _, present := structured["planningRequiredInputs"]; !present && len(planningNode.RequiredPlanningInputs) > 0 {
		structured["planningRequiredInputs"] = append([]string{}, planningNode.RequiredPlanningInputs...)
	}
}

func addStepContract(structured map[string]any, run *taskverification.RunState) {
	if structured == nil || run == nil || run.RunnableTemplate == nil || run.RunnableTemplate.CompiledWorkflow == nil {
		return
	}
	if run.Status != taskverification.TaskStatusActive {
		return
	}
	stepNumber, _ := activeOrCurrentStep(run)
	if stepNumber == 0 || stepNumber > len(run.RunnableTemplate.CompiledWorkflow.WorkflowSteps) {
		return
	}
	step := &run.RunnableTemplate.CompiledWorkflow.WorkflowSteps[stepNumber-1]
	structured["stepContract"] = stepContract(stepNumber, step)
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

func requiredPlanningParameters(definitions []taskverification.TemplateParameter) map[string]any {
	if len(definitions) == 0 {
		return map[string]any{}
	}
	required := make(map[string]any, len(definitions))
	for _, definition := range definitions {
		required[definition.Name] = map[string]any{
			"name":        definition.Name,
			"description": definition.Description,
			"required":    true,
		}
	}
	return required
}

func planningValidationToolResult(validationErr *taskverification.PlanningValidationError, run *taskverification.RunState, workingDir string) *mcp.CallToolResult {
	structured := onboardingStructuredContent(run, workingDir)
	structured["error"] = validationErr.Error()
	structured["planningParametersField"] = "planning.parameters"
	structured["requiredPlanningParameterNames"] = append([]string(nil), validationErr.RequiredParameterNames...)
	structured["providedPlanningParameterNames"] = append([]string(nil), validationErr.ProvidedParameterNames...)
	if len(validationErr.MissingParameters) > 0 {
		structured["missingRequiredPlanningParameters"] = append([]string(nil), validationErr.MissingParameters...)
	}
	if len(validationErr.UnknownParameters) > 0 {
		structured["unknownPlanningParameters"] = append([]string(nil), validationErr.UnknownParameters...)
	}
	structured["nextAction"] = "Resend centian.task_complete_planning with a complete planning.parameters object containing every required planning parameter. Execution remains blocked until the enforced planning contract is satisfied."
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: validationErr.Error()},
		},
		StructuredContent: structured,
	}
}

func stepContract(stepNumber int, step *taskverification.Step) map[string]any {
	contract := map[string]any{
		"step":         stepNumber,
		"id":           step.ID,
		"path":         string(step.Path),
		"name":         step.Name,
		"description":  step.Description,
		"instructions": step.Instructions,
	}
	if len(step.Checks) > 0 {
		checks := make([]map[string]any, 0, len(step.Checks))
		for _, check := range step.Checks {
			checks = append(checks, map[string]any{
				"id":             check.ID,
				"description":    check.Description,
				"command":        check.Command,
				"preConditions":  contractConditions(check.PreConditions),
				"postConditions": contractConditions(check.PostConditions),
			})
		}
		contract["checks"] = checks
	}
	if len(step.Invariants) > 0 {
		invariants := make([]map[string]any, 0, len(step.Invariants))
		for _, invariant := range step.Invariants {
			invariants = append(invariants, map[string]any{
				"id":                   invariant.ID,
				"description":          invariant.Description,
				"command":              invariant.Command,
				"technicalDescription": taskverification.InvariantTechnicalDescription(invariant),
			})
		}
		contract["invariants"] = invariants
	}
	return contract
}

func contractConditions(conditions []taskverification.Condition) []map[string]any {
	if len(conditions) == 0 {
		return []map[string]any{}
	}
	described := make([]map[string]any, 0, len(conditions))
	for _, condition := range conditions {
		entry := map[string]any{
			"type":                 condition.Type,
			"technicalDescription": taskverification.ConditionTechnicalDescription(condition),
		}
		if condition.Path != "" {
			entry["path"] = condition.Path
		}
		if condition.Value != nil {
			entry["value"] = condition.Value
		}
		if len(condition.Values) > 0 {
			entry["values"] = append([]any(nil), condition.Values...)
		}
		described = append(described, entry)
	}
	return described
}

func frozenContractSummary(run *taskverification.RunState) map[string]any {
	summary := map[string]any{
		"planSummary":    "",
		"selectedFiles":  []string{},
		"parameters":     map[string]string{},
		"invariantCount": 0,
	}
	if run == nil || run.Planning == nil {
		return summary
	}

	summary["planSummary"] = run.Planning.PlanSummary
	summary["selectedFiles"] = append([]string{}, run.Planning.SelectedFiles...)
	summary["parameters"] = cloneStringMap(run.Planning.Parameters)
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

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
