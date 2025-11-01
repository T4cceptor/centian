package daemon

import (
	"os"
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

	// When: stopping the daemon
	err = daemon.Stop()

	// Then: the daemon should stop successfully
	if err != nil {
		t.Errorf("Failed to stop daemon: %v", err)
	}

	if daemon.IsRunning() {
		t.Error("Daemon should not be running after stop")
	}

}

// TestDaemonPortAllocation tests that only one daemon can bind to the fixed port
func TestDaemonPortAllocation(t *testing.T) {
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

	// Then: it should bind to the default port
	port1 := daemon1.port
	if port1 != DefaultDaemonPort {
		t.Errorf("First daemon should use default port %d, got %d", DefaultDaemonPort, port1)
	}

	// Given: second daemon attempting to use the same port
	daemon2, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create second daemon: %v", err)
	}

	// When: starting the second daemon (should fail since port is in use)
	err = daemon2.Start()

	// Then: the second daemon should fail to start
	if err == nil {
		daemon2.Stop() // Clean up if it somehow started
		t.Error("Second daemon should fail to start when first is running on same port")
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

// TestGetDaemonPort tests that the daemon uses the default port
func TestGetDaemonPort(t *testing.T) {
	// Create a daemon
	daemon, err := NewDaemon()
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// Test that it uses the default port
	if daemon.port != DefaultDaemonPort {
		t.Errorf("Expected daemon to use default port %d, got %d", DefaultDaemonPort, daemon.port)
	}

	err = daemon.Start()
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Verify the daemon is listening on the expected port
	if daemon.GetPort() != DefaultDaemonPort {
		t.Errorf("Expected daemon port %d, got %d", DefaultDaemonPort, daemon.GetPort())
	}
}

// TestNewDaemonClient tests creating a daemon client
func TestNewDaemonClient(t *testing.T) {
	// Test client creation (no daemon needs to be running for creation)
	client, err := NewDaemonClient()
	if err != nil {
		t.Fatalf("Failed to create daemon client: %v", err)
	}

	if client == nil {
		t.Fatal("Client should not be nil")
	}

	if client.port != DefaultDaemonPort {
		t.Errorf("Client should use default port %d, got %d", DefaultDaemonPort, client.port)
	}
}

// TestDaemonServerManagement tests server lifecycle through daemon
func TestDaemonServerManagement(t *testing.T) {

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

// TestConcurrentDaemonStart tests the fix for TOCTOU race condition (Issue #3)
// Verifies that only one daemon can bind to the TCP port when multiple goroutines attempt simultaneously
func TestConcurrentDaemonStart(t *testing.T) {
	// Given: 10 goroutines attempting to start daemons concurrently
	const goroutines = 10
	results := make(chan error, goroutines)
	daemons := make([]*Daemon, goroutines)
	startSignal := make(chan struct{})

	// Pre-create all daemons (they all target the same port: DefaultDaemonPort)
	for i := 0; i < goroutines; i++ {
		daemon, err := NewDaemon()
		if err != nil {
			t.Fatalf("Failed to create daemon %d: %v", i, err)
		}
		daemons[i] = daemon
		t.Logf("Daemon %d: port=%d", i, daemon.port)
	}

	// When: all goroutines attempt to bind to the same TCP port simultaneously
	for i := 0; i < goroutines; i++ {
		go func(index int) {
			<-startSignal // Wait for signal to start
			results <- daemons[index].Start()
		}(i)
	}

	// Signal all goroutines to start at once
	close(startSignal)

	// Collect all results
	var successCount, failCount int
	var errors []string

	for i := 0; i < goroutines; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else {
			failCount++
			errors = append(errors, err.Error())
		}
	}

	// Log results for debugging
	t.Logf("Success: %d, Failures: %d", successCount, failCount)
	if len(errors) > 0 {
		t.Logf("Sample errors: %v", errors[:minInt(3, len(errors))])
	}

	// Then: exactly one daemon should succeed (atomic TCP bind)
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful start, got %d", successCount)
	}

	if failCount != goroutines-1 {
		t.Errorf("Expected %d failures, got %d", goroutines-1, failCount)
	}

	// Cleanup: stop all daemons (only successful one is actually running)
	for _, daemon := range daemons {
		if daemon != nil && daemon.IsRunning() {
			daemon.Stop()
		}
	}

	t.Logf("✓ Race condition test passed: TCP bind is atomic")
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
