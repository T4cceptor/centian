package taskverification

import (
	"os"
	"path/filepath"
	"strings"
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

func TestEvaluateOutputContainsCondition_MatchesStderr(t *testing.T) {
	condition := Condition{Type: "output_contains", Value: "FAIL"}
	result := &commandResult{Stdout: "", Stderr: "FAIL: test case"}

	err := evaluateOutputContainsCondition(condition, result, "")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEvaluateOutputNotContainsCondition_UsesCombinedStreams(t *testing.T) {
	condition := Condition{Type: "output_not_contains", Value: "panic"}
	result := &commandResult{Stdout: "ok", Stderr: "still ok"}

	err := evaluateOutputNotContainsCondition(condition, result, "")

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

func TestEvaluateFileExistsRejectsAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	absolutePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(absolutePath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := evaluateFileExists(absolutePath, tmpDir)

	if err == nil || !strings.Contains(err.Error(), "must be relative") {
		t.Fatalf("expected absolute path rejection, got %v", err)
	}
}

func TestEvaluateFileContainsRejectsTraversalOutsideWorkingDirectory(t *testing.T) {
	rootDir := t.TempDir()
	parentDir := filepath.Dir(rootDir)
	outsideName := "outside.txt"
	outsidePath := filepath.Join(parentDir, outsideName)
	if err := os.WriteFile(outsidePath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outsidePath)

	err := evaluateFileContains(filepath.Join("..", outsideName), "content", rootDir)

	if err == nil || !strings.Contains(err.Error(), "escapes the working directory") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestEvaluateFileContainsAllowsNestedCleanedInRootPath(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "test.txt"), []byte("file content here"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := evaluateFileContains(filepath.Join("nested", "..", "nested", "test.txt"), "content", tmpDir)

	if err != nil {
		t.Fatalf("expected cleaned in-root path to succeed, got %v", err)
	}
}
