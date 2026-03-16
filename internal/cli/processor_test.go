package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	urfavecli "github.com/urfave/cli/v3"
)

func TestGenerateRandomSuffix(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"Given: length 4, When: generating suffix, Then: returns 4-char string", 4},
		{"Given: length 5, When: generating suffix, Then: returns 5-char string", 5},
		{"Given: length 1, When: generating suffix, Then: returns 1-char string", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: generating a random suffix.
			result, err := generateRandomSuffix(tt.length)

			// Then: no error and correct length.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.length {
				t.Errorf("expected length %d, got %d (%q)", tt.length, len(result), result)
			}

			// Then: all characters are from the allowed set.
			for _, c := range result {
				if !strings.ContainsRune(randomSuffixChars, c) {
					t.Errorf("character %q not in allowed set %q", c, randomSuffixChars)
				}
			}
		})
	}

	// Given: two generated suffixes, When: comparing, Then: they differ (probabilistically).
	t.Run("Given: two calls, When: generating suffixes, Then: results differ", func(t *testing.T) {
		a, _ := generateRandomSuffix(8)
		b, _ := generateRandomSuffix(8)
		if a == b {
			t.Errorf("two random suffixes should differ (both were %q)", a)
		}
	})
}

func TestPromptProcessorNameConflict(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedAction string
	}{
		{
			name:           "Given: user selects 1 (rename), When: prompting, Then: returns rename action",
			input:          "1\n",
			expectedAction: "rename",
		},
		{
			name:           "Given: user selects 2 (overwrite), When: prompting, Then: returns overwrite action",
			input:          "2\n",
			expectedAction: "overwrite",
		},
		{
			name:           "Given: user selects 3 (abort), When: prompting, Then: returns abort action",
			input:          "3\n",
			expectedAction: "abort",
		},
		{
			name:           "Given: user enters invalid input, When: prompting, Then: returns abort action",
			input:          "x\n",
			expectedAction: "abort",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: prompting with simulated input.
			reader := strings.NewReader(tt.input)
			action, resolvedName, err := promptProcessorNameConflict("test-proc", reader)

			// Then: no error.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Then: action matches expectation.
			if action != tt.expectedAction {
				t.Errorf("expected action %q, got %q", tt.expectedAction, action)
			}

			// Then: rename action returns a suffixed name.
			if tt.expectedAction == "rename" {
				if !strings.HasPrefix(resolvedName, "test-proc_") {
					t.Errorf("expected renamed to start with 'test-proc_', got %q", resolvedName)
				}
				// Suffix should be underscore + 4 chars = 5 chars after the original name.
				suffix := strings.TrimPrefix(resolvedName, "test-proc_")
				if len(suffix) != 4 {
					t.Errorf("expected 4-char suffix, got %d chars (%q)", len(suffix), suffix)
				}
			}

			// Then: overwrite returns the original name.
			if tt.expectedAction == "overwrite" && resolvedName != "test-proc" {
				t.Errorf("expected overwrite to return original name 'test-proc', got %q", resolvedName)
			}
		})
	}
}

// setupTestHome creates an isolated HOME with an initialized centian config.
// Returns a cleanup function that restores the original HOME.
func setupTestHome(t *testing.T) func() {
	t.Helper()
	tempDir := t.TempDir()
	testHome := filepath.Join(tempDir, "processor_cmd_test")
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", testHome)

	// Create default config.
	cfg := config.DefaultConfig()
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	return func() {
		os.Setenv("HOME", originalHome)
	}
}

