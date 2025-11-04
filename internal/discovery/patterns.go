package discovery

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetDefaultRegexDiscoverer creates a RegexDiscoverer with default patterns for common MCP configs
func GetDefaultRegexDiscoverer() *RegexDiscoverer {
	return &RegexDiscoverer{
		DiscovererName:        "Regex MCP Config Discoverer",
		DiscovererDescription: "Discovers MCP configurations using flexible regex patterns",
		Patterns:              GetDefaultPatterns(),
		SearchPaths:           GetDefaultSearchPaths(),
		MaxDepth:              7,
		Enabled:               true,
	}
}

// GetDefaultPatterns returns the default discovery patterns for common MCP config files
func GetDefaultPatterns() []Pattern {
	return []Pattern{
		// High-priority specific patterns
		{
			FileRegex:    `claude_desktop_config\.json$`,
			ContentRegex: []string{`"mcpServers"`},
			Parser:       "claudeDesktopParser",
			SourceType:   "claude-desktop",
			Priority:     100,
		},
		{
			FileRegex:    `\.vscode/mcp\.json$`,
			ContentRegex: []string{`"servers"`},
			Parser:       "vscodeParser",
			SourceType:   "vscode-mcp",
			Priority:     95,
		},
		{
			FileRegex:    `\.vscode/settings\.json$`,
			ContentRegex: []string{`"mcp\.servers"`},
			Parser:       "settingsParser",
			SourceType:   "vscode-settings",
			Priority:     90,
		},

		// Medium-priority MCP-specific patterns
		{
			FileRegex:    `.*mcp.*\.json$`,
			ContentRegex: []string{`"servers":`, `"command":`, `"url":`},
			Parser:       "detectAndParse",
			SourceType:   "auto-detect",
			Priority:     80,
		},
		{
			FileRegex:    `\.claude/.*\.json$`,
			ContentRegex: []string{`"servers":`, `"mcp"`},
			Parser:       "detectAndParse",
			SourceType:   "claude-code",
			Priority:     75,
		},
		{
			FileRegex:    `\.continue/config\.json$`,
			ContentRegex: []string{`"mcp"`, `"servers"`, `"tools"`},
			Parser:       "genericParser",
			SourceType:   "continue-dev",
			Priority:     70,
		},

		// Claude Code specific patterns
		{
			FileRegex:    `CLAUDE\.md$`,
			ContentRegex: []string{`mcp`, `server`},
			Parser:       "genericParser",
			SourceType:   "claude-code-markdown",
			Priority:     65,
		},

		// Editor-specific patterns
		{
			FileRegex:    `.*/Zed/settings\.json$`,
			ContentRegex: []string{`"mcp"`, `"servers"`},
			Parser:       "settingsParser",
			SourceType:   "zed",
			Priority:     60,
		},
		{
			FileRegex:    `.*/Cursor/.*/settings\.json$`,
			ContentRegex: []string{`"mcp"`, `"servers"`},
			Parser:       "settingsParser",
			SourceType:   "cursor",
			Priority:     55,
		},

		// Generic config patterns with content filtering
		{
			FileRegex:    `.*config\.json$`,
			ContentRegex: []string{`"mcp"`, `"servers".*"command"`, `"mcpServers"`},
			Parser:       "genericParser",
			SourceType:   "auto-detect",
			Priority:     40,
		},
		{
			FileRegex:    `.*settings\.json$`,
			ContentRegex: []string{`"mcp\."`, `"mcpServers"`},
			Parser:       "settingsParser",
			SourceType:   "auto-detect",
			Priority:     30,
		},

		// Lower priority broad patterns
		{
			FileRegex:    `\.mcp/.*\.json$`,
			ContentRegex: []string{`"servers":`, `"command":`, `"url":`},
			Parser:       "detectAndParse",
			SourceType:   "generic-mcp",
			Priority:     20,
		},
	}
}

