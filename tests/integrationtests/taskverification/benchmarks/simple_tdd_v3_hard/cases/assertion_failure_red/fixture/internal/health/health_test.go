package health

import "testing"

func TestHealthStatus(t *testing.T) {
	if status := HealthStatus(); status != "ready" {
		t.Fatalf("expected ready, got %s", status)
	}
}
