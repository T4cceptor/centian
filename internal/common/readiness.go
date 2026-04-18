package common

import (
	"errors"
	"net/http"
	"time"
)

// ErrReadinessTimeout reports that a readiness probe did not succeed before
// its deadline.
var ErrReadinessTimeout = errors.New("timed out waiting for readiness")

// WaitForReadiness polls ready until it succeeds, hook returns an error, or the
// timeout elapses.
func WaitForReadiness(
	client *http.Client,
	timeout time.Duration,
	interval time.Duration,
	hook func() error,
	ready func(*http.Client) bool,
) error {
	if client == nil {
		client = &http.Client{}
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if hook != nil {
			if err := hook(); err != nil {
				return err
			}
		}
		if ready != nil && ready(client) {
			return nil
		}
		time.Sleep(interval)
	}
	return ErrReadinessTimeout
}
