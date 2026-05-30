package cli

import (
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	urfavecli "github.com/urfave/cli/v3"
)

func TestDemoCommandStructure(t *testing.T) {
	if DemoCommand == nil {
		t.Fatal("DemoCommand is nil")
	}
	if DemoCommand.Name != "demo" {
		t.Fatalf("expected demo command name, got %q", DemoCommand.Name)
	}
	if DemoCommand.Action == nil {
		t.Fatal("DemoCommand.Action is nil")
	}
	flagNames := map[string]bool{}
	for _, flag := range DemoCommand.Flags {
		if typed, ok := flag.(*urfavecli.StringFlag); ok {
			flagNames[typed.Name] = true
		}
	}
	for _, expected := range []string{"agent", "file", "model", "profile", "path", "codex-config"} {
		if !flagNames[expected] {
			t.Fatalf("expected %q flag on DemoCommand", expected)
		}
	}
}

func TestDemoCommandDocumentsStaticDefaultAndDeprecatedLegacyFlags(t *testing.T) {
	if !strings.Contains(DemoCommand.Description, "seed the bundled IT Ops") {
		t.Fatalf("expected static IT Ops demo description, got %q", DemoCommand.Description)
	}
	if !strings.Contains(strings.ToLower(DemoCommand.Description), "deprecated") {
		t.Fatalf("expected deprecated legacy flow note, got %q", DemoCommand.Description)
	}
	legacyFlags := map[string]bool{"agent": false, "file": false}
	for _, flag := range DemoCommand.Flags {
		typed, ok := flag.(*urfavecli.StringFlag)
		if !ok {
			continue
		}
		if _, tracked := legacyFlags[typed.Name]; !tracked {
			continue
		}
		if !strings.Contains(strings.ToLower(typed.Usage), "deprecated") {
			t.Fatalf("expected deprecated %s flag usage, got %q", typed.Name, typed.Usage)
		}
		legacyFlags[typed.Name] = true
	}
	for flagName, found := range legacyFlags {
		if !found {
			t.Fatalf("expected %s flag", flagName)
		}
	}
}

func TestRejectDeprecatedDemoFlags(t *testing.T) {
	for _, flagName := range []string{"agent", "file", "path", "model", "profile", "codex-config"} {
		t.Run(flagName, func(t *testing.T) {
			cmd := newTestCLICommand(DemoCommand.Flags)
			cmd.Set(flagName, "value")

			err := rejectDeprecatedDemoFlags(cmd)
			if err == nil || !strings.Contains(err.Error(), "deprecated") || !strings.Contains(err.Error(), "--"+flagName) {
				t.Fatalf("expected deprecated %s error, got %v", flagName, err)
			}
		})
	}
}

func TestNewDemoConfigUsesDisklessDemoProject(t *testing.T) {
	cfg := newDemoConfig()
	if cfg.Proxy.Host != config.DefaultProxyHost {
		t.Fatalf("expected loopback host, got %q", cfg.Proxy.Host)
	}
	if cfg.Proxy.Port != "0" {
		t.Fatalf("expected ephemeral port, got %q", cfg.Proxy.Port)
	}
	if len(cfg.Projects) != 1 {
		t.Fatalf("expected only demo project, got %d", len(cfg.Projects))
	}
	project := cfg.Projects[demoProjectSlug]
	if project == nil {
		t.Fatal("expected demo project")
	}
	if project.AuthEnabled == nil || *project.AuthEnabled {
		t.Fatalf("expected auth disabled, got %#v", project.AuthEnabled)
	}
	if project.Capabilities == nil || project.Capabilities.EventStorage == nil || project.Capabilities.EventStorage.Path != ":memory:" {
		t.Fatalf("expected in-memory event storage, got %#v", project.Capabilities)
	}
	if project.Metadata["name"] != demoProjectName {
		t.Fatalf("expected demo display name, got %#v", project.Metadata["name"])
	}
	if len(project.Gateways) != 0 {
		t.Fatalf("expected no demo gateways, got %d", len(project.Gateways))
	}
}

func TestShouldShutdownDemo(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "empty defaults to yes", input: "\n", expected: true},
		{name: "explicit yes", input: "y\n", expected: true},
		{name: "explicit no", input: "n\n", expected: false},
		{name: "full no", input: "no\n", expected: false},
		{name: "unexpected value treated as no", input: "later\n", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := shouldShutdownDemo(strings.NewReader(tt.input)); actual != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}
