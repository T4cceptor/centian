// Copyright 2026 Centian Contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at.
//
//     http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestMainEntryPoint checks that the main entrypoint can be invoked.
func TestMainEntryPoint(t *testing.T) {
	// Given: run the CLI with help to avoid side effects.
	originalArgs := os.Args
	os.Args = []string{"centian", "help"}
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	stdout, stderr := captureStdoutAndStderr(t, func() {
		// When: invoke the real main entry point.
		main()
	})

	// Then: help text is printed.
	if stdout == "" {
		t.Fatalf("expected help text on stdout; got empty output")
	}
	if !strings.Contains(stdout, "centian") {
		t.Fatalf("expected help output to mention binary name; got %q", stdout)
	}
	if !strings.Contains(stdout, "Proxy and modify your MCP server and tools.") {
		t.Fatalf("expected help output to include description; got %q", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected no stderr output; got %q", stderr)
	}
}

func captureStdoutAndStderr(t *testing.T, fn func()) (string, string) {
	t.Helper()

	originalStdout := os.Stdout
	originalStderr := os.Stderr

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	stdoutDone := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, stdoutReader)
		stdoutDone <- buffer.String()
	}()

	stderrDone := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, stderrReader)
		stderrDone <- buffer.String()
	}()

	fn()

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = originalStdout
	os.Stderr = originalStderr

	stdout := <-stdoutDone
	stderr := <-stderrDone
	_ = stdoutReader.Close()
	_ = stderrReader.Close()
	return stdout, stderr
}
