package config

import (
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

func TestResolveEventStorePathUsesConfiguredPath(t *testing.T) {
	path, err := ResolveEventStorePath(&EventStorageCapabilitySettings{
		Driver: DefaultEventStorageDriver,
		Path:   "/tmp/custom-events.sqlite",
	})
	assert.NilError(t, err)
	assert.Equal(t, path, "/tmp/custom-events.sqlite")
}

func TestResolveEventStorePathUsesDefaultLogsPath(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("CENTIAN_LOG_DIR", logDir)

	path, err := ResolveEventStorePath(nil)
	assert.NilError(t, err)
	assert.Equal(t, path, filepath.Join(logDir, "events.sqlite"))
}

func TestResolveEventStorePathRejectsUnsupportedDriver(t *testing.T) {
	_, err := ResolveEventStorePath(&EventStorageCapabilitySettings{Driver: "postgres"})
	assert.ErrorContains(t, err, `unsupported event storage driver "postgres"`)
}
