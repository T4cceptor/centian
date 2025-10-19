package proxy

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNewStdioProxy tests the creation of a new stdio proxy
func TestNewStdioProxy(t *testing.T) {
	// Given: a context and command parameters
	ctx := context.Background()
	command := "echo"
	args := []string{"test"}

	// When: creating a new stdio proxy
	proxy, err := NewStdioProxy(ctx, command, args)

	// Then: the proxy should be created successfully
	if err != nil {
		t.Fatalf("Failed to create stdio proxy: %v", err)
	}

	if proxy == nil {
		t.Fatal("Proxy should not be nil")
	}

	if proxy.command != command {
		t.Errorf("Expected command %s, got %s", command, proxy.command)
	}

	if len(proxy.args) != len(args) {
		t.Errorf("Expected %d args, got %d", len(args), len(proxy.args))
	}

	if proxy.sessionID == "" {
		t.Error("Session ID should not be empty")
	}

	if proxy.serverID == "" {
		t.Error("Server ID should not be empty")
	}

	if proxy.running {
		t.Error("Proxy should not be running initially")
	}
}

// TestStdioProxyStartStop tests starting and stopping the proxy
func TestStdioProxyStartStop(t *testing.T) {
	// Given: a stdio proxy with a simple echo command
	ctx := context.Background()
	proxy, err := NewStdioProxy(ctx, "echo", []string{"test"})
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// When: starting the proxy
	err = proxy.Start()

	// Then: the proxy should start successfully
	if err != nil {
		t.Fatalf("Failed to start proxy: %v", err)
	}

	if !proxy.IsRunning() {
		t.Error("Proxy should be running after start")
	}

	// Wait a moment for the echo command to complete
	time.Sleep(100 * time.Millisecond)

	// When: stopping the proxy
	err = proxy.Stop()

	// Then: the proxy should stop successfully
	if err != nil {
		t.Errorf("Failed to stop proxy: %v", err)
	}

	if proxy.IsRunning() {
		t.Error("Proxy should not be running after stop")
	}
}

// TestStdioProxyWithInvalidCommand tests proxy with invalid command
func TestStdioProxyWithInvalidCommand(t *testing.T) {
	// Given: a context and invalid command
	ctx := context.Background()
	command := "nonexistent_command_12345"
	args := []string{}

	// When: creating a proxy with invalid command
	proxy, err := NewStdioProxy(ctx, command, args)

	// Then: the proxy should be created (validation happens at start)
	if err != nil {
		t.Fatalf("Failed to create proxy with invalid command: %v", err)
	}

	// When: starting the proxy with invalid command
	err = proxy.Start()

	// Then: starting should fail
	if err == nil {
		t.Error("Expected error when starting proxy with invalid command")
		proxy.Stop() // Clean up if somehow it started
	}
}

// TestParseCommand tests the command parsing functionality
func TestParseCommand(t *testing.T) {
	testCases := []struct {
		name            string
		args            []string
		expectedCommand string
		expectedArgs    []string
		expectError     bool
	}{
		{
			name:            "No arguments",
			args:            []string{},
			expectedCommand: "",
			expectedArgs:    nil,
			expectError:     true,
		},
		{
			name:            "Default npx command",
			args:            []string{"@modelcontextprotocol/server-memory"},
			expectedCommand: "npx",
			expectedArgs:    []string{"@modelcontextprotocol/server-memory"},
			expectError:     false,
		},
		{
			name:            "Custom command with args",
			args:            []string{"--cmd", "python", "-m", "my_server"},
			expectedCommand: "python",
			expectedArgs:    []string{"-m", "my_server"},
			expectError:     false,
		},
		{
			name:            "Custom command without args",
			args:            []string{"--cmd", "cat"},
			expectedCommand: "cat",
			expectedArgs:    []string{},
			expectError:     false,
		},
		{
			name:            "Command flag without value",
			args:            []string{"--cmd"},
			expectedCommand: "",
			expectedArgs:    nil,
			expectError:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// When: parsing the command
			command, args, err := ParseCommand(tc.args)

			// Then: the result should match expectations
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if command != tc.expectedCommand {
				t.Errorf("Expected command %s, got %s", tc.expectedCommand, command)
			}

			if len(args) != len(tc.expectedArgs) {
				t.Errorf("Expected %d args, got %d", len(tc.expectedArgs), len(args))
			}

			for i, expected := range tc.expectedArgs {
				if i >= len(args) || args[i] != expected {
					t.Errorf("Expected arg[%d] %s, got %s", i, expected, args[i])
				}
			}
		})
	}
}

