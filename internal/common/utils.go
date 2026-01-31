// Package common holds functions and structs that are used throughout all other
// packages in this repository.
// It mainly provides utils functions, and MCP models.
package common

import (
	"os"
	"regexp"
	"time"
)

// GetCurrentWorkingDir gets the current working directory.
func GetCurrentWorkingDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return pwd
}

// GetSecondsFromInt returns a duration (in seconds) for a provided int value.
func GetSecondsFromInt(i int) time.Duration {
	return time.Duration(i) * time.Second
}

// IsURLCompliant checks if a name is URL-safe (alphanumeric, dash, underscore only).
// Names must start with alphanumeric character and can contain alphanumeric, dash, or underscore.
// This ensures names can be safely used in URL paths like /mcp/<gateway>/<server>.
func IsURLCompliant(name string) bool {
	if name == "" {
		return false
	}
	// Pattern: start with alphanumeric, followed by alphanumeric/dash/underscore.
	pattern := `^[a-zA-Z0-9\/][a-zA-Z0-9_-]*$`
	matched, _ := regexp.MatchString(pattern, name)
	return matched
}
