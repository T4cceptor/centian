package proxy

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file registers upstream resource surfaces and routes proxied resource
// requests to downstream servers. It mirrors the pattern in proxy_tools.go.

type resourceEntry struct {
	resource   *mcp.Resource
	serverName string
}

type resourceTemplateEntry struct {
	resourceTemplate *mcp.ResourceTemplate
	serverName       string
}

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

	desiredResources, collisions := p.desiredResourceState(session)
	p.updatePoolResourceCollisions(session.downstreamSessionKey, collisions)

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

	desiredTemplates, collisions := p.desiredResourceTemplateState(session)
	p.updatePoolResourceTemplateCollisions(session.downstreamSessionKey, collisions)

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
	conn, err := p.sessionConnection(session, serverName)
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
	if servers := p.collidingServersForResourceURI(session.downstreamSessionKey, uri); len(servers) > 0 {
		return fmt.Errorf(
			"resource URI %q is hidden because multiple downstreams expose it: %s",
			uri,
			strings.Join(servers, ", "),
		)
	}
	conn := p.findConnectionForResourceURI(session, uri)
	if conn == nil {
		return fmt.Errorf("no downstream connection found for resource URI %q", uri)
	}
	return normalizeForwardedMethodError(mcpMethodSubscribe, conn.Subscribe(ctx, uri))
}

// forwardUnsubscribe forwards a resource unsubscription request to the downstream that owns the URI.
func (p *CentianEndpoint) forwardUnsubscribe(ctx context.Context, session *UpstreamSession, req *mcp.UnsubscribeRequest) error {
	uri := req.Params.URI
	if servers := p.collidingServersForResourceURI(session.downstreamSessionKey, uri); len(servers) > 0 {
		return fmt.Errorf(
			"resource URI %q is hidden because multiple downstreams expose it: %s",
			uri,
			strings.Join(servers, ", "),
		)
	}
	conn := p.findConnectionForResourceURI(session, uri)
	if conn == nil {
		return fmt.Errorf("no downstream connection found for resource URI %q", uri)
	}
	return normalizeForwardedMethodError(mcpMethodUnsubscribe, conn.Unsubscribe(ctx, uri))
}

