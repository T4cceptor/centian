package processor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CentianAI/centian-cli/internal/config"
	"gotest.tools/assert"
)

func TestRunScaffoldInteractive(t *testing.T) {
	// Given: interactive input for a python passthrough processor
	tempDir := t.TempDir()
	input := strings.Join([]string{
		"1",
		"1",
		"my_processor",
		tempDir,
		"n",
	}, "\n")
	var output bytes.Buffer

	// When: running the scaffold
	err := RunScaffoldInteractive(strings.NewReader(input), &output)

	// Then: the processor and test input are created
	assert.NilError(t, err)
	processorPath := filepath.Join(tempDir, "my_processor.py")
	assert.Assert(t, exists(processorPath))
	data, readErr := os.ReadFile(processorPath)
	assert.NilError(t, readErr)
	assert.Assert(t, strings.Contains(string(data), "Centian Processor: my_processor"))
	assert.Assert(t, strings.Contains(output.String(), "Processor created"))

	testInputPath := filepath.Join(tempDir, "my_processor_test.json")
	assert.Assert(t, !exists(testInputPath))
}

func TestRunScaffoldInteractive_CancelOverwrite(t *testing.T) {
	// Given: an existing processor file and overwrite declined
	tempDir := t.TempDir()
	processorPath := filepath.Join(tempDir, "existing.py")
	assert.NilError(t, os.WriteFile(processorPath, []byte("original"), 0o644))

	input := strings.Join([]string{
		"1",
		"1",
		"existing",
		tempDir,
		"n",
		"",
	}, "\n")
	var output bytes.Buffer

	// When: running the scaffold
	err := RunScaffoldInteractive(strings.NewReader(input), &output)

	// Then: it cancels and preserves the file
	assert.NilError(t, err)
	data, readErr := os.ReadFile(processorPath)
	assert.NilError(t, readErr)
	assert.Equal(t, string(data), "original")
	assert.Assert(t, strings.Contains(output.String(), "Cancelled."))
}

func TestPromptLanguage(t *testing.T) {
	// Given: language choices
	cases := []struct {
		name     string
		input    string
		expected scaffoldLanguage
		wantErr  bool
	}{
		{"python", "1\n", langPython, false},
		{"javascript", "2\n", langJavaScript, false},
		{"typescript", "3\n", langTypeScript, false},
		{"bash", "4\n", langBash, false},
		{"invalid", "9\n", "", true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			// When: prompting for language
			lang, err := promptLanguage(bufioReader(testCase.input), &output)

			// Then: the selection matches expectations
			if testCase.wantErr {
				assert.Assert(t, err != nil)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, lang, testCase.expected)
		})
	}
}

func TestPromptProcessorType(t *testing.T) {
	// Given: processor type choices
	cases := []struct {
		name     string
		input    string
		expected scaffoldType
		wantErr  bool
	}{
		{"passthrough", "1\n", typePassthrough, false},
		{"validator", "2\n", typeValidator, false},
		{"transformer", "3\n", typeTransformer, false},
		{"logger", "4\n", typeLogger, false},
		{"custom", "5\n", typeCustom, false},
		{"invalid", "9\n", "", true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			// When: prompting for processor type
			procType, err := promptProcessorType(bufioReader(testCase.input), &output)

			// Then: the selection matches expectations
			if testCase.wantErr {
				assert.Assert(t, err != nil)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, procType, testCase.expected)
		})
	}
}

func TestPromptProcessorName(t *testing.T) {
	// Given: various processor names
	cases := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{"simple", "my_processor\n", "my_processor", false},
		{"sanitized", "My Processor!\n", "My_Processor", false},
		{"empty", "\n", "", true},
		{"no-alnum", "!!!\n", "", true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			// When: prompting for processor name
			name, err := promptProcessorName(bufioReader(testCase.input), &output)

			// Then: the result matches expectations
			if testCase.wantErr {
				assert.Assert(t, err != nil)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, name, testCase.expected)
		})
	}
}

