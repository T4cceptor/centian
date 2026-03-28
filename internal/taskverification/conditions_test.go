package taskverification

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateStdoutContainsCondition_NonStringValue(t *testing.T) {
	// Given: a condition with a non-string value
	condition := Condition{Type: "stdout_contains", Value: 42}
	result := &commandResult{Stdout: "hello world"}

	// When: evaluating the condition
	err := evaluateStdoutContainsCondition(condition, result, "")

	// Then: an error is returned instead of a panic
	if err == nil {
		t.Fatal("expected error for non-string value, got nil")
	}
}

func TestEvaluateStdoutContainsCondition_ValidString(t *testing.T) {
	// Given: a condition with a valid string value
	condition := Condition{Type: "stdout_contains", Value: "hello"}
	result := &commandResult{Stdout: "hello world"}

	// When: evaluating the condition
	err := evaluateStdoutContainsCondition(condition, result, "")

	// Then: no error is returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEvaluateStdoutNotContainsCondition_NonStringValue(t *testing.T) {
	// Given: a condition with a non-string value
	condition := Condition{Type: "stdout_not_contains", Value: true}
	result := &commandResult{Stdout: "hello world"}

	// When: evaluating the condition
	err := evaluateStdoutNotContainsCondition(condition, result, "")

	// Then: an error is returned instead of a panic
	if err == nil {
		t.Fatal("expected error for non-string value, got nil")
	}
}

func TestEvaluateStdoutNotContainsCondition_ValidString(t *testing.T) {
	// Given: a condition with a valid string value not present in stdout
	condition := Condition{Type: "stdout_not_contains", Value: "missing"}
	result := &commandResult{Stdout: "hello world"}

	// When: evaluating the condition
	err := evaluateStdoutNotContainsCondition(condition, result, "")

	// Then: no error is returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEvaluateFileContainsCondition_NonStringValue(t *testing.T) {
	// Given: a condition with a non-string value and a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("file content"), 0o644); err != nil {
		t.Fatal(err)
	}
	condition := Condition{Type: "file_contains", Value: 3.14, Path: "test.txt"}

	// When: evaluating the condition
	err := evaluateFileContainsCondition(condition, nil, tmpDir)

	// Then: an error is returned instead of a panic
	if err == nil {
		t.Fatal("expected error for non-string value, got nil")
	}
}

func TestEvaluateFileContainsCondition_ValidString(t *testing.T) {
	// Given: a condition with a valid string value and a matching file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("file content here"), 0o644); err != nil {
		t.Fatal(err)
	}
	condition := Condition{Type: "file_contains", Value: "content", Path: "test.txt"}

	// When: evaluating the condition
	err := evaluateFileContainsCondition(condition, nil, tmpDir)

	// Then: no error is returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEvaluateFileNotContainsCondition_NonStringValue(t *testing.T) {
	// Given: a condition with a non-string value and a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("file content"), 0o644); err != nil {
		t.Fatal(err)
	}
	condition := Condition{Type: "file_not_contains", Value: []int{1, 2}, Path: "test.txt"}

	// When: evaluating the condition
	err := evaluateFileNotContainsCondition(condition, nil, tmpDir)

	// Then: an error is returned instead of a panic
	if err == nil {
		t.Fatal("expected error for non-string value, got nil")
	}
}

func TestEvaluateFileNotContainsCondition_ValidString(t *testing.T) {
	// Given: a condition with a valid string value not present in the file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("file content here"), 0o644); err != nil {
		t.Fatal(err)
	}
	condition := Condition{Type: "file_not_contains", Value: "missing", Path: "test.txt"}

	// When: evaluating the condition
	err := evaluateFileNotContainsCondition(condition, nil, tmpDir)

	// Then: no error is returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
