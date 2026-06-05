package cli

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/assert"
)

// Given: input containing a name line
// When: prompting for a line
// Then: the label is written and the trimmed name is returned.
func TestPromptLineReadsTrimmedInput(t *testing.T) {
	var out bytes.Buffer
	name, err := promptLine(strings.NewReader("  CI deploy bot \n"), &out, "Enter a name: ")

	assert.NilError(t, err)
	assert.Equal(t, name, "CI deploy bot")
	assert.Equal(t, out.String(), "Enter a name: ")
}

// Given: empty (EOF) input, as with a non-interactive invocation
// When: prompting for a line
// Then: an empty string is returned without error.
func TestPromptLineEmptyOnEOF(t *testing.T) {
	var out bytes.Buffer
	name, err := promptLine(strings.NewReader(""), &out, "Enter a name: ")

	assert.NilError(t, err)
	assert.Equal(t, name, "")
}
