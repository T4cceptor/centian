package auth

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/assert"
)

// Given: a DirectGrantAuthorizer and principals with various grant lists
// When: authorizing gateway/project access
// Then: empty list = allow-all, "*" = allow-all, otherwise case-insensitive match,
// and unknown resource types / nil principals are denied.
func TestDirectGrantAuthorizer(t *testing.T) {
	authz := DirectGrantAuthorizer{}
	ctx := context.Background()

	t.Run("empty allowlist allows all gateways", func(t *testing.T) {
		p := &Principal{ID: "pr_1", Gateways: nil}
		assert.NilError(t, authz.Authorize(ctx, p, ActionAccess, GatewayResource("default")))
		assert.NilError(t, authz.Authorize(ctx, p, ActionAccess, GatewayResource("other")))
	})

	t.Run("wildcard allows all gateways", func(t *testing.T) {
		p := &Principal{ID: "pr_1", Gateways: []string{"*"}}
		assert.NilError(t, authz.Authorize(ctx, p, ActionAccess, GatewayResource("anything")))
	})

	t.Run("explicit gateway list enforced case-insensitively", func(t *testing.T) {
		p := &Principal{ID: "pr_1", Gateways: []string{"Alpha"}}
		assert.NilError(t, authz.Authorize(ctx, p, ActionAccess, GatewayResource("alpha")))
		assert.Assert(t, errors.Is(authz.Authorize(ctx, p, ActionAccess, GatewayResource("beta")), ErrUnauthorized))
	})

	t.Run("project grants are independent of gateway grants", func(t *testing.T) {
		p := &Principal{ID: "pr_1", Gateways: []string{"alpha"}, Projects: []string{"acme"}}
		assert.NilError(t, authz.Authorize(ctx, p, ActionAccess, ProjectResource("acme")))
		assert.Assert(t, errors.Is(authz.Authorize(ctx, p, ActionAccess, ProjectResource("other")), ErrUnauthorized))
		// gateway list must not leak into project decisions
		assert.Assert(t, errors.Is(authz.Authorize(ctx, p, ActionAccess, ProjectResource("alpha")), ErrUnauthorized))
	})

	t.Run("nil principal is denied", func(t *testing.T) {
		assert.Assert(t, errors.Is(authz.Authorize(ctx, nil, ActionAccess, GatewayResource("default")), ErrUnauthorized))
	})

	t.Run("unknown resource type is denied", func(t *testing.T) {
		p := &Principal{ID: "pr_1"}
		assert.Assert(t, errors.Is(authz.Authorize(ctx, p, ActionAccess, Resource{Type: "secret", ID: "x"}), ErrUnauthorized))
	})
}
