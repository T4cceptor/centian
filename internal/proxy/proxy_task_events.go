package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/taskverification"
)

type taskActionEventIDKey struct{}

func withTaskActionEventID(ctx context.Context, actionEventID string) context.Context {
	if actionEventID == "" {
		return ctx
	}
	return context.WithValue(ctx, taskActionEventIDKey{}, actionEventID)
}

func taskActionEventIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(taskActionEventIDKey{}).(string)
	return value
}

func (p *CentianEndpoint) recordTaskEvent(
	session *UpstreamSession,
	run *taskverification.RunState,
	eventType taskverification.TaskEventType,
	outcome taskverification.TaskEventOutcome,
	relatedActionEventID string,
	payload map[string]any,
) {
	if p == nil || p.server == nil || p.server.TaskVerification == nil || session == nil || run == nil {
		return
	}
	_ = p.server.TaskVerification.RecordTaskEvent(run, session.id, session.identityKey, eventType, outcome, relatedActionEventID, payload)
}

func (p *CentianEndpoint) recordTaskActionContext(run *taskverification.RunState, actionEventID string) {
	if p == nil || p.server == nil || p.server.TaskVerification == nil || run == nil || actionEventID == "" {
		return
	}
	_ = p.server.TaskVerification.RecordActionEventTaskContext(run, actionEventID)
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
