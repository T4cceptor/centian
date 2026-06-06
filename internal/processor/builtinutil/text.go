package builtinutil

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TextNode identifies one string value found in processor input.
type TextNode struct {
	Path string
	Text string
}

// ArgumentNode identifies one request argument JSON value.
type ArgumentNode struct {
	Path   string
	Value  any
	Text   string
	Scalar bool
}

// TextReplacement is returned by string mutation callbacks.
type TextReplacement struct {
	Text string
	Keep bool
}

// KeepText returns a replacement that keeps a string value.
func KeepText(text string) TextReplacement {
	return TextReplacement{Text: text, Keep: true}
}

// DropText returns a replacement that removes a string value where removal is supported.
func DropText() TextReplacement {
	return TextReplacement{Keep: false}
}

// WalkRequestArguments visits all string values in payload.request.Params.arguments.
func WalkRequestArguments(ctx *DataContext, visit func(TextNode)) error {
	value, ok, err := requestArgumentsValue(ctx)
	if err != nil || !ok {
		return err
	}
	walkTextValue(value, "payload.request.Params.arguments", visit)
	return nil
}

// WalkRequestArgumentValues visits all JSON values in payload.request.Params.arguments.
// Paths are relative to payload.request.Params.arguments.
func WalkRequestArgumentValues(ctx *DataContext, visit func(ArgumentNode)) error {
	value, ok, err := requestArgumentsValue(ctx)
	if err != nil || !ok {
		return err
	}
	walkArgumentValue(value, "", visit)
	return nil
}

// ReplaceRequestArgumentStrings replaces or removes string values in request arguments.
func ReplaceRequestArgumentStrings(ctx *DataContext, replace func(TextNode) TextReplacement) (bool, error) {
	value, ok, err := requestArgumentsValue(ctx)
	if err != nil || !ok {
		return false, err
	}

	updated, keep, changed := replaceTextValue(value, "payload.request.Params.arguments", replace)
	params := ctx.Payload.Request.Params
	if !keep {
		params.Arguments = nil
		return true, nil
	}
	if !changed {
		return false, nil
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		return false, err
	}
	params.Arguments = raw
	return true, nil
}

// WalkResultText visits text content blocks in payload.result.content.
func WalkResultText(ctx *DataContext, visit func(TextNode)) {
	result := resultPayload(ctx)
	if result == nil {
		return
	}
	for i, item := range result.Content {
		text, ok := item.(*mcp.TextContent)
		if !ok {
			continue
		}
		visit(TextNode{
			Path: fmt.Sprintf("payload.result.content[%d].text", i),
			Text: text.Text,
		})
	}
}

// ReplaceResultText replaces or removes text content blocks in payload.result.content.
func ReplaceResultText(ctx *DataContext, replace func(TextNode) TextReplacement) bool {
	result := resultPayload(ctx)
	if result == nil {
		return false
	}

	changed := false
	content := make([]mcp.Content, 0, len(result.Content))
	for i, item := range result.Content {
		text, ok := item.(*mcp.TextContent)
		if !ok {
			content = append(content, item)
			continue
		}
		next := replace(TextNode{
			Path: fmt.Sprintf("payload.result.content[%d].text", i),
			Text: text.Text,
		})
		if !next.Keep {
			changed = true
			continue
		}
		if next.Text != text.Text {
			clone := *text
			clone.Text = next.Text
			content = append(content, &clone)
			changed = true
			continue
		}
		content = append(content, item)
	}
	if changed {
		result.Content = content
	}
	return changed
}

// WalkStructuredContent visits all string values in payload.result.structuredContent.
func WalkStructuredContent(ctx *DataContext, visit func(TextNode)) {
	result := resultPayload(ctx)
	if result == nil || result.StructuredContent == nil {
		return
	}
	walkTextValue(result.StructuredContent, "payload.result.structuredContent", visit)
}

// ReplaceStructuredContentStrings replaces or removes string values in structured content.
func ReplaceStructuredContentStrings(ctx *DataContext, replace func(TextNode) TextReplacement) bool {
	result := resultPayload(ctx)
	if result == nil || result.StructuredContent == nil {
		return false
	}
	updated, keep, changed := replaceTextValue(result.StructuredContent, "payload.result.structuredContent", replace)
	if !keep {
		result.StructuredContent = nil
		return true
	}
	if changed {
		result.StructuredContent = updated
	}
	return changed
}

