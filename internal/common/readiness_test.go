package common

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestWaitForReadinessReturnsNilWhenReady(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	attempts := 0

	err := WaitForReadiness(client, 100*time.Millisecond, time.Millisecond, nil, func(*http.Client) bool {
		attempts++
		return attempts == 2
	})
	if err != nil {
		t.Fatalf("WaitForReadiness: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestWaitForReadinessReturnsHookError(t *testing.T) {
	want := errors.New("boom")

	err := WaitForReadiness(nil, 100*time.Millisecond, time.Millisecond, func() error {
		return want
	}, func(*http.Client) bool {
		return false
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected hook error, got %v", err)
	}
}

func TestWaitForReadinessTimesOut(t *testing.T) {
	err := WaitForReadiness(nil, 5*time.Millisecond, time.Millisecond, nil, func(*http.Client) bool {
		return false
	})
	if !errors.Is(err, ErrReadinessTimeout) {
		t.Fatalf("expected readiness timeout, got %v", err)
	}
}
