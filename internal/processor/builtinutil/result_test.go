package builtinutil

import (
	"net/http"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"gotest.tools/assert"
)

func TestAppendReportInitializesAnnotations(t *testing.T) {
	ctx := &DataContext{}

	AppendReport(ctx, &common.EventAnnotation{
		Processor: "test",
		Action:    "annotated",
	})

	assert.Assert(t, ctx.Annotations != nil)
	assert.Equal(t, len(ctx.Annotations.Reports), 1)
	assert.Equal(t, ctx.Annotations.Reports[0].Processor, "test")
}

func TestMarkModifiedInitializesEvent(t *testing.T) {
	ctx := &DataContext{}

	MarkModified(ctx)

	assert.Assert(t, ctx.Event != nil)
	assert.Assert(t, ctx.Event.Modified)
}

func TestBlockWithTextResultInitializesPayloadAndEvent(t *testing.T) {
	ctx := &DataContext{}

	BlockWithTextResult(ctx, BlockResultOptions{
		Processor: "guard",
		Message:   "blocked",
		Status:    http.StatusForbidden,
		StructuredContent: map[string]any{
			"blocked": true,
		},
	})

	assert.Assert(t, ctx.Payload != nil)
	assert.Assert(t, ctx.Payload.Result != nil)
	assert.Assert(t, ctx.Payload.Result.IsError)
	assert.Equal(t, ctx.Event.Status, http.StatusForbidden)
	assert.Assert(t, !ctx.Event.Success)
	assert.Assert(t, ctx.Event.Modified)

	structured := ctx.Payload.Result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["blocked"], true)
	assert.Equal(t, structured["processor"], "guard")
}
