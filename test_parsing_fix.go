package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CentianAI/centian-cli/internal/discovery"
)

func main() {
	// Read the actual MCP config file
	configPath := "/Users/brb/_devspace/centian-cli/.vscode/mcp.json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	// Test the parseVSCodeConfig function directly
	discoverer := &discovery.RegexDiscoverer{}

	// Use reflection to call the private method
	// For now, let's manually parse the JSON to test the logic
	var config struct {
		Servers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Type    string            `json:"type"`
			Headers map[string]string `json:"headers"`
		} `json:"servers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return
	}

	fmt.Printf("Found %d servers in config:\n", len(config.Servers))

	for name, serverConfig := range config.Servers {
		fmt.Printf("\nServer: %s\n", name)
		fmt.Printf("  Command: %s\n", serverConfig.Command)
		fmt.Printf("  URL: %s\n", serverConfig.URL)
		fmt.Printf("  Type: %s\n", serverConfig.Type)
		if len(serverConfig.Headers) > 0 {
			fmt.Printf("  Headers: %v\n", serverConfig.Headers)
		}

		// Test the filtering logic
		if serverConfig.Command == "" && serverConfig.URL == "" {
			fmt.Printf("  ❌ Would be SKIPPED (no command or URL)\n")
		} else {
			fmt.Printf("  ✅ Would be INCLUDED\n")

			transport := "stdio"
			var url string

			if serverConfig.URL != "" {
				transport = "http"
				url = serverConfig.URL
			}

			fmt.Printf("  Transport: %s\n", transport)
			if url != "" {
				fmt.Printf("  Final URL: %s\n", url)
			}
		}
	}
}