func TestPromptOutputDir(t *testing.T) {
	// Given: a temp working directory
	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	assert.NilError(t, err)
	assert.NilError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	// When: selecting default output directory
	defaultDir, err := defaultProcessorDir()
	assert.NilError(t, err)
	output, err := promptOutputDir(bufioReader("\n"), &bytes.Buffer{})

	// Then: default is returned
	assert.NilError(t, err)
	assert.Equal(t, output, defaultDir)

	// When: entering a custom output directory
	customDir := filepath.Join(tempDir, "custom")
	output, err = promptOutputDir(bufioReader(customDir+"\n"), &bytes.Buffer{})

	// Then: custom directory is returned
	assert.NilError(t, err)
	assert.Equal(t, output, customDir)
}

func TestPromptOverwrite(t *testing.T) {
	// Given: overwrite prompt inputs
	cases := []struct {
		name     string
		input    string
		expected bool
	}{
		{"yes", "y\n", true},
		{"yes-uppercase", "Y\n", true},
		{"no", "n\n", false},
		{"empty", "\n", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			// When: prompting for overwrite
			overwrite, err := promptOverwrite(bufioReader(testCase.input), &output, "/tmp/file")

			// Then: selection matches expectations
			assert.NilError(t, err)
			assert.Equal(t, overwrite, testCase.expected)
		})
	}
}

func TestPrompt(t *testing.T) {
	// Given: a prompt input without newline
	var output bytes.Buffer

	// When: prompting for input
	value, err := prompt(bufioReader("value"), &output, "Label: ")

	// Then: the value is returned
	assert.NilError(t, err)
	assert.Equal(t, value, "value")
}

func TestSanitizeName(t *testing.T) {
	// Given: names with spaces and special characters
	cases := []struct {
		input    string
		expected string
	}{
		{"my processor", "my_processor"},
		{"my-processor", "my-processor"},
		{"my.processor", "myprocessor"},
		{"", ""},
	}

	for _, testCase := range cases {
		// When: sanitizing the name
		result := sanitizeName(testCase.input)

		// Then: it is sanitized
		assert.Equal(t, result, testCase.expected)
	}
}

func TestDefaultProcessorDir(t *testing.T) {
	// Given: a temp working directory
	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	assert.NilError(t, err)
	assert.NilError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	// When: resolving the default processor directory
	dir, err := defaultProcessorDir()

	// Then: it points to the working directory
	assert.NilError(t, err)
	expected := fmt.Sprintf("/private%s", tempDir)
	assert.Equal(t, dir, expected)
}

func TestExtensionForLanguage(t *testing.T) {
	// Given: language inputs
	cases := []struct {
		lang     scaffoldLanguage
		expected string
	}{
		{langPython, "py"},
		{langJavaScript, "js"},
		{langTypeScript, "ts"},
		{langBash, "sh"},
		{"unknown", "txt"},
	}

	for _, testCase := range cases {
		// When: resolving extension
		ext := extensionForLanguage(testCase.lang)

		// Then: extension matches
		assert.Equal(t, ext, testCase.expected)
	}
}

func TestGenerateProcessorTemplate(t *testing.T) {
	// Given: language and processor combinations
	cases := []struct {
		name     string
		lang     scaffoldLanguage
		procType scaffoldType
	}{
		{"python", langPython, typePassthrough},
		{"javascript", langJavaScript, typeValidator},
		{"typescript", langTypeScript, typeTransformer},
		{"bash", langBash, typeLogger},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// When: generating a template
			content, err := generateProcessorTemplate(testCase.lang, testCase.procType, "demo")

			// Then: placeholders are replaced
			assert.NilError(t, err)
			assert.Assert(t, strings.Contains(content, "PROCESSOR_NAME") == false)
			assert.Assert(t, strings.Contains(content, "PROCESSOR_TYPE") == false)
			assert.Assert(t, strings.Contains(content, "TIMESTAMP") == false)
			assert.Assert(t, strings.Contains(content, "demo"))
		})
	}

	// Given: an unsupported language
	_, err := generateProcessorTemplate("unknown", typeCustom, "demo")

	// Then: we get an error
	assert.Assert(t, err != nil)
}

