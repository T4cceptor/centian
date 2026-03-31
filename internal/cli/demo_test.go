package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	for _, expected := range []string{"agent", "path"} {
		if !flagNames[expected] {
			t.Fatalf("expected %q flag on DemoCommand", expected)
		}
	}
}

func TestResolveDemoRootDefault(t *testing.T) {
	cwd := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(original)
	}()

	root, err := resolveDemoRoot("")
	if err != nil {
		t.Fatalf("resolveDemoRoot: %v", err)
	}
	expected, err := filepath.Abs(filepath.Join(cwd, ".centian", "demo"))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if normalizeDarwinPath(root) != normalizeDarwinPath(expected) {
		t.Fatalf("expected %s, got %s", expected, root)
	}
}

func normalizeDarwinPath(value string) string {
	return strings.TrimPrefix(value, "/private")
}

func TestHandleDemoCommandRequiresAgent(t *testing.T) {
	cmd := &urfavecli.Command{}
	err := handleDemoCommand(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}
