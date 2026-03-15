package everything

import (
	"errors"
	"os/exec"
	"testing"

	"gotest.tools/assert"
)

func TestIsExpectedDirectCloseError(t *testing.T) {
	assert.Assert(t, isExpectedDirectCloseError(&exec.ExitError{}))
	assert.Assert(t, !isExpectedDirectCloseError(errors.New("plain error")))
	assert.Assert(t, !isExpectedDirectCloseError(nil))
}
