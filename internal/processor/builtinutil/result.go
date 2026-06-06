package builtinutil

import (
	"net/http"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BlockResultOptions controls the MCP error result injected by a built-in processor.
type BlockResultOptions struct {
	Processor         string
	Message           string
	Status            int
	StructuredContent map[string]any
}

// AppendReport appends an annotation report, initializing the annotation part.
func AppendReport(ctx *DataContext, report common.EventAnnotation) {
	if ctx == nil {
		return
	}
	if ctx.Annotations == nil {
		ctx.Annotations = &AnnotationPart{}
	}
	ctx.Annotations.Reports = append(ctx.Annotations.Reports, report)
}

// MarkModified marks the processor event as modified.
func MarkModified(ctx *DataContext) {
	if ctx == nil {
		return
	}
	ensureEvent(ctx).Modified = true
}

// BlockWithTextResult injects an MCP error result and marks the event failed and modified.
func BlockWithTextResult(ctx *DataContext, options BlockResultOptions) {
	if ctx == nil {
		return
	}
	if ctx.Payload == nil {
		ctx.Payload = &PayloadPart{}
	}
	status := options.Status
	if status == 0 {
		status = http.StatusForbidden
	}
	structured := options.StructuredContent
	if structured == nil {
		structured = map[string]any{}
	}
	if options.Processor != "" {
		structured["processor"] = options.Processor
	}

	ctx.Payload.Result = &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: options.Message},
		},
		IsError:           true,
		StructuredContent: structured,
	}

	event := ensureEvent(ctx)
	event.Status = status
	event.Success = false
	event.Modified = true
}

func ensureEvent(ctx *DataContext) *common.MetaContext {
	if ctx.Event == nil {
		ctx.Event = &common.MetaContext{}
	}
	return ctx.Event
}
