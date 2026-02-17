package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferProcessorNameFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Given: a python script path, When: inferring name, Then: returns filename without extension",
			path:     "./processors/security_validator.py",
			expected: "security_validator",
		},
		{
			name:     "Given: a JS script with hyphens, When: inferring name, Then: preserves hyphens",
			path:     "/home/user/my-logger.js",
			expected: "my-logger",
		},
		{
			name:     "Given: a bash script, When: inferring name, Then: returns lowercase name",
			path:     "./Audit.sh",
			expected: "audit",
		},
		{
			name:     "Given: a typescript script, When: inferring name, Then: strips .ts extension",
			path:     "processors/request_filter.ts",
			expected: "request_filter",
		},
		{
			name:     "Given: a path with multiple dots, When: inferring name, Then: only strips last extension",
			path:     "./my.processor.py",
			expected: "my.processor",
		},
		{
			name:     "Given: just a filename, When: inferring name, Then: works without directory",
			path:     "script.py",
			expected: "script",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: inferring the processor name from the path.
			result := InferProcessorNameFromPath(tt.path)

			// Then: the result matches the expected name.
			if result != tt.expected {
				t.Errorf("InferProcessorNameFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestInferCommandFromPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantCmd     string
		wantArgs    []string
		wantErr     bool
		errContains string
	}{
		{
			name:     "Given: a .py file, When: inferring command, Then: returns python3",
			path:     "./processors/script.py",
			wantCmd:  "python3",
			wantArgs: []string{"./processors/script.py"},
		},
		{
			name:     "Given: a .js file, When: inferring command, Then: returns node",
			path:     "./processors/script.js",
			wantCmd:  "node",
			wantArgs: []string{"./processors/script.js"},
		},
		{
			name:     "Given: a .ts file, When: inferring command, Then: returns npx with ts-node",
			path:     "./processors/script.ts",
			wantCmd:  "npx",
			wantArgs: []string{"ts-node", "./processors/script.ts"},
		},
		{
			name:     "Given: a .sh file, When: inferring command, Then: returns bash",
			path:     "./processors/script.sh",
			wantCmd:  "bash",
			wantArgs: []string{"./processors/script.sh"},
		},
		{
			name:        "Given: an unsupported extension, When: inferring command, Then: returns error",
			path:        "./processors/script.rb",
			wantErr:     true,
			errContains: "unsupported file extension '.rb'",
		},
		{
			name:        "Given: no extension, When: inferring command, Then: returns error",
			path:        "./processors/script",
			wantErr:     true,
			errContains: "unsupported file extension",
		},
		{
			name:     "Given: uppercase extension, When: inferring command, Then: handles case-insensitively",
			path:     "./processors/script.PY",
			wantCmd:  "python3",
			wantArgs: []string{"./processors/script.PY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: inferring the command from the path.
			cmd, args, err := InferCommandFromPath(tt.path)

			// Then: error expectation matches.
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Then: command and args match expectations.
			if cmd != tt.wantCmd {
				t.Errorf("command = %q, want %q", cmd, tt.wantCmd)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args length = %d, want %d", len(args), len(tt.wantArgs))
			}
			for i, arg := range args {
				if arg != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestHasProcessor(t *testing.T) {
	// Given: a config with one processor.
	cfg := &GlobalConfig{
		Processors: []*ProcessorConfig{
			{Name: "existing-processor", Type: "cli", Enabled: true},
		},
	}

	// When/Then: checking for existing processor returns true.
	if !cfg.HasProcessor("existing-processor") {
		t.Error("expected HasProcessor to return true for existing processor")
	}

	// When/Then: checking for non-existing processor returns false.
	if cfg.HasProcessor("nonexistent") {
		t.Error("expected HasProcessor to return false for nonexistent processor")
	}

	// When/Then: checking on nil Processors slice returns false.
	emptyCfg := &GlobalConfig{}
	if emptyCfg.HasProcessor("anything") {
		t.Error("expected HasProcessor to return false on empty config")
	}
}

func TestAddProcessor(t *testing.T) {
	// Given: a config with no processors.
	cfg := &GlobalConfig{
		Processors: []*ProcessorConfig{},
	}

	// When: adding a processor.
	proc := &ProcessorConfig{Name: "new-proc", Type: "cli", Enabled: true}
	cfg.AddProcessor(proc)

	// Then: config has one processor.
	if len(cfg.Processors) != 1 {
		t.Fatalf("expected 1 processor, got %d", len(cfg.Processors))
	}
	if cfg.Processors[0].Name != "new-proc" {
		t.Errorf("expected processor name 'new-proc', got %q", cfg.Processors[0].Name)
	}
}

func TestReplaceProcessor(t *testing.T) {
	// Given: a config with two processors.
	cfg := &GlobalConfig{
		Processors: []*ProcessorConfig{
			{Name: "first", Type: "cli", Enabled: true, Timeout: 10},
			{Name: "second", Type: "cli", Enabled: true, Timeout: 20},
		},
	}

	// When: replacing the first processor.
	replacement := &ProcessorConfig{Name: "first", Type: "cli", Enabled: false, Timeout: 30}
	replaced := cfg.ReplaceProcessor("first", replacement)

	// Then: replacement was successful and position preserved.
	if !replaced {
		t.Fatal("expected ReplaceProcessor to return true")
	}
	if len(cfg.Processors) != 2 {
		t.Fatalf("expected 2 processors, got %d", len(cfg.Processors))
	}
	if cfg.Processors[0].Timeout != 30 {
		t.Errorf("expected replaced processor timeout 30, got %d", cfg.Processors[0].Timeout)
	}
	if cfg.Processors[0].Enabled {
		t.Error("expected replaced processor to be disabled")
	}

	// When: replacing a nonexistent processor.
	notFound := cfg.ReplaceProcessor("nonexistent", replacement)

	// Then: returns false.
	if notFound {
		t.Error("expected ReplaceProcessor to return false for nonexistent processor")
	}
}

func TestProcessorAddPersistence(t *testing.T) {
	// Given: a temp HOME directory for isolated config.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "processor_add_test")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Given: a default config saved to disk.
	cfg := DefaultConfig()
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	// When: adding a processor and saving.
	cmd, args, err := InferCommandFromPath("./processors/audit.py")
	if err != nil {
		t.Fatalf("unexpected error from InferCommandFromPath: %v", err)
	}
	configArgs := make([]interface{}, len(args))
	for i, a := range args {
		configArgs[i] = a
	}
	proc := &ProcessorConfig{
		Name:    InferProcessorNameFromPath("./processors/audit.py"),
		Type:    "cli",
		Enabled: true,
		Timeout: 15,
		Config: map[string]interface{}{
			"command": cmd,
			"args":    configArgs,
		},
	}
	cfg.AddProcessor(proc)
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save config with processor: %v", err)
	}

	// Then: loading the config preserves the processor.
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if len(loaded.Processors) != 1 {
		t.Fatalf("expected 1 processor, got %d", len(loaded.Processors))
	}
	p := loaded.Processors[0]
	if p.Name != "audit" {
		t.Errorf("expected processor name 'audit', got %q", p.Name)
	}
	if p.Type != "cli" {
		t.Errorf("expected processor type 'cli', got %q", p.Type)
	}
	command, ok := p.Config["command"].(string)
	if !ok || command != "python3" {
		t.Errorf("expected command 'python3', got %v", p.Config["command"])
	}
}
