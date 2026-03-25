package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/taskverification"
)

type taskActionRequestIDKey struct{}

func withTaskActionRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, taskActionRequestIDKey{}, requestID)
}

func taskActionRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(taskActionRequestIDKey{}).(string)
	return value
}

func taskPhaseSnapshot(run *taskverification.RunState) (taskverification.TaskPhase, taskverification.WorkflowNodeKind) {
	if run == nil {
		return "", ""
	}
	nodeKind := taskverification.WorkflowNodeKind("")
	if node, exists := run.CurrentNode(); exists {
		nodeKind = node.Kind
	}
	return run.Phase, nodeKind
}

func (p *CentianEndpoint) recordTaskEvent(
	session *UpstreamSession,
	run *taskverification.RunState,
	sourcePhase taskverification.TaskPhase,
	sourceNodeKind taskverification.WorkflowNodeKind,
	resultingPhase taskverification.TaskPhase,
	resultingNodeKind taskverification.WorkflowNodeKind,
	eventType taskverification.TaskEventType,
	outcome taskverification.TaskEventOutcome,
	relatedActionRequestID string,
	payload map[string]any,
) {
	if p == nil || p.server == nil || p.server.TaskVerification == nil || session == nil || run == nil {
		return
	}
	_ = p.server.TaskVerification.RecordTaskEvent(
		run,
		session.id,
		session.identityKey,
		sourcePhase,
		sourceNodeKind,
		resultingPhase,
		resultingNodeKind,
		eventType,
		outcome,
		relatedActionRequestID,
		payload,
	)
}

func (p *CentianEndpoint) recordTaskActionContext(
	run *taskverification.RunState,
	requestID string,
	invocationPhase taskverification.TaskPhase,
	invocationNodeKind taskverification.WorkflowNodeKind,
) {
	if p == nil || p.server == nil || p.server.TaskVerification == nil || run == nil || requestID == "" {
		return
	}
	_ = p.server.TaskVerification.RecordActionEventTaskContext(run, requestID, invocationPhase, invocationNodeKind)
}

func stepEventPayload(result *taskverification.StepResult) map[string]any {
	if result == nil {
		return nil
	}
	payload := map[string]any{
		"step":   result.Step,
		"stepId": result.StepID,
		"passed": result.Passed,
	}
	if result.Summary != "" {
		payload["summary"] = result.Summary
	}
	if result.FailureKind != "" {
		payload["failureKind"] = string(result.FailureKind)
	}
	if result.FailurePhase != "" {
		payload["failurePhase"] = string(result.FailurePhase)
	}
	if result.FailedCheckID != "" {
		payload["failedCheckId"] = result.FailedCheckID
	}
	if result.FailedInvariantID != "" {
		payload["failedInvariantId"] = result.FailedInvariantID
	}
	return payload
}
