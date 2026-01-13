package common

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"gotest.tools/assert"
)

// ========================================
// GetSecondsFromInt Tests
// ========================================

func TestGetSecondsFromInt_PositiveValue(t *testing.T) {
	// Given: a positive integer
	seconds := 5

	// When: converting to duration
	duration := GetSecondsFromInt(seconds)

	// Then: should return correct duration
	assert.Equal(t, 5*time.Second, duration)
}

func TestGetSecondsFromInt_Zero(t *testing.T) {
	// Given: zero value
	seconds := 0

	// When: converting to duration
	duration := GetSecondsFromInt(seconds)

	// Then: should return zero duration
	assert.Equal(t, 0*time.Second, duration)
}

func TestGetSecondsFromInt_LargeValue(t *testing.T) {
	// Given: a large integer
	seconds := 3600

	// When: converting to duration
	duration := GetSecondsFromInt(seconds)

	// Then: should return correct duration (1 hour)
	assert.Equal(t, time.Hour, duration)
}

func TestGetSecondsFromInt_NegativeValue(t *testing.T) {
	// Given: a negative integer
	seconds := -10

	// When: converting to duration
	duration := GetSecondsFromInt(seconds)

	// Then: should return negative duration
	assert.Equal(t, -10*time.Second, duration)
}

// ========================================
// GetCurrentWorkingDir Tests
// ========================================

func TestGetCurrentWorkingDir_ValidDirectory(t *testing.T) {
	// Given: a valid current working directory
	expectedDir, err := os.Getwd()
	assert.NilError(t, err)

	// When: calling GetCurrentWorkingDir
	actualDir := GetCurrentWorkingDir()

	// Then: should return the current directory
	assert.Equal(t, expectedDir, actualDir)
}

func TestGetCurrentWorkingDir_NotEmpty(t *testing.T) {
	// Given: a valid environment

	// When: calling GetCurrentWorkingDir
	dir := GetCurrentWorkingDir()

	// Then: should return non-empty string
	assert.Assert(t, dir != "")
}

// ========================================
// StreamPrint Tests
// ========================================

func TestStreamPrint_BasicString(t *testing.T) {
	// Given: a basic string to print

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: calling StreamPrint with high speed
	StreamPrint(100.0, "Hello") // High speed to reduce test time

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Then: should print the exact string
	assert.Equal(t, "Hello", output)
}

func TestStreamPrint_WithFormatting(t *testing.T) {
	// Given: a formatted string
	name := "Alice"
	count := 5

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: calling StreamPrint with formatting
	StreamPrint(100.0, "Hello %s, you have %d messages", name, count)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Then: should print the formatted string
	assert.Equal(t, "Hello Alice, you have 5 messages", output)
}

func TestStreamPrint_EmptyString(t *testing.T) {
	// Given: an empty string

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: calling StreamPrint
	StreamPrint(100.0, "")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Then: should print nothing
	assert.Equal(t, "", output)
}

func TestStreamPrint_SpecialCharacters(t *testing.T) {
	// Given: a string with special characters

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: calling StreamPrint
	StreamPrint(100.0, "Line1\nLine2\tTab")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Then: should preserve special characters
	assert.Equal(t, "Line1\nLine2\tTab", output)
}

func TestStreamPrint_SpeedAffectsDelay(t *testing.T) {
	// Given: a string to print with different speeds

	// When: printing with slow speed
	start := time.Now()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	StreamPrint(1.0, "Test") // Slow speed

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Consume output
	io.Copy(io.Discard, r)

	elapsed := time.Since(start)

	// Then: should take measurable time (at least 50ms per char * 4 chars / speed)
	// At speed 1.0: delay = 50ms per char, 4 chars = ~200ms
	minExpected := 150 * time.Millisecond
	assert.Assert(t, elapsed >= minExpected, "expected at least %v, got %v", minExpected, elapsed)
}

func TestStreamPrint_MultipleFormatArgs(t *testing.T) {
	// Given: multiple format arguments

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: calling StreamPrint with multiple args
	StreamPrint(100.0, "%s %d %t %f", "test", 42, true, 3.14)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Then: should format all arguments correctly
	assert.Equal(t, "test 42 true 3.140000", output)
}

func TestStreamPrint_UnicodeCharacters(t *testing.T) {
	// Given: a string with unicode characters

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: calling StreamPrint
	StreamPrint(100.0, "Hello World 🌍")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Then: should handle unicode correctly
	assert.Equal(t, "Hello World 🌍", output)
}