// getExcludedDirectories returns directories to exclude from search to avoid permission issues
func getExcludedDirectories() []string {
	switch runtime.GOOS {
	case "darwin": // macOS
		return []string{
			"Library/Caches", "Library/Logs", "Library/Mail", "Library/Safari",
			"Library/Keychains", "Library/Group Containers", "Library/Containers",
			"Library/Application Support/com.apple.", "Library/Application Support/AddressBook",
			"Library/Application Support/CallHistoryDB", "Library/Application Support/com.crashlytics",
			"Movies", "Music", "Pictures", // Large media directories
			".Trash", ".cache", ".npm/_cacache", "node_modules", // Cache directories
			"Applications", "System", "private", "usr", "bin", "sbin", "etc", // System directories
		}
	case "linux":
		return []string{
			".cache", ".npm/_cacache", "node_modules", // Cache directories
			".local/share/Trash", ".gvfs", ".dbus", // System directories
			"snap", ".snap", ".mozilla", ".thunderbird", // App data
			"/proc", "/sys", "/dev", "/run", "/tmp", // System mount points
		}
	case "windows":
		return []string{
			"AppData/Local/Temp", "AppData/Local/Microsoft", "AppData/Local/Google",
			"AppData/Roaming/Microsoft", "AppData/Local/Packages",
			"ntuser.dat", "NTUSER.DAT", // Windows user registry files
		}
	default:
		return []string{}
	}
}

// GetDefaultSearchPaths returns the default paths to search for MCP configurations
func GetDefaultSearchPaths() []string {
	homeDir, _ := os.UserHomeDir()

	paths := []string{
		// Current directory and common project locations
		"./",
		"./.vscode",
		"./.claude",
		"./.continue",
		"./.mcp",

		// Search entire home directory (exclusions handled in shouldSkipDirectory)
		homeDir,
	}

	// Add platform-specific config directories
	switch runtime.GOOS {
	case "darwin": // macOS
		paths = append(paths,
			filepath.Join(homeDir, "Library", "Application Support", "Claude"),
			filepath.Join(homeDir, "Library", "Application Support", "Code", "User"),
			filepath.Join(homeDir, "Library", "Application Support", "Cursor", "User"),
			filepath.Join(homeDir, "Library", "Application Support", "Zed"),
			filepath.Join(homeDir, ".claude"),
			filepath.Join(homeDir, ".config"),
			filepath.Join(homeDir, ".continue"),
			filepath.Join(homeDir, ".mcp"),
		)
	case "linux":
		paths = append(paths,
			filepath.Join(homeDir, ".config", "Code", "User"),
			filepath.Join(homeDir, ".config", "Cursor", "User"),
			filepath.Join(homeDir, ".config", "zed"),
			filepath.Join(homeDir, ".config", "claude"),
			filepath.Join(homeDir, ".claude"),
			filepath.Join(homeDir, ".continue"),
			filepath.Join(homeDir, ".mcp"),
		)
	case "windows":
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")
		if appData != "" {
			paths = append(paths,
				filepath.Join(appData, "Claude"),
				filepath.Join(appData, "Code", "User"),
				filepath.Join(appData, "Cursor", "User"),
			)
		}
		if localAppData != "" {
			paths = append(paths,
				filepath.Join(localAppData, "Zed"),
			)
		}
		if homeDir != "" {
			paths = append(paths,
				filepath.Join(homeDir, ".claude"),
				filepath.Join(homeDir, ".continue"),
				filepath.Join(homeDir, ".mcp"),
			)
		}
	}

	return paths
}

// GetPriorityPatterns returns patterns sorted by priority (highest first)
func GetPriorityPatterns() []Pattern {
	patterns := GetDefaultPatterns()

	// Sort by priority (higher first)
	for i := 0; i < len(patterns); i++ {
		for j := 0; j < len(patterns)-1-i; j++ {
			if patterns[j].Priority < patterns[j+1].Priority {
				patterns[j], patterns[j+1] = patterns[j+1], patterns[j]
			}
		}
	}

	return patterns
}

// CreateCustomRegexDiscoverer creates a RegexDiscoverer with custom patterns
func CreateCustomRegexDiscoverer(name, description string, patterns []Pattern, searchPaths []string) *RegexDiscoverer {
	if len(searchPaths) == 0 {
		searchPaths = GetDefaultSearchPaths()
	}

	return &RegexDiscoverer{
		DiscovererName:        name,
		DiscovererDescription: description,
		Patterns:              patterns,
		SearchPaths:           searchPaths,
		MaxDepth:              3,
		Enabled:               true,
	}
}
