package proxy

import (
	"context"
	"time"

	"github.com/T4cceptor/centian/internal/taskverification"
)

func (p *CentianEndpoint) taskIdleTimeout() time.Duration {
	if p == nil {
		return 0
	}
	var seconds int
	if p.project != nil && p.project.Config != nil {
		seconds = p.project.Config.TaskVerificationCapability().GetIdleTimeoutSeconds()
	} else if p.server != nil && p.server.Config != nil && p.server.Config.Proxy != nil {
		// Fallback: legacy server config for backwards compatibility with tests.
		seconds = p.server.Config.Proxy.TaskVerificationCapability().GetIdleTimeoutSeconds()
	}
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func (p *CentianEndpoint) refreshTaskActivityLocked(session *UpstreamSession) {
	if session == nil || session.taskRun == nil {
		return
	}
	run := session.taskRun
	timeout := p.taskIdleTimeout()
	if run.Status != taskverification.TaskStatusActive || timeout <= 0 {
		p.cancelTaskTimeoutLocked(session)
		if timeout <= 0 {
			run.LastActivityAt = 0
		}
		if run.Status != taskverification.TaskStatusTimedOut {
			run.ExpiresAt = 0
		}
		return
	}

	now := time.Now().UTC()
	run.LastActivityAt = now.UnixMilli()
	run.ExpiresAt = now.Add(timeout).UnixMilli()
	session.taskTimeoutVersion++
	version := session.taskTimeoutVersion
	runID := run.RunID
	expiresAt := run.ExpiresAt
	if session.taskTimeoutTimer != nil {
		session.taskTimeoutTimer.Stop()
	}
	session.taskTimeoutTimer = time.AfterFunc(timeout, func() {
		p.handleTaskIdleTimeout(session, runID, version, expiresAt)
	})
}

func (p *CentianEndpoint) cancelTaskTimeoutLocked(session *UpstreamSession) {
	if session == nil {
		return
	}
	session.taskTimeoutVersion++
	if session.taskTimeoutTimer != nil {
		session.taskTimeoutTimer.Stop()
		session.taskTimeoutTimer = nil
	}
}

func (p *CentianEndpoint) maybeExpireActiveTaskLocked(session *UpstreamSession, relatedActionRequestID string) {
	if p.taskIdleTimeout() <= 0 || session == nil || session.taskRun == nil {
		return
	}
	run := session.taskRun
	if run.Status != taskverification.TaskStatusActive || run.ExpiresAt == 0 || time.Now().UTC().UnixMilli() < run.ExpiresAt {
		return
	}
	p.timeoutActiveTaskLocked(session, relatedActionRequestID)
}

func (p *CentianEndpoint) timeoutActiveTaskLocked(session *UpstreamSession, relatedActionRequestID string) {
	taskService := p.taskVerificationService()
	if session == nil || session.taskRun == nil || taskService == nil {
		return
	}
	run := session.taskRun
	if run.Status != taskverification.TaskStatusActive {
		return
	}

	sourcePhase, sourceNodeKind := taskPhaseSnapshot(run)
	if err := taskService.TimeoutTask(context.Background(), run); err != nil {
		return
	}
	session.taskTimeoutVersion++
	if session.taskTimeoutTimer != nil {
		session.taskTimeoutTimer.Stop()
		session.taskTimeoutTimer = nil
	}
	resultingPhase, resultingNodeKind := taskPhaseSnapshot(run)
	p.recordTaskEvent(
		session,
		run,
		sourcePhase,
		sourceNodeKind,
		resultingPhase,
		resultingNodeKind,
		taskverification.TaskEventTypeTimedOut,
		taskverification.TaskEventOutcomeSucceeded,
		relatedActionRequestID,
		map[string]any{"reason": "idle_timeout"},
	)
}

func (p *CentianEndpoint) handleTaskIdleTimeout(session *UpstreamSession, runID string, version uint64, expiresAt int64) {
	if session == nil {
		return
	}

	session.taskMu.Lock()
	defer session.taskMu.Unlock()

	if session.taskTimeoutVersion != version || session.taskRun == nil {
		return
	}
	if session.taskRun.RunID != runID || session.taskRun.Status != taskverification.TaskStatusActive {
		return
	}
	if session.taskRun.ExpiresAt != expiresAt {
		return
	}
	p.timeoutActiveTaskLocked(session, "")
}
