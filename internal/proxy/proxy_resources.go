package proxy

import (
	"context"
	"fmt"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file registers upstream resource surfaces and routes proxied resource
// requests to downstream servers. It mirrors the pattern in proxy_tools.go.

// syncAvailableResources reconciles the upstream server's registered resources
// against the set currently available from connected downstream servers.
//
// The collection step (building desiredResources) is deliberately separate from
// the registration step so that a future capability-filtering processor can be
// inserted between them (e.g. to redact resources based on policy before
// advertising them to upstream clients).
func (p *CentianEndpoint) syncAvailableResources(session *UpstreamSession) {
	if session == nil || session.upstreamServer == nil {
		return
	}
	if session.registeredResources == nil {
		session.registeredResources = make(map[string]struct{})
	}

	// --- Collection step ---
	type resourceEntry struct {
		resource   *mcp.Resource
		serverName string
	}
	desiredResources := make(map[string]resourceEntry) // keyed by URI
	for serverName, conn := range session.downstreamConns {
		if !conn.IsConnected() {
			continue
		}
		for _, resource := range conn.Resources() {
			if resource == nil {
				continue
			}
			// Edge case: if multiple downstreams expose the same URI in an aggregated
			// proxy, the last one wins. URI namespacing across downstreams is not yet
			// implemented; callers should be aware of this limitation.
			desiredResources[resource.URI] = resourceEntry{resource: resource, serverName: serverName}
		}
	}

	// --- Registration step (future hook point for capability filtering) ---

	staleURIs := make([]string, 0)
	for uri := range session.registeredResources {
		if _, ok := desiredResources[uri]; !ok {
			staleURIs = append(staleURIs, uri)
		}
	}
	if len(staleURIs) > 0 {
		session.upstreamServer.RemoveResources(staleURIs...)
		for _, uri := range staleURIs {
			delete(session.registeredResources, uri)
		}
	}

	for uri, entry := range desiredResources {
		if _, ok := session.registeredResources[uri]; ok {
			continue
		}
		p.registerResource(session, entry.serverName, entry.resource)
	}
}

// registerResource adds a single downstream resource to the upstream-facing server.
func (p *CentianEndpoint) registerResource(session *UpstreamSession, serverName string, resource *mcp.Resource) {
	if session.registeredResources == nil {
		session.registeredResources = make(map[string]struct{})
	}
	if _, exists := session.registeredResources[resource.URI]; exists {
		return
	}
	session.registeredResources[resource.URI] = struct{}{}

	uri := resource.URI
	session.upstreamServer.AddResource(copyResourceForRegistration(resource), func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return p.forwardReadResource(ctx, session, serverName, uri)
	})
}

// registerAvailableResources acquires the necessary locks and calls syncAvailableResources.
func (p *CentianEndpoint) registerAvailableResources(session *UpstreamSession) {
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
	p.syncAvailableResources(session)
	p.toolRegMu.Unlock()
}

// syncAvailableResourceTemplates reconciles the upstream server's registered resource templates
// against the set currently available from connected downstream servers.
func (p *CentianEndpoint) syncAvailableResourceTemplates(session *UpstreamSession) {
	if session == nil || session.upstreamServer == nil {
		return
	}
	if session.registeredResourceTemplates == nil {
		session.registeredResourceTemplates = make(map[string]struct{})
	}

	type resourceTemplateEntry struct {
		resourceTemplate *mcp.ResourceTemplate
		serverName       string
	}
	desiredTemplates := make(map[string]resourceTemplateEntry) // keyed by URI template
	for serverName, conn := range session.downstreamConns {
		if !conn.IsConnected() {
			continue
		}
		for _, resourceTemplate := range conn.ResourceTemplates() {
			if resourceTemplate == nil {
				continue
			}
			desiredTemplates[resourceTemplate.URITemplate] = resourceTemplateEntry{
				resourceTemplate: resourceTemplate,
				serverName:       serverName,
			}
		}
	}

	staleTemplates := make([]string, 0)
	for uriTemplate := range session.registeredResourceTemplates {
		if _, ok := desiredTemplates[uriTemplate]; !ok {
			staleTemplates = append(staleTemplates, uriTemplate)
		}
	}
	if len(staleTemplates) > 0 {
		session.upstreamServer.RemoveResourceTemplates(staleTemplates...)
		for _, uriTemplate := range staleTemplates {
			delete(session.registeredResourceTemplates, uriTemplate)
		}
	}

	for uriTemplate, entry := range desiredTemplates {
		if _, ok := session.registeredResourceTemplates[uriTemplate]; ok {
			continue
		}
		p.registerResourceTemplate(session, entry.serverName, entry.resourceTemplate)
	}
}

