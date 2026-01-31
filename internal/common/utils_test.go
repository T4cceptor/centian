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