// forwardCompletion forwards a completion request to the first connected downstream.
// Completions are not resource-specific, so any capable downstream can handle them.
func (p *CentianEndpoint) forwardCompletion(ctx context.Context, session *UpstreamSession, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	for _, entry := range p.sessionConnectionSnapshot(session) {
		conn := entry.conn
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
	for _, entry := range p.sessionConnectionSnapshot(session) {
		conn := entry.conn
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

func (p *CentianEndpoint) desiredResourceState(session *UpstreamSession) (map[string]resourceEntry, map[string][]string) {
	desiredResources := make(map[string]resourceEntry)
	if session == nil {
		return desiredResources, map[string][]string{}
	}

	if !p.isAggregatedProxy {
		for _, entry := range p.sessionConnectionSnapshot(session) {
			serverName := entry.serverName
			conn := entry.conn
			if !conn.IsConnected() {
				continue
			}
			for _, resource := range conn.Resources() {
				if resource == nil {
					continue
				}
				desiredResources[resource.URI] = resourceEntry{resource: resource, serverName: serverName}
			}
		}
		return desiredResources, map[string][]string{}
	}

	owners := make(map[string]map[string]struct{})
	entries := make(map[string]resourceEntry)
	for _, entry := range p.sessionConnectionSnapshot(session) {
		serverName := entry.serverName
		conn := entry.conn
		if !conn.IsConnected() {
			continue
		}
		for _, resource := range conn.Resources() {
			if resource == nil {
				continue
			}
			if owners[resource.URI] == nil {
				owners[resource.URI] = make(map[string]struct{})
				entries[resource.URI] = resourceEntry{resource: resource, serverName: serverName}
			}
			owners[resource.URI][serverName] = struct{}{}
		}
	}

	collisions := make(map[string][]string)
	for uri, entry := range entries {
		servers := sortedServerSet(owners[uri])
		if len(servers) > 1 {
			collisions[uri] = servers
			continue
		}
		desiredResources[uri] = entry
	}
	return desiredResources, collisions
}

func (p *CentianEndpoint) desiredResourceTemplateState(session *UpstreamSession) (map[string]resourceTemplateEntry, map[string][]string) {
	desiredTemplates := make(map[string]resourceTemplateEntry)
	if session == nil {
		return desiredTemplates, map[string][]string{}
	}

	if !p.isAggregatedProxy {
		for _, entry := range p.sessionConnectionSnapshot(session) {
			serverName := entry.serverName
			conn := entry.conn
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
		return desiredTemplates, map[string][]string{}
	}

	owners := make(map[string]map[string]struct{})
	entries := make(map[string]resourceTemplateEntry)
	for _, entry := range p.sessionConnectionSnapshot(session) {
		serverName := entry.serverName
		conn := entry.conn
		if !conn.IsConnected() {
			continue
		}
		for _, resourceTemplate := range conn.ResourceTemplates() {
			if resourceTemplate == nil {
				continue
			}
			if owners[resourceTemplate.URITemplate] == nil {
				owners[resourceTemplate.URITemplate] = make(map[string]struct{})
				entries[resourceTemplate.URITemplate] = resourceTemplateEntry{
					resourceTemplate: resourceTemplate,
					serverName:       serverName,
				}
			}
			owners[resourceTemplate.URITemplate][serverName] = struct{}{}
		}
	}

	collisions := make(map[string][]string)
	for uriTemplate, entry := range entries {
		servers := sortedServerSet(owners[uriTemplate])
		if len(servers) > 1 {
			collisions[uriTemplate] = servers
			continue
		}
		desiredTemplates[uriTemplate] = entry
	}
	return desiredTemplates, collisions
}

func (p *CentianEndpoint) updatePoolResourceCollisions(downstreamSessionKey string, next map[string][]string) {
	pool := p.downstreamPool(downstreamSessionKey)
	if pool == nil {
		return
	}

	pool.collisionMu.Lock()
	defer pool.collisionMu.Unlock()

	previous := cloneCollisionMap(pool.resourceCollisions)
	p.logCollisionStateChanges("resource URI", previous, next)
	pool.resourceCollisions = cloneCollisionMap(next)
}

func (p *CentianEndpoint) updatePoolResourceTemplateCollisions(downstreamSessionKey string, next map[string][]string) {
	pool := p.downstreamPool(downstreamSessionKey)
	if pool == nil {
		return
	}

	pool.collisionMu.Lock()
	defer pool.collisionMu.Unlock()

	previous := cloneCollisionMap(pool.resourceTemplateCollisions)
	p.logCollisionStateChanges("resource template", previous, next)
	pool.resourceTemplateCollisions = cloneCollisionMap(next)
}

func (p *CentianEndpoint) logCollisionStateChanges(kind string, previous, next map[string][]string) {
	for _, key := range sortedCollisionKeys(next) {
		servers := next[key]
		if prevServers, ok := previous[key]; ok && slices.Equal(prevServers, servers) {
			continue
		}
		common.LogWarn(
			"ProxyEndpoint[%s]: aggregated %s %q collides across downstreams [%s]; omitting it from the upstream surface",
			p.name,
			kind,
			key,
			strings.Join(servers, ", "),
		)
	}

	for _, key := range sortedCollisionKeys(previous) {
		if _, stillColliding := next[key]; stillColliding {
			continue
		}
		common.LogInfo(
			"ProxyEndpoint[%s]: aggregated %s %q collision resolved; advertising it again if a single downstream still exposes it",
			p.name,
			kind,
			key,
		)
	}
}

func (p *CentianEndpoint) collidingServersForResourceURI(downstreamSessionKey, uri string) []string {
	pool := p.downstreamPool(downstreamSessionKey)
	if pool == nil {
		return nil
	}

	pool.collisionMu.RLock()
	defer pool.collisionMu.RUnlock()

	return slices.Clone(pool.resourceCollisions[uri])
}

func (p *CentianEndpoint) downstreamPool(downstreamSessionKey string) *DownstreamSessionPool {
	if downstreamSessionKey == "" {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.downstreamPools[downstreamSessionKey]
}

func cloneCollisionMap(source map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(source))
	for key, servers := range source {
		cloned[key] = slices.Clone(servers)
	}
	return cloned
}

func sortedCollisionKeys(collisions map[string][]string) []string {
	keys := make([]string, 0, len(collisions))
	for key := range collisions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedServerSet(serverSet map[string]struct{}) []string {
	servers := make([]string, 0, len(serverSet))
	for serverName := range serverSet {
		servers = append(servers, serverName)
	}
	sort.Strings(servers)
	return servers
}
