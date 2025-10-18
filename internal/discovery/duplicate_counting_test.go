package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDuplicateCountingLogic(t *testing.T) {
	// Given-When-Then Test Case for Per-File Duplicate Counting
	//
	// GIVEN: Three test config files with specific duplicate patterns:
	//
	// Config 1 (duplicate_test_config1.json):
	// - server-a: HTTP server at localhost:8001 with API_KEY header
	// - server-b: stdio server with npx command and filesystem args
	// - server-c: HTTP server at localhost:9000 (no headers)
	//
	// Config 2 (duplicate_test_config2.json):
	// - server-a: IDENTICAL to Config 1 (same HTTP URL + headers)
	// - server-b: IDENTICAL to Config 1 (same stdio command + args)
	// - server-c: DIFFERENT from Config 1 (HTTP at localhost:9001 vs 9000)
	//
	// Config 3 (duplicate_test_config3.json):
	// - server-a: IDENTICAL to Config 1 & 2 (same HTTP URL + headers)
	// - server-b: DIFFERENT from others (python command vs npx command)
	//
	// WHEN: The duplicate counting logic analyzes all servers across all configs
	// The countDuplicatesPerFile function:
	// 1. Creates config signatures for each server (transport + command + args + env + url + headers)
	// 2. Groups servers by identical config signatures
	// 3. Counts how many servers in each file have duplicates elsewhere
	//
	// THEN: Expected per-file duplicate counts:
	// - Config 1: 2 duplicates (server-a appears in Config 2&3, server-b appears in Config 2)
	// - Config 2: 2 duplicates (server-a appears in Config 1&3, server-b appears in Config 1)
	// - Config 3: 1 duplicate (server-a appears in Config 1&2, server-b is unique)

	testFiles := []struct {
		path            string
		expectedServers int
		expectedHTTP    int
		expectedStdio   int
		expectedDupes   int
	}{
		{
			path:            "../../test_configs/duplicate_test_config1.json",
			expectedServers: 3,
			expectedHTTP:    2,
			expectedStdio:   1,
			expectedDupes:   2, // server-a and server-b have duplicates in other configs
		},
		{
			path:            "../../test_configs/duplicate_test_config2.json",
			expectedServers: 3,
			expectedHTTP:    2,
			expectedStdio:   1,
			expectedDupes:   2, // server-a and server-b have duplicates in other configs
		},
		{
			path:            "../../test_configs/duplicate_test_config3.json",
			expectedServers: 2,
			expectedHTTP:    1,
			expectedStdio:   1,
			expectedDupes:   1, // only server-a has duplicates in other configs
		},
	}

	var allServers []DiscoveredServer

	// Parse all test configs and convert to absolute paths for comparison
	for _, testFile := range testFiles {
		testData, err := os.ReadFile(testFile.path)
		if err != nil {
			t.Fatalf("Failed to read test config file %s: %v", testFile.path, err)
		}

		servers, err := parseVSCodeConfig(testData, testFile.path)
		if err != nil {
			t.Fatalf("Failed to parse test config %s: %v", testFile.path, err)
		}

		// Verify basic counts
		if len(servers) != testFile.expectedServers {
			t.Errorf("Config %s: expected %d servers, got %d", testFile.path, testFile.expectedServers, len(servers))
		}

		httpCount := 0
		stdioCount := 0
		for _, server := range servers {
			switch server.Transport {
			case "http":
				httpCount++
			case "stdio":
				stdioCount++
			}
		}

		if httpCount != testFile.expectedHTTP {
			t.Errorf("Config %s: expected %d HTTP servers, got %d", testFile.path, testFile.expectedHTTP, httpCount)
		}

		if stdioCount != testFile.expectedStdio {
			t.Errorf("Config %s: expected %d stdio servers, got %d", testFile.path, testFile.expectedStdio, stdioCount)
		}

		allServers = append(allServers, servers...)
	}

	// Now test the duplicate counting logic
	// We need to implement a function that counts duplicates per file before global deduplication
	duplicateCounts := countDuplicatesPerFile(allServers)

	for _, testFile := range testFiles {
		// Convert relative path to absolute path for comparison
		absPath, err := filepath.Abs(testFile.path)
		if err != nil {
			t.Fatalf("Failed to get absolute path for %s: %v", testFile.path, err)
		}

		count, exists := duplicateCounts[absPath]
		if !exists {
			t.Errorf("No duplicate count found for config %s (abs: %s)", testFile.path, absPath)
			continue
		}

		if count != testFile.expectedDupes {
			t.Errorf("Config %s: expected %d duplicates, got %d", testFile.path, testFile.expectedDupes, count)
		}
	}
}