// TotalScannedTextBytes follows the current built-in convention: result text when
// a result exists, otherwise request argument text.
func TotalScannedTextBytes(ctx *DataContext) int {
	if resultPayload(ctx) != nil {
		return ResultTextBytes(ctx)
	}
	return RequestArgumentTextBytes(ctx)
}

// RequestArgumentTextBytes counts bytes from all request argument strings.
func RequestArgumentTextBytes(ctx *DataContext) int {
	value, ok, err := requestArgumentsValue(ctx)
	if err != nil || !ok {
		return 0
	}
	return textValueBytes(value)
}

// ResultTextBytes counts bytes from result text content and structured content strings.
func ResultTextBytes(ctx *DataContext) int {
	result := resultPayload(ctx)
	if result == nil {
		return 0
	}
	total := 0
	WalkResultText(ctx, func(node TextNode) {
		total += len(node.Text)
	})
	total += textValueBytes(result.StructuredContent)
	return total
}

func requestArgumentsValue(ctx *DataContext) (any, bool, error) {
	if ctx == nil || ctx.Payload == nil || ctx.Payload.Request == nil || ctx.Payload.Request.Params == nil {
		return nil, false, nil
	}
	raw := ctx.Payload.Request.Params.Arguments
	if len(raw) == 0 {
		return nil, false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func resultPayload(ctx *DataContext) *mcp.CallToolResult {
	if ctx == nil || ctx.Payload == nil {
		return nil
	}
	return ctx.Payload.Result
}

func walkTextValue(value any, path string, visit func(TextNode)) {
	switch typed := value.(type) {
	case string:
		visit(TextNode{Path: path, Text: typed})
	case []any:
		for i, item := range typed {
			walkTextValue(item, fmt.Sprintf("%s[%d]", path, i), visit)
		}
	case map[string]any:
		keys := sortedKeys(typed)
		for _, key := range keys {
			walkTextValue(typed[key], path+"."+key, visit)
		}
	}
}

func walkArgumentValue(value any, path string, visit func(ArgumentNode)) {
	if text, ok := scalarText(value); ok {
		visit(ArgumentNode{Path: path, Value: value, Text: text, Scalar: true})
		return
	}
	visit(ArgumentNode{Path: path, Value: value})
	switch typed := value.(type) {
	case []any:
		for i, item := range typed {
			walkArgumentValue(item, fmt.Sprintf("%s[%d]", path, i), visit)
		}
	case map[string]any:
		keys := sortedKeys(typed)
		for _, key := range keys {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			walkArgumentValue(typed[key], childPath, visit)
		}
	}
}

func scalarText(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "null", true
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	default:
		return "", false
	}
}

func replaceTextValue(value any, path string, replace func(TextNode) TextReplacement) (any, bool, bool) {
	switch typed := value.(type) {
	case string:
		next := replace(TextNode{Path: path, Text: typed})
		if !next.Keep {
			return nil, false, true
		}
		return next.Text, true, next.Text != typed
	case []any:
		changed := false
		out := make([]any, 0, len(typed))
		for i, item := range typed {
			next, keep, childChanged := replaceTextValue(item, fmt.Sprintf("%s[%d]", path, i), replace)
			changed = changed || childChanged
			if keep {
				out = append(out, next)
			}
		}
		if len(out) != len(typed) {
			changed = true
		}
		return out, true, changed
	case map[string]any:
		changed := false
		out := make(map[string]any, len(typed))
		keys := sortedKeys(typed)
		for _, key := range keys {
			next, keep, childChanged := replaceTextValue(typed[key], path+"."+key, replace)
			changed = changed || childChanged
			if keep {
				out[key] = next
			}
		}
		if len(out) != len(typed) {
			changed = true
		}
		return out, true, changed
	default:
		return typed, true, false
	}
}

func textValueBytes(value any) int {
	switch typed := value.(type) {
	case string:
		return len(typed)
	case []any:
		total := 0
		for _, item := range typed {
			total += textValueBytes(item)
		}
		return total
	case map[string]any:
		total := 0
		for _, item := range typed {
			total += textValueBytes(item)
		}
		return total
	default:
		return 0
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
