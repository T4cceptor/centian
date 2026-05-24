package proxy

import (
	"fmt"
	"path"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	governanceDeniedWaitingForApproval = "waiting_for_approval"
	governanceDeniedNoAllowlist        = "no_allowlist"
	governanceDeniedNoPatternMatch     = "no_matching_pattern"
	governanceDeniedRegistrationNeeded = "registration_required"
	governanceDeniedTaskCompleted      = "task_completed"
	governanceDeniedTaskFailed         = "task_failed"
	governanceDeniedTaskTimedOut       = "task_timed_out"
)

func (p *CentianEndpoint) enforceWorkflowNodeToolGovernance(session *UpstreamSession, callCtx CallContext) (*mcp.CallToolResult, bool) {
	if session == nil || callCtx == nil {
		return nil, false
	}

	policy := p.taskVerificationPolicy()
	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	run := session.taskRun
	if run == nil {
		if policy.requiresRegistration() {
			return governanceDeniedResult(callCtx, taskverification.TaskPhaseInitialization, "", "", nil, governanceDeniedRegistrationNeeded, p.server.TaskVerification.WorkingDir), true
		}
		return nil, false
	}
	if !policy.enforcesActiveTaskGovernance() {
		return nil, false
	}
	if run.Status != taskverification.TaskStatusActive {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, "", nil, governanceReasonForTaskStatus(run.Status), p.server.TaskVerification.WorkingDir), true
	}

	node, exists := run.CurrentNode()
	if !exists {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, "", nil, "unknown_workflow_node", p.server.TaskVerification.WorkingDir), true
	}
	if node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, node.Kind, node.AllowedTools, governanceDeniedWaitingForApproval, p.server.TaskVerification.WorkingDir), true
	}
	if len(node.AllowedTools) == 0 {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, node.Kind, node.AllowedTools, governanceDeniedNoAllowlist, p.server.TaskVerification.WorkingDir), true
	}
	if matchesAllowedTool(node.AllowedTools, callCtx.GetOriginalToolName(), callCtx.GetToolName()) {
		return nil, false
	}
	return governanceDeniedResult(callCtx, run.Phase, run.Status, node.Kind, node.AllowedTools, governanceDeniedNoPatternMatch, p.server.TaskVerification.WorkingDir), true
}

func matchesAllowedTool(patterns []string, upstreamName, canonicalName string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if matched, err := path.Match(pattern, upstreamName); err == nil && matched {
			return true
		}
		if matched, err := path.Match(pattern, canonicalName); err == nil && matched {
			return true
		}
	}
	return false
}

func governanceDeniedResult(
	callCtx CallContext,
	phase taskverification.TaskPhase,
	status taskverification.TaskStatus,
	nodeKind taskverification.WorkflowNodeKind,
	allowedTools []string,
	reason string,
	workingDir string,
) *mcp.CallToolResult {
	allowedToolCopy := append([]string{}, allowedTools...)
	requestedTool := ""
	if callCtx != nil {
		requestedTool = callCtx.GetOriginalToolName()
		if requestedTool == "" {
			requestedTool = callCtx.GetToolName()
		}
	}

	message := fmt.Sprintf(
		"tool %q is not allowed in workflow phase %q (%s): %s",
		requestedTool,
		phase,
		nodeKind,
		reason,
	)
	if phase == taskverification.TaskPhaseInitialization {
		message = `All actions are blocked until task registration at centian.
		Use 'centian.task_list_templates' to list all templates, 
		select one, and call 'centian.task_register' accordingly.
		Follow the centian workflow as provided to you.
		`
	} else if nodeKind == "" {
		message = fmt.Sprintf(
			"tool %q is not allowed in workflow phase %q: %s",
			requestedTool,
			phase,
			reason,
		)
	}

	result := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
		StructuredContent: map[string]any{
			"error":           message,
			"requestedTool":   requestedTool,
			"status":          string(status),
			"phase":           string(phase),
			"currentNodeKind": string(nodeKind),
			"reason":          reason,
			"allowedTools":    allowedToolCopy,
			"nextAction":      governanceNextAction(reason),
		},
	}
	if structured, ok := result.StructuredContent.(map[string]any); ok {
		addWorkspaceContext(structured, workingDir)
	}

	if callCtx != nil {
		callCtx.SetStatus(403)
		callCtx.SetError(message)
		callCtx.SetResult(result)
		callCtx.SetDirection(common.DirectionServerToClient)
		callCtx.SetMessageType(common.MessageTypeResponse)
		addCentianToolGovernanceAnnotation(callCtx, phase, reason)
		if logHandler := callCtx.GetLogHandler(); logHandler != nil {
			_ = logHandler.Log(callCtx)
		}
	}

	return result
}

func addCentianToolGovernanceAnnotation(callCtx CallContext, phase taskverification.TaskPhase, reason string) {
	meta := callCtx.GetMetaContext()
	if meta == nil {
		meta = common.NewMetaContext("", common.DirectionUnknown, common.MessageTypeUnknown)
		callCtx.SetMetaContext(meta)
	}
	meta.Annotations = append(meta.Annotations, common.EventAnnotation{
		Type:      "governance_events",
		Processor: "centian",
		Action:    "blocked",
		Category:  "risk",
		Severity:  "high",
		Message:   centianToolGovernanceAnnotationMessage(phase, reason),
	})
}

func centianToolGovernanceAnnotationMessage(phase taskverification.TaskPhase, reason string) string {
	switch reason {
	case governanceDeniedRegistrationNeeded:
		return "task registration required before tool use"
	case governanceDeniedWaitingForApproval:
		return "tool use blocked while waiting for approval"
	case governanceDeniedNoAllowlist:
		return fmt.Sprintf("no tools allowed in phase %s", phase)
	case governanceDeniedNoPatternMatch:
		return fmt.Sprintf("tool not allowed in phase %s", phase)
	case governanceDeniedTaskCompleted:
		return "task already completed"
	case governanceDeniedTaskFailed:
		return "task is failed"
	case governanceDeniedTaskTimedOut:
		return "task is timed out"
	default:
		return fmt.Sprintf("tool blocked by centian governance: %s", reason)
	}
}

func governanceNextAction(reason string) string {
	switch reason {
	case governanceDeniedRegistrationNeeded:
		return "Call centian.task_list_templates, then centian.task_register."
	case governanceDeniedTaskCompleted:
		return "Task is complete; stop task work."
	case governanceDeniedTaskFailed:
		return "Restart the task or register a new task run."
	case governanceDeniedTaskTimedOut:
		return "Call centian.task_resume or centian.task_restart."
	case governanceDeniedWaitingForApproval:
		return "Wait for approval before continuing task work."
	default:
		return "Follow the current Centian workflow state before retrying this tool."
	}
}

func governanceReasonForTaskStatus(status taskverification.TaskStatus) string {
	switch status {
	case taskverification.TaskStatusCompleted:
		return governanceDeniedTaskCompleted
	case taskverification.TaskStatusFailed:
		return governanceDeniedTaskFailed
	case taskverification.TaskStatusTimedOut:
		return governanceDeniedTaskTimedOut
	default:
		return governanceDeniedRegistrationNeeded
	}
}
