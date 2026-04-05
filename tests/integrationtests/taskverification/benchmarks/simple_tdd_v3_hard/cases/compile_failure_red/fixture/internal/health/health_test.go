package health

import "testing"

func TestHealthStatus(t *testing.T) {
	response := StatusResponse{
		Message: HealthStatus(),
	}
	if response.Message != "ok" {
		t.Fatalf("expected ok, got %q", response.Message)
	}
}
