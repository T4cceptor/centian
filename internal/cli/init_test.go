package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func newInitUITestInput(input string) *InitUI {
	return &InitUI{reader: bufio.NewReader(strings.NewReader(input))}
}

func setTempStdin(t *testing.T, input string) {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "stdin-*")
	assert.NilError(t, err)

	_, err = tmpFile.WriteString(input)
	assert.NilError(t, err)
	_, err = tmpFile.Seek(0, 0)
	assert.NilError(t, err)

	originalStdin := os.Stdin
	os.Stdin = tmpFile
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = tmpFile.Close()
	})
}

func TestPromptInitOption(t *testing.T) {
	t.Run("quickstart", func(t *testing.T) {
		ui := newInitUITestInput("1\n")
		option, err := ui.promptInitOption(false)
		assert.NilError(t, err)
		assert.Equal(t, option, InitOptionQuickstart)
	})

	t.Run("empty", func(t *testing.T) {
		ui := newInitUITestInput("2\n")
		option, err := ui.promptInitOption(false)
		assert.NilError(t, err)
		assert.Equal(t, option, InitOptionEmpty)
	})

	t.Run("from path", func(t *testing.T) {
		ui := newInitUITestInput("3\n")
		option, err := ui.promptInitOption(false)
		assert.NilError(t, err)
		assert.Equal(t, option, InitOptionFromPath)
	})

	t.Run("invalid choice reprompts", func(t *testing.T) {
		ui := newInitUITestInput("9\n2\n")
		option, err := ui.promptInitOption(false)
		assert.NilError(t, err)
		assert.Equal(t, option, InitOptionEmpty)
	})
}

func TestPromptConfigPath(t *testing.T) {
	t.Run("valid path", func(t *testing.T) {
		testPath := filepath.Join(t.TempDir(), "config.json")
		assert.NilError(t, os.WriteFile(testPath, []byte(`{}`), 0o644))

		ui := newInitUITestInput(testPath + "\n")
		path, err := ui.promptConfigPath()
		assert.NilError(t, err)
		assert.Equal(t, path, testPath)
	})

	t.Run("empty input then valid path", func(t *testing.T) {
		testPath := filepath.Join(t.TempDir(), "config.json")
		assert.NilError(t, os.WriteFile(testPath, []byte(`{}`), 0o644))

		ui := newInitUITestInput("\n" + testPath + "\n")
		path, err := ui.promptConfigPath()
		assert.NilError(t, err)
		assert.Equal(t, path, testPath)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		ui := newInitUITestInput("/does/not/exist.json\n")
		_, err := ui.promptConfigPath()
		assert.Assert(t, err != nil)
	})
}

func TestImportFromPath(t *testing.T) {
	cfg := config.DefaultConfig()

	t.Run("missing file", func(t *testing.T) {
		_, err := importFromPath(cfg, "/does/not/exist.json")
		assert.Assert(t, err != nil)
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.json")
		assert.NilError(t, os.WriteFile(path, []byte(`{invalid}`), 0o644))

		_, err := importFromPath(cfg, path)
		assert.Assert(t, err != nil)
	})

	t.Run("valid config with no servers", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.json")
		assert.NilError(t, os.WriteFile(path, []byte(`{"mcpServers":{}}`), 0o644))

		imported, err := importFromPath(cfg, path)
		assert.NilError(t, err)
		assert.Equal(t, imported, 0)
	})
}

func TestHandleInteractiveInit(t *testing.T) {
	t.Run("empty option", func(t *testing.T) {
		cfg := config.DefaultConfig()
		ui := newInitUITestInput("2\n")

		imported, quickstart, err := handleInteractiveInit(cfg, ui)
		assert.NilError(t, err)
		assert.Equal(t, imported, 0)
		assert.Assert(t, !quickstart)
	})

	t.Run("quickstart without npx returns error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		ui := newInitUITestInput("1\n")
		t.Setenv("PATH", "")

		_, _, err := handleInteractiveInit(cfg, ui)
		assert.Assert(t, err != nil)
	})

	t.Run("from path with missing file falls back to empty", func(t *testing.T) {
		cfg := config.DefaultConfig()
		ui := newInitUITestInput("3\n/does/not/exist.json\n")

		imported, quickstart, err := handleInteractiveInit(cfg, ui)
		assert.NilError(t, err)
		assert.Equal(t, imported, 0)
		assert.Assert(t, !quickstart)
	})

	t.Run("from path with valid empty config", func(t *testing.T) {
		cfg := config.DefaultConfig()
		path := filepath.Join(t.TempDir(), "empty.json")
		assert.NilError(t, os.WriteFile(path, []byte(`{"mcpServers":{}}`), 0o644))
		ui := newInitUITestInput(fmt.Sprintf("3\n%s\n", path))

		imported, quickstart, err := handleInteractiveInit(cfg, ui)
		assert.NilError(t, err)
		assert.Equal(t, imported, 0)
		assert.Assert(t, !quickstart)
	})
}

func TestApplyQuickstartConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	applyQuickstartConfig(cfg)

	defaultGateway, ok := cfg.Gateways["default"]
	assert.Assert(t, ok)
	assert.Assert(t, defaultGateway != nil)

	server, ok := defaultGateway.MCPServers["sequential-thinking"]
	assert.Assert(t, ok)
	assert.Assert(t, server != nil)
	assert.Equal(t, server.Command, "npx")
	assert.DeepEqual(t, server.Args, []string{"-y", "@modelcontextprotocol/server-sequential-thinking"})
}

func TestCreateDefaultAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	key, err := createDefaultAPIKey()
	assert.NilError(t, err)
	assert.Assert(t, key != "")

	keysPath, err := auth.DefaultAPIKeysPath()
	assert.NilError(t, err)
	_, statErr := os.Stat(keysPath)
	assert.NilError(t, statErr)
}

func TestHandleQuickstart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setTempStdin(t, "\n")

	cfg := config.DefaultConfig()
	err := handleQuickstart("/tmp/config.json", cfg)
	assert.NilError(t, err)
}

func TestPrintInitSuccessUsesDefaultProxyPortInSnippet(t *testing.T) {
	setTempStdin(t, "\n")

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	assert.NilError(t, err)
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()

	printInitSuccess("/tmp/config.json", 0)

	_ = writer.Close()
	output := <-done
	_ = reader.Close()

	assert.Assert(t, strings.Contains(output, "http://localhost:9666/mcp/default"))
}