func TestHandleProcessorAdd(t *testing.T) {
	t.Run("Given: a valid python path, When: adding processor, Then: processor is saved to config", func(t *testing.T) {
		// Given: isolated config environment.
		cleanup := setupTestHome(t)
		defer cleanup()

		// When: running 'centian processor add --path ./audit.py'.
		app := &urfavecli.Command{
			Name: "centian",
			Commands: []*urfavecli.Command{
				ProcessorCommand,
			},
		}
		err := app.Run(context.Background(), []string{"centian", "processor", "add", "--path", "./audit.py"})

		// Then: no error.
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then: config contains the processor with inferred name and command.
		cfg, err := config.LoadConfig()
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if !cfg.HasProcessor("audit") {
			t.Fatal("expected processor 'audit' to exist in config")
		}
		proc := cfg.Processors[0]
		if proc.Type != "cli" {
			t.Errorf("expected type 'cli', got %q", proc.Type)
		}
		if proc.Config["command"] != "python3" {
			t.Errorf("expected command 'python3', got %v", proc.Config["command"])
		}
		if proc.Timeout != 15 {
			t.Errorf("expected timeout 15, got %d", proc.Timeout)
		}
		if !proc.Enabled {
			t.Error("expected processor to be enabled")
		}
	})

	t.Run("Given: --name override, When: adding processor, Then: uses provided name", func(t *testing.T) {
		// Given: isolated config environment.
		cleanup := setupTestHome(t)
		defer cleanup()

		// When: running with explicit --name.
		app := &urfavecli.Command{
			Name: "centian",
			Commands: []*urfavecli.Command{
				ProcessorCommand,
			},
		}
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add",
			"--path", "./scripts/my_logger.js",
			"--name", "custom-logger",
		})

		// Then: no error.
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then: processor uses the provided name, not inferred.
		cfg, _ := config.LoadConfig()
		if !cfg.HasProcessor("custom-logger") {
			t.Fatal("expected processor 'custom-logger' to exist")
		}
		if cfg.HasProcessor("my_logger") {
			t.Error("inferred name 'my_logger' should not exist when --name was provided")
		}
	})

	t.Run("Given: a .sh file, When: adding processor, Then: command is bash", func(t *testing.T) {
		cleanup := setupTestHome(t)
		defer cleanup()

		app := &urfavecli.Command{
			Name:     "centian",
			Commands: []*urfavecli.Command{ProcessorCommand},
		}
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add", "--path", "./filter.sh",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cfg, _ := config.LoadConfig()
		if !cfg.HasProcessor("filter") {
			t.Fatal("expected processor 'filter' to exist")
		}
		if cfg.Processors[0].Config["command"] != "bash" {
			t.Errorf("expected command 'bash', got %v", cfg.Processors[0].Config["command"])
		}
	})

	t.Run("Given: a .ts file, When: adding processor, Then: command is npx with ts-node", func(t *testing.T) {
		cleanup := setupTestHome(t)
		defer cleanup()

		app := &urfavecli.Command{
			Name:     "centian",
			Commands: []*urfavecli.Command{ProcessorCommand},
		}
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add", "--path", "./validator.ts",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cfg, _ := config.LoadConfig()
		if !cfg.HasProcessor("validator") {
			t.Fatal("expected processor 'validator' to exist")
		}
		if cfg.Processors[0].Config["command"] != "npx" {
			t.Errorf("expected command 'npx', got %v", cfg.Processors[0].Config["command"])
		}
	})

	t.Run("Given: unsupported extension, When: adding processor, Then: returns error", func(t *testing.T) {
		cleanup := setupTestHome(t)
		defer cleanup()

		app := &urfavecli.Command{
			Name:     "centian",
			Commands: []*urfavecli.Command{ProcessorCommand},
		}
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add", "--path", "./script.rb",
		})

		// Then: error about unsupported extension.
		if err == nil {
			t.Fatal("expected error for unsupported extension")
		}
		if !strings.Contains(err.Error(), "unsupported file extension") {
			t.Errorf("expected 'unsupported file extension' error, got: %v", err)
		}
	})

	t.Run("Given: multiple processors added, When: loading config, Then: all persist", func(t *testing.T) {
		cleanup := setupTestHome(t)
		defer cleanup()

		app := &urfavecli.Command{
			Name:     "centian",
			Commands: []*urfavecli.Command{ProcessorCommand},
		}

		// When: adding two processors sequentially.
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add", "--path", "./first.py",
		})
		if err != nil {
			t.Fatalf("first add failed: %v", err)
		}

		err = app.Run(context.Background(), []string{
			"centian", "processor", "add", "--path", "./second.js",
		})
		if err != nil {
			t.Fatalf("second add failed: %v", err)
		}

		// Then: both processors exist in config.
		cfg, _ := config.LoadConfig()
		if len(cfg.Processors) != 2 {
			t.Fatalf("expected 2 processors, got %d", len(cfg.Processors))
		}
		if !cfg.HasProcessor("first") {
			t.Error("expected processor 'first' to exist")
		}
		if !cfg.HasProcessor("second") {
			t.Error("expected processor 'second' to exist")
		}
	})

	t.Run("Given: webhook processor flags, When: adding webhook processor, Then: webhook config is saved", func(t *testing.T) {
		cleanup := setupTestHome(t)
		defer cleanup()

		app := &urfavecli.Command{
			Name:     "centian",
			Commands: []*urfavecli.Command{ProcessorCommand},
		}
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add",
			"--type", "webhook",
			"--url", "https://example.com/processors/audit",
			"--header", "Authorization=Bearer ${TOKEN}",
			"--header", "X-Trace=trace-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if !cfg.HasProcessor("audit") {
			t.Fatal("expected processor 'audit' to exist in config")
		}
		proc := cfg.Processors[0]
		if proc.Type != "webhook" {
			t.Errorf("expected type 'webhook', got %q", proc.Type)
		}
		if proc.Config["url"] != "https://example.com/processors/audit" {
			t.Errorf("expected webhook url to be saved, got %v", proc.Config["url"])
		}
		headers, ok := proc.Config["headers"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected headers object, got %T", proc.Config["headers"])
		}
		if headers["Authorization"] != "Bearer ${TOKEN}" {
			t.Errorf("expected Authorization header to persist, got %v", headers["Authorization"])
		}
		if headers["X-Trace"] != "trace-1" {
			t.Errorf("expected X-Trace header to persist, got %v", headers["X-Trace"])
		}
	})

	t.Run("Given: webhook processor without url, When: adding, Then: returns error", func(t *testing.T) {
		cleanup := setupTestHome(t)
		defer cleanup()

		app := &urfavecli.Command{
			Name:     "centian",
			Commands: []*urfavecli.Command{ProcessorCommand},
		}
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add",
			"--type", "webhook",
		})

		if err == nil {
			t.Fatal("expected error for missing webhook url")
		}
		if !strings.Contains(err.Error(), "--url is required") {
			t.Errorf("expected missing url error, got: %v", err)
		}
	})

	t.Run("Given: webhook processor with path, When: adding, Then: returns error", func(t *testing.T) {
		cleanup := setupTestHome(t)
		defer cleanup()

		app := &urfavecli.Command{
			Name:     "centian",
			Commands: []*urfavecli.Command{ProcessorCommand},
		}
		err := app.Run(context.Background(), []string{
			"centian", "processor", "add",
			"--type", "webhook",
			"--path", "./script.py",
			"--url", "https://example.com/processors/audit",
		})

		if err == nil {
			t.Fatal("expected error for webhook path")
		}
		if !strings.Contains(err.Error(), "--path is not supported") {
			t.Errorf("expected webhook path error, got: %v", err)
		}
	})
}
