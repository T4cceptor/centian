package proxy

import (
	"context"
	"testing"

	"gotest.tools/assert"
)

func TestCallContextFromContext(t *testing.T) {
	var nilCtx context.Context
	callCtx, ok := CallContextFromContext(nilCtx)
	assert.Assert(t, callCtx == nil)
	assert.Assert(t, !ok)

	callCtx, ok = CallContextFromContext(context.Background())
	assert.Assert(t, callCtx == nil)
	assert.Assert(t, !ok)

	expected := &mockCallContext{}
	ctx := WithCallContext(context.Background(), expected)
	callCtx, ok = CallContextFromContext(ctx)
	assert.Assert(t, ok)
	assert.Assert(t, callCtx == expected)
}
