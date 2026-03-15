package proxy

import (
	"errors"
	"fmt"
	"strings"
)

const (
	mcpMethodCallTool     = "tools/call"
	mcpMethodReadResource = "resources/read"
	mcpMethodGetPrompt    = "prompts/get"
	mcpMethodComplete     = "completion/complete"
	mcpMethodSubscribe    = "resources/subscribe"
	mcpMethodUnsubscribe  = "resources/unsubscribe"
)

func normalizeForwardedMethodError(method string, err error) error {
	if err == nil {
		return nil
	}

	prefix := fmt.Sprintf("calling %q: ", method)
	if !strings.HasPrefix(err.Error(), prefix) {
		return err
	}

	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		return unwrapped
	}

	inner := strings.TrimPrefix(err.Error(), prefix)
	if inner == "" || inner == err.Error() {
		return err
	}
	return errors.New(inner)
}
