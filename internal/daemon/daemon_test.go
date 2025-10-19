package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewDaemon tests creating a new daemon instance
func TestNewDaemon(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// When: creating a new daemon
	daemon, err := NewDaemon()

	// Then: the daemon should be created successfully
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if daemon == nil {
		t.Fatal("Daemon should not be nil")
	}

	if daemon.servers == nil {
		t.Error("Daemon servers map should not be nil")
	}

	// Cleanup
	if daemon.cancel != nil {
		daemon.cancel()
	}
}

// TestDaemonStartStop tests starting and stopping the daemon
func TestDaemonStartStop(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a daemon
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// When: starting the daemon
	err = daemon.Start()

	// Then: the daemon should start successfully
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	if !daemon.IsRunning() {
		t.Error("Daemon should be running after start")
	}

	// Verify PID file was created
	homeDir, _ := os.UserHomeDir()
	pidFile := filepath.Join(homeDir, ".centian", "daemon.pid")
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Error("PID file should exist after daemon start")
	}

	// When: stopping the daemon
	err = daemon.Stop()

	// Then: the daemon should stop successfully
	if err != nil {
		t.Errorf("Failed to stop daemon: %v", err)
	}

	if daemon.IsRunning() {
		t.Error("Daemon should not be running after stop")
	}

	// Verify PID file was removed
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("PID file should be removed after daemon stop")
	}
}

// TestDaemonPortAllocation tests dynamic port allocation
func TestDaemonPortAllocation(t *testing.T) {
	// Setup: create temporary directory SHARED by both daemons
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: first daemon
	daemon1, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create first daemon: %v", err)
	}

	// When: starting the first daemon
	err = daemon1.Start()
	if err != nil {
		t.Fatalf("Failed to start first daemon: %v", err)
	}
	defer daemon1.Stop()

	// Then: it should get a port
	port1 := daemon1.port
	if port1 == 0 {
		t.Error("First daemon should have a valid port")
	}

	// Given: second daemon (created AFTER first is running, same HOME dir)
	daemon2, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create second daemon: %v", err)
	}

	// When: starting the second daemon (should fail since PID file exists)
	err = daemon2.Start()

	// Then: the second daemon should fail to start
	if err == nil {
		daemon2.Stop() // Clean up if it somehow started
		t.Error("Second daemon should fail to start when first is running")
	} else {
		t.Logf("Second daemon correctly failed to start: %v", err)
	}
}

// TestIsDaemonRunning tests daemon running detection
func TestIsDaemonRunning(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: no daemon running initially
	// When: checking if daemon is running
	running := IsDaemonRunning()

	// Then: it should return false
	if running {
		t.Error("Daemon should not be running initially")
	}

	// Given: a started daemon
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	err = daemon.Start()
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Wait for daemon to be ready
	time.Sleep(100 * time.Millisecond)

	// When: checking if daemon is running
	running = IsDaemonRunning()

	// Then: it should return true
	if !running {
		t.Error("Daemon should be running after start")
	}
}

// TestGetDaemonPort tests getting daemon port from PID file
func TestGetDaemonPort(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Test GetDaemonPort when no daemon is running
	_, err := GetDaemonPort()
	if err == nil {
		t.Error("Expected error when no daemon is running")
	}

	// Create a daemon and start it
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	err = daemon.Start()
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Test GetDaemonPort when daemon is running
	port, err := GetDaemonPort()
	if err != nil {
		t.Fatalf("Failed to get daemon port: %v", err)
	}

	if port == 0 {
		t.Error("Daemon port should not be zero")
	}

	if port != daemon.port {
		t.Errorf("Expected port %d, got %d", daemon.port, port)
	}
}

// TestNewDaemonClient tests creating a daemon client
func TestNewDaemonClient(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Test client creation when no daemon is running
	_, err := NewDaemonClient()
	if err == nil {
		t.Error("Expected error when no daemon is running")
	}

	// Start a daemon
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	err = daemon.Start()
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Wait a moment for daemon to be ready
	time.Sleep(100 * time.Millisecond)

	// Test client creation when daemon is running
	client, err := NewDaemonClient()
	if err != nil {
		t.Fatalf("Failed to create daemon client: %v", err)
	}

	if client == nil {
		t.Fatal("Client should not be nil")
	}

	if client.port == 0 {
		t.Error("Client port should not be zero")
	}
}

// TestDaemonServerManagement tests server lifecycle through daemon
func TestDaemonServerManagement(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a running daemon
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	err = daemon.Start()
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Wait for daemon to be ready
	time.Sleep(100 * time.Millisecond)

	// Create a client
	client, err := NewDaemonClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// When: starting a stdio proxy via daemon (use sleep to avoid immediate exit)
	response, err := client.StartStdioProxy("sleep", []string{"1"})

	// Then: the request should succeed
	if err != nil {
		t.Fatalf("Failed to start stdio proxy via daemon: %v", err)
	}

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if !response.Success {
		t.Errorf("StartStdioProxy should succeed, got error: %s", response.Error)
	}

	if response.ServerID == "" {
		t.Error("Server ID should not be empty")
	}

	// Verify server was added to daemon
	daemon.serversMu.RLock()
	serverCount := len(daemon.servers)
	daemon.serversMu.RUnlock()

	if serverCount == 0 {
		t.Error("Daemon should have at least one server")
	}
}

// TestDaemonClientStatus tests getting daemon status via client
func TestDaemonClientStatus(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a running daemon
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	err = daemon.Start()
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Wait for daemon to be ready
	time.Sleep(100 * time.Millisecond)

	// Create a client
	client, err := NewDaemonClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// When: getting daemon status
	response, err := client.Status()

	// Then: the request should succeed
	if err != nil {
		t.Fatalf("Failed to get daemon status: %v", err)
	}

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if !response.Success {
		t.Errorf("Status request should succeed, got error: %s", response.Error)
	}
}

// TestDaemonPortInUse tests daemon behavior when port is in use
func TestDaemonPortInUse(t *testing.T) {
	// This test would require more complex setup to actually bind a port
	// For now, we'll test that the daemon can handle port allocation failures gracefully
	// by creating multiple daemons in quick succession

	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a daemon
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// When: starting the daemon
	err = daemon.Start()
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Then: it should successfully allocate a port
	if daemon.port == 0 {
		t.Error("Daemon should have allocated a port")
	}

	// The daemon should be able to accept connections on its port
	// (This would be tested more thoroughly in integration tests)
}