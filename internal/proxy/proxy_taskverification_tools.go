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
	taskListTemplatesTool      = "centian.task_list_templates"
	taskRegisterTool           = "centian.task_register"
	taskCompleteOnboardingTool = "centian.task_complete_onboarding"
	taskCompletePlanningTool   = "centian.task_complete_planning"
	taskStartStepTool          = "centian.task_start_step"
	taskCompleteStepTool       = "centian.task_complete_step"
	taskRestartTool            = "centian.task_restart"
	taskFailTool               = "centian.task_fail"
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
		Description: "List available task verification templates.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, p.wrapTaskToolHandler(session, taskListTemplatesTool, p.handleTaskListTemplatesTool))
	session.registeredStaticTools[taskListTemplatesTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskRegisterTool,
		Description: "Register a task verification run from a template.",
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
		Description: "Persist onboarding context and advance the task into planning.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"onboarding": map[string]any{"type": "object"},
			},
			"required": []string{"onboarding"},
		},
	}, p.wrapTaskToolHandler(session, taskCompleteOnboardingTool, p.handleTaskCompleteOnboardingTool))
	session.registeredStaticTools[taskCompleteOnboardingTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskCompletePlanningTool,
		Description: "Persist planning context, freeze the contract, and enter execution.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"planning": map[string]any{"type": "object"},
			},
			"required": []string{"planning"},
		},
	}, p.wrapTaskToolHandler(session, taskCompletePlanningTool, p.handleTaskCompletePlanningTool))
	session.registeredStaticTools[taskCompletePlanningTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskStartStepTool,
		Description: "Start a task step by running preconditions and capturing invariant baselines.",
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
		Description: "Complete a task step by running postconditions and invariant checks.",
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
		Name:        taskRestartTool,
		Description: "Restart the active task verification run and clear step state.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, p.wrapTaskToolHandler(session, taskRestartTool, p.handleTaskRestartTool))
	session.registeredStaticTools[taskRestartTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskFailTool,
		Description: "Explicitly fail the active task verification run.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{"type": "string"},
			},
		},
	}, p.wrapTaskToolHandler(session, taskFailTool, p.handleTaskFailTool))
	session.registeredStaticTools[taskFailTool] = struct{}{}
}

func (p *CentianEndpoint) wrapTaskToolHandler(
	session *UpstreamSession,
	toolName string,
	handler taskToolHandler,
) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		actionEventID := getNewUUIDV7()
		ctx = withTaskActionEventID(ctx, actionEventID)
		result, err := handler(ctx, session, req)
		p.logTaskToolCall(session, actionEventID, toolName, req, result, err)
		session.taskMu.Lock()
		run := session.taskRun
		session.taskMu.Unlock()
		if run != nil {
			p.recordTaskActionContext(run, actionEventID)
		}
		return result, err
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
	return toolResult(strings.Join(lines, "\n"), map[string]any{"templates": structured}), nil
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
	p.recordTaskEvent(session, run, taskverification.TaskEventTypeRegistered, taskverification.TaskEventOutcomeSucceeded, taskActionEventIDFromContext(ctx), map[string]any{
		"draftParameters": run.DraftParameters,
	})

	structured := runStructuredContent(run)
	stepCount := len(run.SelectedTemplate.CompiledWorkflow.ExecutionSteps)
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

	if err := p.server.TaskVerification.CompleteOnboarding(session.taskRun, args.Onboarding); err != nil {
		p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeOnboardingCompleted, taskverification.TaskEventOutcomeFailed, taskActionEventIDFromContext(ctx), map[string]any{
			"error": err.Error(),
		})
		return nil, err
	}
	p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeOnboardingCompleted, taskverification.TaskEventOutcomeSucceeded, taskActionEventIDFromContext(ctx), map[string]any{
		"projectSummary": args.Onboarding.ProjectSummary,
	})
	structured := runStructuredContent(session.taskRun)
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

	if err := p.server.TaskVerification.CompletePlanning(session.taskRun, args.Planning); err != nil {
		p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypePlanningCompleted, taskverification.TaskEventOutcomeFailed, taskActionEventIDFromContext(ctx), map[string]any{
			"error": err.Error(),
		})
		return nil, err
	}
	p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypePlanningCompleted, taskverification.TaskEventOutcomeSucceeded, taskActionEventIDFromContext(ctx), map[string]any{
		"planning": args.Planning,
	})
	if node, exists := session.taskRun.CurrentNode(); exists && node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
		p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeApprovalWaitEntered, taskverification.TaskEventOutcomeSucceeded, taskActionEventIDFromContext(ctx), nil)
	}
	structured := runStructuredContent(session.taskRun)
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

	result, err := p.server.TaskVerification.StartStep(session.taskRun, args.Step)
	if err != nil {
		p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeStepStarted, taskverification.TaskEventOutcomeFailed, taskActionEventIDFromContext(ctx), map[string]any{
			"step":  args.Step,
			"error": err.Error(),
		})
		return nil, err
	}
	outcome := taskverification.TaskEventOutcomeSucceeded
	if !result.Passed {
		outcome = taskverification.TaskEventOutcomeFailed
	}
	p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeStepStarted, outcome, taskActionEventIDFromContext(ctx), stepEventPayload(result))
	return stepToolResult(result, session.taskRun), nil
}

