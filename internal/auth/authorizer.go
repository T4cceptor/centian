package auth

import (
	"context"
	"errors"
	"strings"
)

// This file defines the Authorizer seam and its default implementation. The
// authorization decision is centralized here so the inline allowlist behavior
// can later be extended (roles, policies, tenant isolation) without touching the
// call sites. For now DirectGrantAuthorizer replicates the historical
// per-credential allowlist semantics exactly.

// ErrUnauthorized is returned by an Authorizer when a principal is not permitted
// to access a resource.
var ErrUnauthorized = errors.New("principal not authorized for resource")

// Action identifies the kind of operation being authorized. A single access
// action suffices for the current scope; the resource type distinguishes
// gateways from projects.
type Action string

// ActionAccess is the only action modelled today (access a resource).
const ActionAccess Action = "access"

// Resource types understood by the default authorizer.
const (
	ResourceTypeGateway = "gateway"
	ResourceTypeProject = "project"
)

// Resource identifies the target of an authorization decision.
type Resource struct {
	Type string
	ID   string
}

// GatewayResource builds a gateway-scoped resource.
func GatewayResource(name string) Resource {
	return Resource{Type: ResourceTypeGateway, ID: name}
}

// ProjectResource builds a project-scoped resource.
func ProjectResource(slug string) Resource {
	return Resource{Type: ResourceTypeProject, ID: slug}
}

// Authorizer decides whether a principal may perform an action on a resource.
type Authorizer interface {
	Authorize(ctx context.Context, p *Principal, action Action, resource Resource) error
}

// DirectGrantAuthorizer authorizes using the principal's inline direct grants
// (the Gateways/Projects allowlists). It is stateless and safe for concurrent use.
type DirectGrantAuthorizer struct{}

// Authorize permits access when the principal's relevant grant list allows the
// resource id. Roles are not consulted (none exist yet).
func (DirectGrantAuthorizer) Authorize(_ context.Context, p *Principal, _ Action, resource Resource) error {
	if p == nil {
		return ErrUnauthorized
	}
	switch resource.Type {
	case ResourceTypeGateway:
		if allowMatch(p.Gateways, resource.ID) {
			return nil
		}
	case ResourceTypeProject:
		if allowMatch(p.Projects, resource.ID) {
			return nil
		}
	}
	return ErrUnauthorized
}

// allowMatch reports whether an allowlist grants the target.
//
// Backward-compatible semantics (a literal lift of the former
// APIKeyEntry.AllowsGateway/AllowsProject logic):
//   - an empty list means allow-all,
//   - a "*" entry means allow-all,
//   - otherwise a case-insensitive, trimmed equality match is required.
func allowMatch(allowlist []string, target string) bool {
	if len(allowlist) == 0 {
		return true
	}
	for _, entry := range allowlist {
		if entry == "*" || strings.EqualFold(strings.TrimSpace(entry), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
