package common

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInternalLoggerSupportsConfiguredOutputs(t *testing.T) {
	tests := []struct {
		name           string
		options        LoggerOptions
		wantInFile     bool
		wantInConsole  bool
		unwantedInFile []string
	}{
		{
			name: "file output respects log level",
			options: LoggerOptions{
				Level:    "warn",
				Output:   "file",
				FilePath: filepath.Join(t.TempDir(), "centian.log"),
			},
			wantInFile:     true,
			wantInConsole:  false,
			unwantedInFile: []string{"[DEBUG] debug message", "[INFO] info message"},
		},
		{
			name: "console output only",
			options: LoggerOptions{
				Level:  "debug",
				Output: "console",
			},
			wantInFile:    false,
			wantInConsole: true,
		},
		{
			name: "both outputs",
			options: LoggerOptions{
				Level:    "debug",
				Output:   "both",
				FilePath: filepath.Join(t.TempDir(), "centian.log"),
			},
			wantInFile:    true,
			wantInConsole: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var console bytes.Buffer
			tt.options.ConsoleWriter = &console

			if err := InitInternalLogger(tt.options); err != nil {
				t.Fatalf("InitInternalLogger() error = %v", err)
			}
			t.Cleanup(func() {
				_ = CloseLogger()
			})

			LogDebug("debug message")
			LogInfo("info message")
			LogWarn("warn message")
			LogError("error message")

			if err := CloseLogger(); err != nil {
				t.Fatalf("CloseLogger() error = %v", err)
			}

			consoleOutput := console.String()
			if tt.wantInConsole {
				for _, expected := range []string{"[DEBUG] debug message", "[INFO] info message", "[WARN] warn message", "[ERROR] error message"} {
					if !strings.Contains(consoleOutput, expected) {
						t.Fatalf("expected console output to contain %q, got %q", expected, consoleOutput)
					}
				}
			} else if consoleOutput != "" {
				t.Fatalf("expected no console output, got %q", consoleOutput)
			}

			if tt.wantInFile {
				data, err := os.ReadFile(tt.options.FilePath)
				if err != nil {
					t.Fatalf("os.ReadFile() error = %v", err)
				}
				fileOutput := string(data)
				for _, expected := range []string{"[WARN] warn message", "[ERROR] error message"} {
					if !strings.Contains(fileOutput, expected) {
						t.Fatalf("expected file output to contain %q, got %q", expected, fileOutput)
					}
				}
				for _, unexpected := range tt.unwantedInFile {
					if strings.Contains(fileOutput, unexpected) {
						t.Fatalf("expected file output not to contain %q, got %q", unexpected, fileOutput)
					}
				}
			} else if tt.options.FilePath != "" {
				if _, err := os.Stat(tt.options.FilePath); !os.IsNotExist(err) {
					t.Fatalf("expected no log file at %s", tt.options.FilePath)
				}
			}
		})
	}
}

func TestInitInternalLoggerRejectsInvalidOptions(t *testing.T) {
	t.Cleanup(func() {
		_ = CloseLogger()
	})

	if err := InitInternalLogger(LoggerOptions{Level: "trace"}); err == nil {
		t.Fatal("expected invalid log level to fail")
	}

	if err := InitInternalLogger(LoggerOptions{Output: "syslog"}); err == nil {
		t.Fatal("expected invalid log output to fail")
	}
}