func (p *CentianEndpoint) handleTaskCompleteStepTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskStepArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	result, err := p.server.TaskVerification.CompleteStep(session.taskRun, args.Step)
	if err != nil {
		p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeStepCompleted, taskverification.TaskEventOutcomeFailed, taskActionEventIDFromContext(ctx), map[string]any{
			"step":  args.Step,
			"error": err.Error(),
		})
		return nil, err
	}
	outcome := taskverification.TaskEventOutcomeSucceeded
	if !result.Passed {
		outcome = taskverification.TaskEventOutcomeFailed
	}
	p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeStepCompleted, outcome, taskActionEventIDFromContext(ctx), stepEventPayload(result))
	if result.Passed {
		if node, exists := session.taskRun.CurrentNode(); exists && node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
			p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeApprovalWaitEntered, taskverification.TaskEventOutcomeSucceeded, taskActionEventIDFromContext(ctx), nil)
		}
	}
	return stepToolResult(result, session.taskRun), nil
}

func (p *CentianEndpoint) handleTaskRestartTool(ctx context.Context, session *UpstreamSession, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	if err := p.server.TaskVerification.RestartTask(session.taskRun); err != nil {
		p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeRestarted, taskverification.TaskEventOutcomeFailed, taskActionEventIDFromContext(ctx), map[string]any{
			"error": err.Error(),
		})
		return nil, err
	}
	p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeRestarted, taskverification.TaskEventOutcomeSucceeded, taskActionEventIDFromContext(ctx), nil)
	return toolResult("Task restarted.", runStructuredContent(session.taskRun)), nil
}

func (p *CentianEndpoint) handleTaskFailTool(ctx context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskFailArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	if err := p.server.TaskVerification.FailTask(session.taskRun, args.Reason); err != nil {
		p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeFailed, taskverification.TaskEventOutcomeFailed, taskActionEventIDFromContext(ctx), map[string]any{
			"reason": args.Reason,
			"error":  err.Error(),
		})
		return nil, err
	}
	p.recordTaskEvent(session, session.taskRun, taskverification.TaskEventTypeFailed, taskverification.TaskEventOutcomeSucceeded, taskActionEventIDFromContext(ctx), map[string]any{
		"reason": args.Reason,
	})
	message := "Task failed."
	if strings.TrimSpace(args.Reason) != "" {
		message = fmt.Sprintf("Task failed: %s", strings.TrimSpace(args.Reason))
	}
	return toolResult(message, runStructuredContent(session.taskRun)), nil
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

func stepToolResult(result *taskverification.StepResult, run *taskverification.RunState) *mcp.CallToolResult {
	structured := runStructuredContent(run)
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
	return toolResult(result.Message, structured)
}

func runStructuredContent(run *taskverification.RunState) map[string]any {
	if run == nil {
		return map[string]any{}
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
		"executionReady":     run.ExecutionReady,
		"stepCount":          len(run.SelectedTemplate.CompiledWorkflow.ExecutionSteps),
		"lastFailureMessage": run.LastFailureMessage,
		"explicitFailReason": run.ExplicitFailReason,
	}
	if node, exists := run.CurrentNode(); exists {
		structured["currentNodeKind"] = string(node.Kind)
		structured["approvalBlocked"] = node.Kind == taskverification.WorkflowNodeKindWaitingForApproval
		structured["allowedTools"] = append([]string{}, node.AllowedTools...)
		if node.NextPath != "" {
			structured["nextNodePath"] = string(node.NextPath)
		}
	}
	if run.Onboarding != nil {
		structured["onboardingSummary"] = run.Onboarding.ProjectSummary
	}
	if run.Planning != nil {
		structured["planningSummary"] = map[string]any{
			"selectedFiles":        run.Planning.SelectedFiles,
			"testTarget":           run.Planning.TestTarget,
			"lintCommand":          run.Planning.LintCommand,
			"implementationTarget": run.Planning.ImplementationTarget,
		}
		structured["frozenContractSummary"] = frozenContractSummary(run)
	}

	if !run.ExecutionReady || run.ExecutionTemplate == nil {
		return structured
	}

	steps := make([]map[string]any, 0, len(run.Steps))
	for index, step := range run.Steps {
		templateStep := run.ExecutionTemplate.CompiledWorkflow.ExecutionSteps[index]
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
	structured["steps"] = steps
	return structured
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
	if run.ExecutionTemplate != nil && run.ExecutionTemplate.CompiledWorkflow != nil {
		invariantCount := 0
		for _, step := range run.ExecutionTemplate.CompiledWorkflow.ExecutionSteps {
			invariantCount += len(step.Invariants)
		}
		summary["invariantCount"] = invariantCount
	}
	return summary
}