// TestStdioProxySessionIDs tests that session and server IDs are unique
func TestStdioProxySessionIDs(t *testing.T) {
	// Given: multiple stdio proxies
	ctx := context.Background()
	proxies := make([]*StdioProxy, 3)

	for i := 0; i < 3; i++ {
		proxy, err := NewStdioProxy(ctx, "echo", []string{"test"})
		if err != nil {
			t.Fatalf("Failed to create proxy %d: %v", i, err)
		}
		proxies[i] = proxy
	}

	// When: checking session and server IDs
	sessionIDs := make(map[string]bool)
	serverIDs := make(map[string]bool)

	for i, proxy := range proxies {
		// Then: session IDs should be unique
		if sessionIDs[proxy.sessionID] {
			t.Errorf("Duplicate session ID found for proxy %d: %s", i, proxy.sessionID)
		}
		sessionIDs[proxy.sessionID] = true

		// Then: server IDs should be unique
		if serverIDs[proxy.serverID] {
			t.Errorf("Duplicate server ID found for proxy %d: %s", i, proxy.serverID)
		}
		serverIDs[proxy.serverID] = true

		// Then: IDs should follow expected format
		if !strings.HasPrefix(proxy.sessionID, "session_") {
			t.Errorf("Session ID should start with 'session_': %s", proxy.sessionID)
		}

		if !strings.HasPrefix(proxy.serverID, "stdio_") {
			t.Errorf("Server ID should start with 'stdio_': %s", proxy.serverID)
		}
	}
}

// TestStdioProxyContextCancellation tests proxy behavior with context cancellation
func TestStdioProxyContextCancellation(t *testing.T) {
	// Given: a cancellable context and a long-running command
	ctx, cancel := context.WithCancel(context.Background())
	proxy, err := NewStdioProxy(ctx, "sleep", []string{"5"})
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// When: starting the proxy
	err = proxy.Start()
	if err != nil {
		t.Fatalf("Failed to start proxy: %v", err)
	}

	// Verify it's running
	if !proxy.IsRunning() {
		t.Error("Proxy should be running")
	}

	// When: cancelling the context
	cancel()

	// Give it time to process the cancellation
	time.Sleep(100 * time.Millisecond)

	// Then: the proxy should handle cancellation gracefully
	proxy.Stop() // Ensure cleanup

	if proxy.IsRunning() {
		t.Error("Proxy should not be running after context cancellation")
	}
}

// TestStdioProxyWait tests the Wait functionality
func TestStdioProxyWait(t *testing.T) {
	// Given: a stdio proxy with a quick command
	ctx := context.Background()
	proxy, err := NewStdioProxy(ctx, "echo", []string{"test"})
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// When: starting the proxy
	err = proxy.Start()
	if err != nil {
		t.Fatalf("Failed to start proxy: %v", err)
	}

	// When: waiting for the proxy to complete
	err = proxy.Wait()

	// Then: the wait should complete without error (echo exits cleanly)
	if err != nil {
		t.Errorf("Wait failed: %v", err)
	}

	// Cleanup
	proxy.Stop()
}

// TestStdioProxyLoggerIntegration tests that the proxy integrates with logger
func TestStdioProxyLoggerIntegration(t *testing.T) {
	// Setup: create temporary directory for logs
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a stdio proxy
	ctx := context.Background()
	proxy, err := NewStdioProxy(ctx, "echo", []string{"test"})
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Then: the proxy should have a logger
	if proxy.logger == nil {
		t.Error("Proxy should have a logger")
	}

	// Cleanup
	if proxy.logger != nil {
		proxy.logger.Close()
	}
}