func (p *CentianEndpoint) registerResourceTemplate(session *UpstreamSession, serverName string, resourceTemplate *mcp.ResourceTemplate) {
	if session.registeredResourceTemplates == nil {
		session.registeredResourceTemplates = make(map[string]struct{})
	}
	if _, exists := session.registeredResourceTemplates[resourceTemplate.URITemplate]; exists {
		return
	}
	session.registeredResourceTemplates[resourceTemplate.URITemplate] = struct{}{}

	session.upstreamServer.AddResourceTemplate(copyResourceTemplateForRegistration(resourceTemplate), func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return p.forwardReadResource(ctx, session, serverName, req.Params.URI)
	})
}

func (p *CentianEndpoint) registerAvailableResourceTemplates(session *UpstreamSession) {
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
	p.syncAvailableResourceTemplates(session)
	p.toolRegMu.Unlock()
}

// forwardReadResource reads a resource from the named downstream server.
func (p *CentianEndpoint) forwardReadResource(ctx context.Context, session *UpstreamSession, serverName, uri string) (*mcp.ReadResourceResult, error) {
	conn, err := session.GetConnectionByServerName(serverName)
	if err != nil {
		return nil, fmt.Errorf("resource %q: %w", uri, err)
	}
	result, err := conn.ReadResource(ctx, uri)
	if err != nil {
		return nil, normalizeForwardedMethodError(mcpMethodReadResource, err)
	}
	return result, nil
}

// forwardSubscribe forwards a resource subscription request to the downstream that owns the URI.
func (p *CentianEndpoint) forwardSubscribe(ctx context.Context, session *UpstreamSession, req *mcp.SubscribeRequest) error {
	uri := req.Params.URI
	conn := p.findConnectionForResourceURI(session, uri)
	if conn == nil {
		return fmt.Errorf("no downstream connection found for resource URI %q", uri)
	}
	return normalizeForwardedMethodError(mcpMethodSubscribe, conn.Subscribe(ctx, uri))
}

// forwardUnsubscribe forwards a resource unsubscription request to the downstream that owns the URI.
func (p *CentianEndpoint) forwardUnsubscribe(ctx context.Context, session *UpstreamSession, req *mcp.UnsubscribeRequest) error {
	uri := req.Params.URI
	conn := p.findConnectionForResourceURI(session, uri)
	if conn == nil {
		return fmt.Errorf("no downstream connection found for resource URI %q", uri)
	}
	return normalizeForwardedMethodError(mcpMethodUnsubscribe, conn.Unsubscribe(ctx, uri))
}

// forwardCompletion forwards a completion request to the first connected downstream.
// Completions are not resource-specific, so any capable downstream can handle them.
func (p *CentianEndpoint) forwardCompletion(ctx context.Context, session *UpstreamSession, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	for _, conn := range session.downstreamConns {
		if conn.IsConnected() {
			result, err := conn.Complete(ctx, req)
			if err != nil {
				return nil, normalizeForwardedMethodError(mcpMethodComplete, err)
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("no downstream connection available for completion request")
}

// findConnectionForResourceURI returns the first connected downstream that advertises the given URI.
func (p *CentianEndpoint) findConnectionForResourceURI(session *UpstreamSession, uri string) DownstreamConnectionInterface {
	for _, conn := range session.downstreamConns {
		if !conn.IsConnected() {
			continue
		}
		for _, resource := range conn.Resources() {
			if resource != nil && resource.URI == uri {
				return conn
			}
		}
	}
	common.LogWarn("ProxyEndpoint[%s]: no downstream found for resource URI %q", p.name, uri)
	return nil
}
