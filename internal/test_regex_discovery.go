package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func testRegexDiscovery() {
	fmt.Println("Testing Regex MCP Discovery System")
	fmt.Println("===================================")

	// Create discovery manager with our new regex discoverer
	dm := NewDiscoveryManager()

	// List all discoverers
	discoverers := dm.ListDiscoverers()
	fmt.Printf("Available discoverers: %d\n", len(discoverers))
	for i, d := range discoverers {
		fmt.Printf("  %d. %s - %s (available: %s)\n", i+1, d["name"], d["description"], d["available"])
	}

	fmt.Println("\nRunning discovery...")
	result := dm.DiscoverAll()

	fmt.Printf("\nDiscovery Results:\n")
	fmt.Printf("Found %d server(s)\n", len(result.Servers))

	if len(result.Errors) > 0 {
		fmt.Printf("Errors: %d\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Printf("  - %s\n", err)
		}
	}

	// Display discovered servers
	for i, server := range result.Servers {
		fmt.Printf("\n%d. %s\n", i+1, server.Name)
		fmt.Printf("   Source: %s\n", server.Source)
		fmt.Printf("   Path: %s\n", server.SourcePath)
		fmt.Printf("   Transport: %s\n", server.Transport)

		if server.Command != "" {
			fmt.Printf("   Command: %s\n", server.Command)
			if len(server.Args) > 0 {
				fmt.Printf("   Args: %v\n", server.Args)
			}
		}

		if server.URL != "" {
			fmt.Printf("   URL: %s\n", server.URL)
		}

		if len(server.Env) > 0 {
			fmt.Printf("   Env vars: %d\n", len(server.Env))
		}
	}

	// Test specific patterns
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("Testing Specific Patterns:")

	patterns := GetPriorityPatterns()

	fmt.Printf("Default patterns: %d\n", len(patterns))
	for i, pattern := range patterns[:5] { // Show first 5 patterns
		fmt.Printf("  %d. Regex: %s (Priority: %d)\n", i+1, pattern.FileRegex, pattern.Priority)
		fmt.Printf("     Parser: %s, SourceType: %s\n", pattern.Parser, pattern.SourceType)
		if len(pattern.ContentRegex) > 0 {
			fmt.Printf("     Content filters: %v\n", pattern.ContentRegex)
		}
	}

	// Export results as JSON
	if len(result.Servers) > 0 {
		fmt.Println("\nExporting results to discovery_results.json...")
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling JSON: %v\n", err)
		} else {
			err = os.WriteFile("discovery_results.json", jsonData, 0644)
			if err != nil {
				fmt.Printf("Error writing file: %v\n", err)
			} else {
				fmt.Println("Results exported successfully!")
			}
		}
	}
}
