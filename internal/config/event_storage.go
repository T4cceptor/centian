package config

import (
	"fmt"

	"github.com/T4cceptor/centian/internal/logging"
)

// ResolveEventStorePath resolves the effective SQLite event-store path using
// the same semantics Centian startup applies at runtime.
func ResolveEventStorePath(settings *EventStorageCapabilitySettings) (string, error) {
	driver := DefaultEventStorageDriver
	if settings != nil {
		driver = settings.GetDriver()
	}
	if driver != DefaultEventStorageDriver {
		return "", fmt.Errorf("unsupported event storage driver %q", driver)
	}
	if settings != nil && settings.Path != "" {
		return settings.Path, nil
	}
	storePath, err := logging.GetDefaultEventStorePath()
	if err != nil {
		return "", fmt.Errorf("failed to determine default event storage path: %w", err)
	}
	return storePath, nil
}
