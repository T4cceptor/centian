package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPromptDefinition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.yaml")
	if err := os.WriteFile(path, []byte("prompt: |\n  hello world\n"), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	prompt, err := LoadPromptDefinition(path)
	if err != nil {
		t.Fatalf("LoadPromptDefinition: %v", err)
	}
	if prompt.Prompt != "hello world" {
		t.Fatalf("unexpected prompt %q", prompt.Prompt)
	}
}

func TestLoadPromptDefinitionRejectsEmptyPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.yaml")
	if err := os.WriteFile(path, []byte("prompt: \"\""), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	_, err := LoadPromptDefinition(path)
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}
