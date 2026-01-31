package discovery

import (
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

func TestGetDefaultRegexDiscoverer(t *testing.T) {
	// Given: default discoverer
	discoverer := GetDefaultRegexDiscoverer()

	// Then: default settings are populated
	assert.Equal(t, discoverer.DiscovererName, "Regex MCP Config Discoverer")
	assert.Assert(t, len(discoverer.Patterns) > 0)
	assert.Assert(t, len(discoverer.SearchPaths) > 0)
	assert.Equal(t, discoverer.MaxDepth, 7)
	assert.Assert(t, discoverer.Enabled)
}

func TestGetDefaultPatterns(t *testing.T) {
	// Given: default patterns
	patterns := GetDefaultPatterns()

	// Then: patterns include high-priority entries
	assert.Assert(t, len(patterns) > 0)
	foundClaude := false
	for _, pattern := range patterns {
		if pattern.FileRegex == `claude_desktop_config\.json$` {
			foundClaude = true
		}
	}
	assert.Assert(t, foundClaude)
}

func TestGetExcludedDirectories(t *testing.T) {
	// Given: excluded directories for current OS
	excluded := getExcludedDirectories()

	// Then: list is non-empty for known platforms
	assert.Assert(t, len(excluded) > 0)
}

func TestGetDefaultSearchPaths(t *testing.T) {
	// Given: a temp home directory
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// When: resolving search paths
	paths := GetDefaultSearchPaths()

	// Then: paths include common defaults
	assert.Assert(t, containsPath(paths, "./"))
	assert.Assert(t, containsPath(paths, ".vscode"))
	assert.Assert(t, containsPath(paths, tempHome))
}

func TestGetPriorityPatterns(t *testing.T) {
	// Given: priority patterns
	patterns := GetPriorityPatterns()

	// Then: patterns are sorted by priority descending
	for i := 0; i < len(patterns)-1; i++ {
		assert.Assert(t, patterns[i].Priority >= patterns[i+1].Priority)
	}
}

func TestCreateCustomRegexDiscoverer(t *testing.T) {
	// Given: custom patterns and no search paths
	patterns := []Pattern{{FileRegex: `config\.json$`, Priority: 1}}

	// When: creating a discoverer without search paths
	discoverer := CreateCustomRegexDiscoverer("name", "desc", patterns, nil)

	// Then: defaults are applied
	assert.Equal(t, discoverer.DiscovererName, "name")
	assert.Equal(t, discoverer.DiscovererDescription, "desc")
	assert.Equal(t, discoverer.MaxDepth, 3)
	assert.Assert(t, discoverer.Enabled)
	assert.Assert(t, len(discoverer.SearchPaths) > 0)

	// When: creating with explicit search paths
	searchPaths := []string{filepath.Join("/tmp", "path")}
	discoverer = CreateCustomRegexDiscoverer("name", "desc", patterns, searchPaths)

	// Then: custom paths are kept
	assert.Equal(t, discoverer.SearchPaths[0], searchPaths[0])
}
