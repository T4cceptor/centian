package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"os/exec"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
)

const (
	poolRetryInitialDelay = 250 * time.Millisecond
	poolRetryMaxDelay     = 5 * time.Second
	poolRetryMaxWindow    = 30 * time.Second
)

type poolConnectFailureKind string

const (
	poolConnectFailureTransient poolConnectFailureKind = "transient"
	poolConnectFailureAuth      poolConnectFailureKind = "auth"
	poolConnectFailurePermanent poolConnectFailureKind = "permanent"
)

func classifyPoolConnectError(err error) poolConnectFailureKind {
	if err == nil {
		return poolConnectFailureTransient
	}
	if _, ok := centoauth.IsAuthorizationRequired(err); ok {
		return poolConnectFailureAuth
	}

	message := err.Error()
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return poolConnectFailurePermanent
	case strings.Contains(message, "both URL or Command configured"):
		return poolConnectFailurePermanent
	case strings.Contains(message, "no URL or Command configured"):
		return poolConnectFailurePermanent
	case strings.Contains(message, "missing downstream oauth context"):
		return poolConnectFailurePermanent
	case strings.Contains(message, "oauth manager not configured"):
		return poolConnectFailurePermanent
	case strings.Contains(message, "executable file not found"):
		return poolConnectFailurePermanent
	case strings.Contains(message, "permission denied"):
		return poolConnectFailurePermanent
	default:
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return poolConnectFailurePermanent
		}
		return poolConnectFailureTransient
	}
}

func poolRetryDelay(serverName string, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	delay := poolRetryInitialDelay
	for step := 1; step < attempt; step++ {
		delay *= 2
		if delay >= poolRetryMaxDelay {
			delay = poolRetryMaxDelay
			break
		}
	}

	jitterWindow := delay / 4
	if jitterWindow <= 0 {
		return delay
	}
	return delay + deterministicRetryJitter(serverName, attempt, jitterWindow)
}

func deterministicRetryJitter(serverName string, attempt int, jitterLimit time.Duration) time.Duration {
	if jitterLimit <= 0 {
		return 0
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(serverName))
	var attemptBuf [8]byte
	binary.LittleEndian.PutUint64(attemptBuf[:], uint64(attempt))
	_, _ = hasher.Write(attemptBuf[:])
	return time.Duration(hasher.Sum64() % uint64(jitterLimit))
}

func (p *CentianEndpoint) launchPoolConnectRetryLocked(
	pool *DownstreamSessionPool,
	serverName string,
	connectOptions *DownstreamConnectOptions,
) {
	if pool == nil || serverName == "" {
		return
	}
	if pool.retryCancels == nil {
		pool.retryCancels = make(map[string]context.CancelFunc)
	}
	if pool.retryAttempts == nil {
		pool.retryAttempts = make(map[string]int)
	}
	if pool.retryTokens == nil {
		pool.retryTokens = make(map[string]uint64)
	}

	//nolint:gosec // cancel is stored on the pool and invoked on worker completion or pool teardown.
	ctx, cancel := context.WithCancel(context.Background())
	pool.nextRetryToken++
	token := pool.nextRetryToken
	pool.connecting[serverName] = true
	pool.retryCancels[serverName] = cancel
	pool.retryAttempts[serverName] = 0
	pool.retryTokens[serverName] = token

	go p.connectDownstreamPoolWithRetry(ctx, pool.downstreamSessionKey, serverName, token, cloneDownstreamConnectOptions(connectOptions))
}

