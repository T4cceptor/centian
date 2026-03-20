package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	taskListTemplatesTool = "centian.task_list_templates"
	taskRegisterTool      = "centian.task_register"
	taskStartStepTool     = "centian.task_start_step"
	taskCompleteStepTool  = "centian.task_complete_step"
	taskRestartTool       = "centian.task_restart"
	taskFailTool          = "centian.task_fail"
)

type taskRegisterArgs struct {
	TemplateID string         `json:"templateId"`
	Parameters map[string]any `json:"parameters"`
}

type taskStepArgs struct {
	Step int `json:"step"`
}

type taskFailArgs struct {
	Reason string `json:"reason"`
}

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
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleTaskListTemplatesTool(ctx, session, req)
	})
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
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleTaskRegisterTool(ctx, session, req)
	})
	session.registeredStaticTools[taskRegisterTool] = struct{}{}

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
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleTaskStartStepTool(ctx, session, req)
	})
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
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleTaskCompleteStepTool(ctx, session, req)
	})
	session.registeredStaticTools[taskCompleteStepTool] = struct{}{}

	server.AddTool(&mcp.Tool{
		Name:        taskRestartTool,
		Description: "Restart the active task verification run and clear step state.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleTaskRestartTool(ctx, session, req)
	})
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
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleTaskFailTool(ctx, session, req)
	})
	session.registeredStaticTools[taskFailTool] = struct{}{}
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
			"parameters":  template.Parameters,
			"stepCount":   template.StepCount,
		})
	}
	if len(lines) == 0 {
		lines = append(lines, "No task templates available.")
	}
	return toolResult(strings.Join(lines, "\n"), map[string]any{"templates": structured}), nil
}

func (p *CentianEndpoint) handleTaskRegisterTool(_ context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	if session.taskRun != nil && (session.taskRun.Status == taskverification.TaskStatusRegistered || session.taskRun.Status == taskverification.TaskStatusInProgress) {
		return nil, fmt.Errorf("an active task is already registered for this session")
	}

	run, err := p.server.TaskVerification.RegisterTask(args.TemplateID, parameters)
	if err != nil {
		return nil, err
	}
	session.taskRun = run

	return toolResult(
		fmt.Sprintf("Registered task %s with %d step(s).", run.TemplateID, len(run.Steps)),
		map[string]any{
			"templateId": run.TemplateID,
			"status":     string(run.Status),
			"stepCount":  len(run.Steps),
			"parameters": run.Parameters,
		},
	), nil
}

func (p *CentianEndpoint) handleTaskStartStepTool(_ context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskStepArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	result, err := p.server.TaskVerification.StartStep(session.taskRun, args.Step)
	if err != nil {
		return nil, err
	}
	return stepToolResult(result, session.taskRun), nil
}

func (p *CentianEndpoint) handleTaskCompleteStepTool(_ context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskStepArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	result, err := p.server.TaskVerification.CompleteStep(session.taskRun, args.Step)
	if err != nil {
		return nil, err
	}
	return stepToolResult(result, session.taskRun), nil
}

func (p *CentianEndpoint) handleTaskRestartTool(_ context.Context, session *UpstreamSession, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	if err := p.server.TaskVerification.RestartTask(session.taskRun); err != nil {
		return nil, err
	}
	return toolResult("Task restarted.", runStructuredContent(session.taskRun)), nil
}

func (p *CentianEndpoint) handleTaskFailTool(_ context.Context, session *UpstreamSession, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := taskFailArgs{}
	if err := decodeToolArguments(req, &args); err != nil {
		return nil, err
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	if err := p.server.TaskVerification.FailTask(session.taskRun, args.Reason); err != nil {
		return nil, err
	}
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
	structured["taskStatus"] = string(result.TaskStatus)
	structured["stepStatus"] = string(result.StepStatus)
	return toolResult(result.Message, structured)
}

func runStructuredContent(run *taskverification.RunState) map[string]any {
	if run == nil {
		return map[string]any{}
	}

	steps := make([]map[string]any, 0, len(run.Steps))
	for index, step := range run.Steps {
		steps = append(steps, map[string]any{
			"step":   index + 1,
			"id":     step.ID,
			"status": step.Status,
		})
	}
	sort.Slice(steps, func(i, j int) bool {
		return steps[i]["step"].(int) < steps[j]["step"].(int)
	})

	return map[string]any{
		"templateId":         run.TemplateID,
		"status":             string(run.Status),
		"parameters":         run.Parameters,
		"steps":              steps,
		"lastFailureMessage": run.LastFailureMessage,
		"explicitFailReason": run.ExplicitFailReason,
	}
}