func TestLanguageLogic(t *testing.T) {
	// Given: processor types per language
	cases := []struct {
		name     string
		logicFn  func(scaffoldType) string
		procType scaffoldType
		expect   string
	}{
		{"python-passthrough", pythonLogic, typePassthrough, "Passthrough"},
		{"python-validator", pythonLogic, typeValidator, "Delete"},
		{"python-transformer", pythonLogic, typeTransformer, "x-processor"},
		{"python-logger", pythonLogic, typeLogger, "log"},
		{"python-custom", pythonLogic, typeCustom, "TODO"},
		{"python-unknown", pythonLogic, "unknown", ""},
		{"javascript-passthrough", javascriptLogic, typePassthrough, "Passthrough"},
		{"javascript-validator", javascriptLogic, typeValidator, "Delete"},
		{"javascript-transformer", javascriptLogic, typeTransformer, "x-processor"},
		{"javascript-logger", javascriptLogic, typeLogger, "log"},
		{"javascript-custom", javascriptLogic, typeCustom, "TODO"},
		{"javascript-unknown", javascriptLogic, "unknown", ""},
		{"typescript-passthrough", typescriptLogic, typePassthrough, "Passthrough"},
		{"typescript-validator", typescriptLogic, typeValidator, "Delete"},
		{"typescript-transformer", typescriptLogic, typeTransformer, "x-processor"},
		{"typescript-logger", typescriptLogic, typeLogger, "log"},
		{"typescript-custom", typescriptLogic, typeCustom, "TODO"},
		{"typescript-unknown", typescriptLogic, "unknown", ""},
		{"bash-passthrough", bashLogic, typePassthrough, "Passthrough"},
		{"bash-validator", bashLogic, typeValidator, "Delete"},
		{"bash-transformer", bashLogic, typeTransformer, "x-processor"},
		{"bash-logger", bashLogic, typeLogger, "log"},
		{"bash-custom", bashLogic, typeCustom, "TODO"},
		{"bash-unknown", bashLogic, "unknown", ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// When: retrieving logic
			logic := testCase.logicFn(testCase.procType)

			// Then: it matches expectations
			if testCase.expect == "" {
				assert.Equal(t, logic, "")
				return
			}
			assert.Assert(t, strings.Contains(logic, testCase.expect))
		})
	}
}

func TestWriteTestInput(t *testing.T) {
	// Given: a temp file path
	path := filepath.Join(t.TempDir(), "test_input.json")

	// When: writing test input
	err := writeTestInput(path)

	// Then: the file is written with valid JSON
	assert.NilError(t, err)
	data, readErr := os.ReadFile(path)
	assert.NilError(t, readErr)
	var payload map[string]any
	assert.NilError(t, json.Unmarshal(data, &payload))
	assert.Equal(t, payload["type"], "request")
}
func TestAddProcessorToConfig(t *testing.T) {
	// Given: a temp config and processor details
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	cfg := config.DefaultConfig()
	assert.NilError(t, config.SaveConfig(cfg))

	// When: adding a processor to config
	err := addProcessorToConfig("demo", langPython, "/tmp/demo.py")

	// Then: processor is persisted
	assert.NilError(t, err)
	loaded, err := config.LoadConfig()
	assert.NilError(t, err)
	assert.Equal(t, len(loaded.Processors), 1)
	assert.Equal(t, loaded.Processors[0].Name, "demo")
}

func TestPrintNextSteps(t *testing.T) {
	// Given: languages and expected commands
	expected := `For further information read the full documentation at:
   docs/processor_development_guide.md

Happy coding!`
	cases := []struct {
		name string
		lang scaffoldLanguage
	}{
		{"python", langPython},
		{"javascript", langJavaScript},
		{"typescript", langTypeScript},
		{"bash", langBash},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			// When: printing next steps
			printNextSteps(&output, testCase.lang, "demo", "demo_path", true)

			// Then: output includes the runner command
			assert.Assert(t, strings.Contains(output.String(), expected))
		})
	}
}

func TestExists(t *testing.T) {
	// Given: an existing file and a missing file
	path := filepath.Join(t.TempDir(), "exists.txt")
	assert.NilError(t, os.WriteFile(path, []byte("ok"), 0o644))

	// Then: exists behaves as expected
	assert.Assert(t, exists(path))
	assert.Assert(t, !exists(filepath.Join(t.TempDir(), "missing.txt")))
}

func bufioReader(input string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(input))
}
