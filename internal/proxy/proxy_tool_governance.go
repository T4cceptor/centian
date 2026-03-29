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

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	run := session.taskRun
	if run == nil {
		if p.server != nil && p.server.Config != nil && p.server.Config.Proxy.TaskVerificationEnabled() {
			return governanceDeniedResult(callCtx, taskverification.TaskPhaseInitialization, "", "", nil, governanceDeniedRegistrationNeeded), true
		}
		return nil, false
	}
	if run.Status != taskverification.TaskStatusActive {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, "", nil, governanceReasonForTaskStatus(run.Status)), true
	}

	node, exists := run.CurrentNode()
	if !exists {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, "", nil, "unknown_workflow_node"), true
	}
	if node.Kind == taskverification.WorkflowNodeKindWaitingForApproval {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, node.Kind, node.AllowedTools, governanceDeniedWaitingForApproval), true
	}
	if len(node.AllowedTools) == 0 {
		return governanceDeniedResult(callCtx, run.Phase, run.Status, node.Kind, node.AllowedTools, governanceDeniedNoAllowlist), true
	}
	if matchesAllowedTool(node.AllowedTools, callCtx.GetOriginalToolName(), callCtx.GetToolName()) {
		return nil, false
	}
	return governanceDeniedResult(callCtx, run.Phase, run.Status, node.Kind, node.AllowedTools, governanceDeniedNoPatternMatch), true
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
		},
	}

	if callCtx != nil {
		callCtx.SetStatus(403)
		callCtx.SetError(message)
		callCtx.SetResult(result)
		callCtx.SetDirection(common.DirectionServerToClient)
		callCtx.SetMessageType(common.MessageTypeResponse)
		if logHandler := callCtx.GetLogHandler(); logHandler != nil {
			_ = logHandler.Log(callCtx)
		}
	}

	return result
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
