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

func TestInferProcessorNameFromWebhookURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected string
	}{
		{
			name:     "Given: URL with path, When: inferring webhook name, Then: use last path segment",
			rawURL:   "https://example.com/processors/audit",
			expected: "audit",
		},
		{
			name:     "Given: URL without path, When: inferring webhook name, Then: use hostname",
			rawURL:   "https://hooks.example.com",
			expected: "hooks.example.com",
		},
		{
			name:     "Given: invalid URL, When: inferring webhook name, Then: use fallback",
			rawURL:   "::bad-url::",
			expected: "webhook-processor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferProcessorNameFromWebhookURL(tt.rawURL)
			if result != tt.expected {
				t.Errorf("InferProcessorNameFromWebhookURL(%q) = %q, want %q", tt.rawURL, result, tt.expected)
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
	// Given: a gateway file with one processor.
	gf := &GatewayFile{
		GlobalProcessors: []*ProcessorConfig{
			{Name: "existing-processor", Type: "cli", Enabled: true},
		},
	}

	// When/Then: checking for existing processor returns true.
	if !gf.HasProcessor("existing-processor") {
		t.Error("expected HasProcessor to return true for existing processor")
	}

	// When/Then: checking for non-existing processor returns false.
	if gf.HasProcessor("nonexistent") {
		t.Error("expected HasProcessor to return false for nonexistent processor")
	}

	// When/Then: checking on nil GlobalProcessors slice returns false.
	emptyGF := &GatewayFile{}
	if emptyGF.HasProcessor("anything") {
		t.Error("expected HasProcessor to return false on empty gateway file")
	}
}

func TestAddProcessor(t *testing.T) {
	// Given: a gateway file with no processors.
	gf := &GatewayFile{
		GlobalProcessors: []*ProcessorConfig{},
	}

	// When: adding a processor.
	proc := &ProcessorConfig{Name: "new-proc", Type: "cli", Enabled: true}
	gf.AddProcessor(proc)

	// Then: gateway file has one processor.
	if len(gf.GlobalProcessors) != 1 {
		t.Fatalf("expected 1 processor, got %d", len(gf.GlobalProcessors))
	}
	if gf.GlobalProcessors[0].Name != "new-proc" {
		t.Errorf("expected processor name 'new-proc', got %q", gf.GlobalProcessors[0].Name)
	}
}

func TestReplaceProcessor(t *testing.T) {
	// Given: a gateway file with two processors.
	gf := &GatewayFile{
		GlobalProcessors: []*ProcessorConfig{
			{Name: "first", Type: "cli", Enabled: true, Timeout: 10},
			{Name: "second", Type: "cli", Enabled: true, Timeout: 20},
		},
	}

	// When: replacing the first processor.
	replacement := &ProcessorConfig{Name: "first", Type: "cli", Enabled: false, Timeout: 30}
	replaced := gf.ReplaceProcessor("first", replacement)

	// Then: replacement was successful and position preserved.
	if !replaced {
		t.Fatal("expected ReplaceProcessor to return true")
	}
	if len(gf.GlobalProcessors) != 2 {
		t.Fatalf("expected 2 processors, got %d", len(gf.GlobalProcessors))
	}
	if gf.GlobalProcessors[0].Timeout != 30 {
		t.Errorf("expected replaced processor timeout 30, got %d", gf.GlobalProcessors[0].Timeout)
	}
	if gf.GlobalProcessors[0].Enabled {
		t.Error("expected replaced processor to be disabled")
	}

	// When: replacing a nonexistent processor.
	notFound := gf.ReplaceProcessor("nonexistent", replacement)

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

	// Given: a default config and gateway file saved to disk.
	cfg := DefaultConfig()
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}
	gf := DefaultGatewayFile()
	if err := SaveGatewayFile(cfg, gf); err != nil {
		t.Fatalf("failed to save initial gateway file: %v", err)
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
	gf.AddProcessor(proc)
	if err := SaveGatewayFile(cfg, gf); err != nil {
		t.Fatalf("failed to save gateway file with processor: %v", err)
	}

	// Then: loading the gateway file preserves the processor.
	loadedCfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	loadedGF, err := LoadGatewayFile(loadedCfg)
	if err != nil {
		t.Fatalf("failed to load gateway file: %v", err)
	}
	if len(loadedGF.GlobalProcessors) != 1 {
		t.Fatalf("expected 1 processor, got %d", len(loadedGF.GlobalProcessors))
	}
	p := loadedGF.GlobalProcessors[0]
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