func (p *CentianEndpoint) connectDownstreamPoolWithRetry(
	ctx context.Context,
	downstreamSessionKey, serverName string,
	retryToken uint64,
	connectOptions *DownstreamConnectOptions,
) {
	startedAt := time.Now()

	for attempt := 1; ; attempt++ {
		conn, ok := p.poolRetryConnection(downstreamSessionKey, serverName, retryToken)
		if !ok {
			p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, false)
			return
		}

		p.setPoolRetryAttempt(downstreamSessionKey, serverName, retryToken, attempt)
		if err := p.connectDownstreamPool(ctx, downstreamSessionKey, conn, connectOptions); err == nil {
			p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, false)
			return
		} else if ctx.Err() != nil {
			p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, true)
			return
		} else {
			switch classifyPoolConnectError(err) {
			case poolConnectFailureAuth:
				p.handlePoolConnectAuthError(downstreamSessionKey, serverName, conn, err)
				p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, false)
				return
			case poolConnectFailurePermanent:
				p.handlePoolConnectFailure(downstreamSessionKey, serverName, conn, err)
				p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, false)
				return
			}

			if !p.poolRetryHasLiveSession(downstreamSessionKey, serverName, retryToken) {
				p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, true)
				return
			}

			delay := poolRetryDelay(serverName, attempt)
			if time.Since(startedAt)+delay > poolRetryMaxWindow {
				common.LogWarn(
					"ProxyEndpoint[%s]: giving up on pooled downstream %s after %d attempts in %s: %v",
					p.name,
					sanitizeLogValue(serverName),
					attempt,
					time.Since(startedAt).Round(time.Millisecond),
					err,
				)
				p.handlePoolConnectFailure(downstreamSessionKey, serverName, conn, err)
				p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, false)
				return
			}

			common.LogWarn(
				"ProxyEndpoint[%s]: failed to connect pooled downstream %s (attempt %d), retrying in %s: %v",
				p.name,
				sanitizeLogValue(serverName),
				attempt,
				delay.Round(time.Millisecond),
				err,
			)

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				p.clearPoolRetryState(downstreamSessionKey, serverName, retryToken, true)
				return
			case <-timer.C:
			}
		}
	}
}

func (p *CentianEndpoint) poolRetryConnection(
	downstreamSessionKey, serverName string,
	retryToken uint64,
) (DownstreamConnectionInterface, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pool := p.downstreamPools[downstreamSessionKey]
	if pool == nil {
		return nil, false
	}
	if pool.retryTokens[serverName] != retryToken {
		return nil, false
	}
	conn, ok := pool.downstreamConns[serverName]
	return conn, ok
}

func (p *CentianEndpoint) poolRetryHasLiveSession(
	downstreamSessionKey, serverName string,
	retryToken uint64,
) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pool := p.downstreamPools[downstreamSessionKey]
	if pool == nil {
		return false
	}
	if pool.retryTokens[serverName] != retryToken {
		return false
	}
	return p.poolHasLiveUpstreamSessionLocked(pool)
}

func (p *CentianEndpoint) poolHasLiveUpstreamSessionLocked(pool *DownstreamSessionPool) bool {
	if pool == nil {
		return false
	}
	for _, session := range pool.upstreamSessions {
		if session == nil {
			continue
		}
		if p.currentUpstreamServerSession(session) != nil {
			return true
		}
	}
	return false
}

func (p *CentianEndpoint) setPoolRetryAttempt(
	downstreamSessionKey, serverName string,
	retryToken uint64,
	attempt int,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	pool := p.downstreamPools[downstreamSessionKey]
	if pool == nil || pool.retryTokens[serverName] != retryToken {
		return
	}
	pool.retryAttempts[serverName] = attempt
}

func (p *CentianEndpoint) clearPoolRetryState(
	downstreamSessionKey, serverName string,
	retryToken uint64,
	clearConnecting bool,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	pool := p.downstreamPools[downstreamSessionKey]
	if pool == nil || pool.retryTokens[serverName] != retryToken {
		return
	}

	delete(pool.retryCancels, serverName)
	delete(pool.retryAttempts, serverName)
	delete(pool.retryTokens, serverName)
	if clearConnecting {
		delete(pool.connecting, serverName)
	}
}

func (p *CentianEndpoint) cancelPoolRetryWorkers(pool *DownstreamSessionPool) {
	if pool == nil {
		return
	}

	p.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(pool.retryCancels))
	for serverName, cancel := range pool.retryCancels {
		cancels = append(cancels, cancel)
		delete(pool.retryAttempts, serverName)
		delete(pool.retryTokens, serverName)
		delete(pool.connecting, serverName)
	}
	pool.retryCancels = make(map[string]context.CancelFunc)
	p.mu.Unlock()

	for _, cancel := range cancels {
		if cancel != nil {
			cancel()
		}
	}
}
