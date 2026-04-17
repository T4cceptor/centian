package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadYAMLFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.yaml")
	if err := os.WriteFile(path, []byte("name: demo\ncount: 2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var payload struct {
		Name  string `yaml:"name"`
		Count int    `yaml:"count"`
	}
	if err := ReadYAMLFile(path, &payload); err != nil {
		t.Fatalf("ReadYAMLFile: %v", err)
	}
	if payload.Name != "demo" || payload.Count != 2 {
		t.Fatalf("unexpected YAML payload: %#v", payload)
	}
}

func TestResolvePathUnderRoot(t *testing.T) {
	root := t.TempDir()

	resolved, err := ResolvePathUnderRoot(root, "cases/demo", "case path", "suite root")
	if err != nil {
		t.Fatalf("ResolvePathUnderRoot: %v", err)
	}
	if resolved != filepath.Join(root, "cases", "demo") {
		t.Fatalf("unexpected resolved path: %q", resolved)
	}

	_, err = ResolvePathUnderRoot(root, "/tmp/demo", "case path", "suite root")
	if err == nil || !strings.Contains(err.Error(), `case path "/tmp/demo" must be relative`) {
		t.Fatalf("expected absolute path rejection, got %v", err)
	}

	_, err = ResolvePathUnderRoot(root, "../escape", "case path", "suite root")
	if err == nil || !strings.Contains(err.Error(), `case path "../escape" must stay within the suite root`) {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestResolveExistingUnderRoot(t *testing.T) {
	root := t.TempDir()
	dirPath := filepath.Join(root, "fixture")
	filePath := filepath.Join(dirPath, "prompt.yaml")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("prompt: hi\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolvedFile, err := ResolveExistingFileUnderRoot(root, "fixture/prompt.yaml", "prompt file", "suite root")
	if err != nil {
		t.Fatalf("ResolveExistingFileUnderRoot: %v", err)
	}
	if resolvedFile != filePath {
		t.Fatalf("unexpected file path: %q", resolvedFile)
	}

	resolvedDir, err := ResolveExistingDirUnderRoot(root, "fixture", "fixture path", "suite root")
	if err != nil {
		t.Fatalf("ResolveExistingDirUnderRoot: %v", err)
	}
	if resolvedDir != dirPath {
		t.Fatalf("unexpected dir path: %q", resolvedDir)
	}

	if err := EnsureExistingPathUnderRoot(root, "fixture/prompt.yaml", "prompt file", "suite root"); err != nil {
		t.Fatalf("EnsureExistingPathUnderRoot: %v", err)
	}

	if _, err := ResolveExistingFileUnderRoot(root, "fixture", "prompt file", "suite root"); err == nil || !strings.Contains(err.Error(), `prompt file "fixture" must be a file`) {
		t.Fatalf("expected file-type rejection, got %v", err)
	}
	if _, err := ResolveExistingDirUnderRoot(root, "fixture/prompt.yaml", "fixture path", "suite root"); err == nil || !strings.Contains(err.Error(), `fixture path "fixture/prompt.yaml" must be a directory`) {
		t.Fatalf("expected dir-type rejection, got %v", err)
	}
	if err := EnsureExistingPathUnderRoot(root, "missing", "locked path", "suite root"); err == nil || !strings.Contains(err.Error(), `locked path "missing" does not exist`) {
		t.Fatalf("expected missing-path rejection, got %v", err)
	}
}
