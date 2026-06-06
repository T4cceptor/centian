package builtinutil

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestRequestArgumentWalkAndReplace(t *testing.T) {
	ctx := contextWithRequestArgs(t, map[string]any{
		"query": "secret",
		"nested": map[string]any{
			"keep": "visible",
		},
		"list": []any{"one", "drop"},
	})

	var paths []string
	err := WalkRequestArguments(ctx, func(node TextNode) {
		paths = append(paths, node.Path)
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, paths, []string{
		"payload.request.Params.arguments.list[0]",
		"payload.request.Params.arguments.list[1]",
		"payload.request.Params.arguments.nested.keep",
		"payload.request.Params.arguments.query",
	})

	changed, err := ReplaceRequestArgumentStrings(ctx, func(node TextNode) TextReplacement {
		if node.Text == "secret" {
			return KeepText("redacted")
		}
		if node.Text == "drop" {
			return DropText()
		}
		return KeepText(node.Text)
	})
	assert.NilError(t, err)
	assert.Assert(t, changed)

	args := decodeArgs(t, ctx)
	assert.Equal(t, args["query"], "redacted")
	assert.DeepEqual(t, args["list"], []any{"one"})
}

func TestResultTextWalkAndReplace(t *testing.T) {
	ctx := &DataContext{
		Payload: &PayloadPart{
			Result: &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "first"},
					&mcp.ImageContent{MIMEType: "image/png"},
					&mcp.TextContent{Text: "remove"},
				},
			},
		},
	}

	var nodes []TextNode
	WalkResultText(ctx, func(node TextNode) {
		nodes = append(nodes, node)
	})
	assert.DeepEqual(t, nodes, []TextNode{
		{Path: "payload.result.content[0].text", Text: "first"},
		{Path: "payload.result.content[2].text", Text: "remove"},
	})

	changed := ReplaceResultText(ctx, func(node TextNode) TextReplacement {
		if node.Text == "first" {
			return KeepText("updated")
		}
		return DropText()
	})
	assert.Assert(t, changed)
	assert.Equal(t, len(ctx.Payload.Result.Content), 2)
	assert.Equal(t, ctx.Payload.Result.Content[0].(*mcp.TextContent).Text, "updated")
	_, ok := ctx.Payload.Result.Content[1].(*mcp.ImageContent)
	assert.Assert(t, ok)
}

func TestStructuredContentWalkAndReplace(t *testing.T) {
	ctx := &DataContext{
		Payload: &PayloadPart{
			Result: &mcp.CallToolResult{
				StructuredContent: map[string]any{
					"message": "secret",
					"items":   []any{"keep", "drop"},
				},
			},
		},
	}

	var paths []string
	WalkStructuredContent(ctx, func(node TextNode) {
		paths = append(paths, node.Path)
	})
	assert.DeepEqual(t, paths, []string{
		"payload.result.structuredContent.items[0]",
		"payload.result.structuredContent.items[1]",
		"payload.result.structuredContent.message",
	})

	changed := ReplaceStructuredContentStrings(ctx, func(node TextNode) TextReplacement {
		if node.Text == "secret" {
			return KeepText("redacted")
		}
		if node.Text == "drop" {
			return DropText()
		}
		return KeepText(node.Text)
	})
	assert.Assert(t, changed)
	structured := ctx.Payload.Result.StructuredContent.(map[string]any)
	assert.Equal(t, structured["message"], "redacted")
	assert.DeepEqual(t, structured["items"], []any{"keep"})
}

func TestTextByteCounts(t *testing.T) {
	requestCtx := contextWithRequestArgs(t, map[string]any{"a": "one", "b": []any{"two"}})
	assert.Equal(t, RequestArgumentTextBytes(requestCtx), len("onetwo"))
	assert.Equal(t, TotalScannedTextBytes(requestCtx), len("onetwo"))

	resultCtx := &DataContext{
		Payload: &PayloadPart{
			Result: &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "abc"}},
				StructuredContent: map[string]any{"d": "ef"},
			},
		},
	}
	assert.Equal(t, ResultTextBytes(resultCtx), len("abcef"))
	assert.Equal(t, TotalScannedTextBytes(resultCtx), len("abcef"))
}

func contextWithRequestArgs(t *testing.T, args any) *DataContext {
	t.Helper()
	raw, err := json.Marshal(args)
	assert.NilError(t, err)
	return &DataContext{
		Payload: &PayloadPart{
			Request: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      "tool",
					Arguments: raw,
				},
			},
		},
	}
}

func decodeArgs(t *testing.T, ctx *DataContext) map[string]any {
	t.Helper()
	var args map[string]any
	err := json.Unmarshal(ctx.Payload.Request.Params.Arguments, &args)
	assert.NilError(t, err)
	return args
}
