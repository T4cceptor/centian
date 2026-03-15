package proxy

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file registers upstream prompt surfaces and routes proxied prompt
// requests to downstream servers. It mirrors the pattern in proxy_tools.go.

// syncAvailablePrompts reconciles the upstream server's registered prompts
// against the set currently available from connected downstream servers.
//
// The collection step (building desiredPrompts) is deliberately separate from
// the registration step so that a future capability-filtering processor can be
// inserted between them (e.g. to redact prompts based on policy before
// advertising them to upstream clients).
func (p *CentianEndpoint) syncAvailablePrompts(session *UpstreamSession) {
	if session == nil || session.upstreamServer == nil {
		return
	}
	if session.registeredPrompts == nil {
		session.registeredPrompts = make(map[string]struct{})
	}

	// --- Collection step ---
	type promptEntry struct {
		prompt     *mcp.Prompt
		serverName string
	}
	desiredPrompts := make(map[string]promptEntry) // keyed by upstream prompt name
	for serverName, conn := range session.downstreamConns {
		if !conn.IsConnected() {
			continue
		}
		for _, prompt := range conn.Prompts() {
			if prompt == nil {
				continue
			}
			upstreamName := prompt.Name
			if p.isAggregatedProxy {
				upstreamName = fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, prompt.Name)
			}
			desiredPrompts[upstreamName] = promptEntry{prompt: prompt, serverName: serverName}
		}
	}

	// --- Registration step (future hook point for capability filtering) ---

	staleNames := make([]string, 0)
	for name := range session.registeredPrompts {
		if _, ok := desiredPrompts[name]; !ok {
			staleNames = append(staleNames, name)
		}
	}
	if len(staleNames) > 0 {
		session.upstreamServer.RemovePrompts(staleNames...)
		for _, name := range staleNames {
			delete(session.registeredPrompts, name)
		}
	}

	for upstreamName, entry := range desiredPrompts {
		if _, ok := session.registeredPrompts[upstreamName]; ok {
			continue
		}
		p.registerPrompt(session, entry.serverName, entry.prompt, upstreamName)
	}
}

// registerPrompt adds a single downstream prompt to the upstream-facing server.
func (p *CentianEndpoint) registerPrompt(session *UpstreamSession, serverName string, prompt *mcp.Prompt, upstreamName string) {
	if session.registeredPrompts == nil {
		session.registeredPrompts = make(map[string]struct{})
	}
	if _, exists := session.registeredPrompts[upstreamName]; exists {
		return
	}
	session.registeredPrompts[upstreamName] = struct{}{}

	clonedPrompt := &mcp.Prompt{
		Name:        upstreamName,
		Description: prompt.Description,
		Arguments:   prompt.Arguments,
	}
	if p.isAggregatedProxy {
		clonedPrompt.Description = fmt.Sprintf("[%s] %s", serverName, prompt.Description)
	}

	downstreamName := prompt.Name
	session.upstreamServer.AddPrompt(clonedPrompt, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return p.forwardGetPrompt(ctx, session, serverName, downstreamName, req)
	})
}

// registerAvailablePrompts acquires the necessary locks and calls syncAvailablePrompts.
func (p *CentianEndpoint) registerAvailablePrompts(session *UpstreamSession) {
	if session == nil {
		return
	}

	p.mu.RLock()
	pool := p.downstreamPools[session.downstreamSessionKey]
	p.mu.RUnlock()

	if pool == nil {
		return
	}

	p.toolRegMu.Lock()
	p.syncAvailablePrompts(session)
	p.toolRegMu.Unlock()
}

// forwardGetPrompt retrieves a prompt from the named downstream server.
func (p *CentianEndpoint) forwardGetPrompt(ctx context.Context, session *UpstreamSession, serverName, downstreamName string, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	conn, err := session.GetConnectionByServerName(serverName)
	if err != nil {
		return nil, fmt.Errorf("prompt %q: %w", downstreamName, err)
	}
	result, err := conn.GetPrompt(ctx, downstreamName, req.Params.Arguments)
	if err != nil {
		return nil, normalizeForwardedMethodError(mcpMethodGetPrompt, err)
	}
	return result, nil
}
