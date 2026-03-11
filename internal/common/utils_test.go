package common

import (
	"os"
	"testing"
	"time"

	"gotest.tools/assert"
)

// ========================================.
// GetSecondsFromInt Tests.
// ========================================.

func TestGetSecondsFromInt_PositiveValue(t *testing.T) {
	// Given: a positive integer.
	seconds := 5

	// When: converting to duration.
	duration := GetSecondsFromInt(seconds)

	// Then: should return correct duration.
	assert.Equal(t, 5*time.Second, duration)
}

func TestGetSecondsFromInt_Zero(t *testing.T) {
	// Given: zero value.
	seconds := 0

	// When: converting to duration.
	duration := GetSecondsFromInt(seconds)

	// Then: should return zero duration.
	assert.Equal(t, 0*time.Second, duration)
}

func TestGetSecondsFromInt_LargeValue(t *testing.T) {
	// Given: a large integer.
	seconds := 3600

	// When: converting to duration.
	duration := GetSecondsFromInt(seconds)

	// Then: should return correct duration (1 hour).
	assert.Equal(t, time.Hour, duration)
}

func TestGetSecondsFromInt_NegativeValue(t *testing.T) {
	// Given: a negative integer.
	seconds := -10

	// When: converting to duration.
	duration := GetSecondsFromInt(seconds)

	// Then: should return negative duration.
	assert.Equal(t, -10*time.Second, duration)
}

// ========================================.
// GetCurrentWorkingDir Tests.
// ========================================.

func TestGetCurrentWorkingDir_ValidDirectory(t *testing.T) {
	// Given: a valid current working directory.
	expectedDir, err := os.Getwd()
	assert.NilError(t, err)

	// When: calling GetCurrentWorkingDir.
	actualDir := GetCurrentWorkingDir()

	// Then: should return the current directory.
	assert.Equal(t, expectedDir, actualDir)
}

func TestGetCurrentWorkingDir_NotEmpty(t *testing.T) {
	// Given: a valid environment.

	// When: calling GetCurrentWorkingDir.
	dir := GetCurrentWorkingDir()

	// Then: should return non-empty string.
	assert.Assert(t, dir != "")
}

func TestIsURLCompliant(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "valid alphanumeric", input: "gateway1", expected: true},
		{name: "valid with dash and underscore", input: "gateway_one-2", expected: true},
		{name: "empty is invalid", input: "", expected: false},
		{name: "starts with dash is invalid", input: "-gateway", expected: false},
		{name: "contains slash is invalid", input: "gateway/name", expected: false},
		{name: "contains space is invalid", input: "bad name", expected: false},
		{name: "contains symbol is invalid", input: "bad$name", expected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, IsURLCompliant(tc.input), tc.expected)
		})
	}
}

func TestGetTransport(t *testing.T) {
	t.Run("both url and cmd set returns invalid transport and error", func(t *testing.T) {
		transport, err := GetTransport("https://example.com/mcp", "python3")
		assert.Equal(t, transport, InvalidTransport)
		assert.Assert(t, err != nil)
	})

	t.Run("url only returns http transport", func(t *testing.T) {
		transport, err := GetTransport("https://example.com/mcp", "")
		assert.Equal(t, transport, HTTPTransport)
		assert.NilError(t, err)
	})

	t.Run("cmd only returns stdio transport", func(t *testing.T) {
		transport, err := GetTransport("", "python3")
		assert.Equal(t, transport, StdioTransport)
		assert.NilError(t, err)
	})

	t.Run("neither url nor cmd returns unknown transport and error", func(t *testing.T) {
		transport, err := GetTransport("", "")
		assert.Equal(t, transport, UnknownTransport)
		assert.Assert(t, err != nil)
	})
}
