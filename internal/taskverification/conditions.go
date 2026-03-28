package taskverification

import (
	"fmt"
	"os"
	"strings"
)

type conditionHandler struct {
	validate func(condition Condition) error
	evaluate func(condition Condition, result *commandResult, workingDir string) error
}

var conditionRegistry = map[string]conditionHandler{
	"exit_code": {
		validate: validateNumericValueCondition,
		evaluate: evaluateExitCodeCondition,
	},
	"exit_code_in": {
		validate: validateExitCodeInCondition,
		evaluate: evaluateExitCodeInCondition,
	},
	"stdout_contains": {
		validate: validateStringValueCondition,
		evaluate: evaluateStdoutContainsCondition,
	},
	"stdout_not_contains": {
		validate: validateStringValueCondition,
		evaluate: evaluateStdoutNotContainsCondition,
	},
	"file_exists": {
		validate: validatePathCondition,
		evaluate: evaluateFileExistsCondition,
	},
	"file_not_exists": {
		validate: validatePathCondition,
		evaluate: evaluateFileNotExistsCondition,
	},
	"file_contains": {
		validate: validatePathStringValueCondition,
		evaluate: evaluateFileContainsCondition,
	},
	"file_not_contains": {
		validate: validatePathStringValueCondition,
		evaluate: evaluateFileNotContainsCondition,
	},
}

func validateNumericValueCondition(condition Condition) error {
	if condition.Value == nil {
		return fmt.Errorf("value is required")
	}
	if _, err := intFromValue(condition.Value); err != nil {
		return err
	}
	return nil
}

func validateExitCodeInCondition(condition Condition) error {
	if len(condition.Values) == 0 {
		return fmt.Errorf("values must not be empty")
	}
	for _, value := range condition.Values {
		if _, err := intFromValue(value); err != nil {
			return err
		}
	}
	return nil
}

func validateStringValueCondition(condition Condition) error {
	value, ok := condition.Value.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fmt.Errorf("value must be a non-empty string")
	}
	return nil
}

func validatePathCondition(condition Condition) error {
	if strings.TrimSpace(condition.Path) == "" {
		return fmt.Errorf("path is required")
	}
	return nil
}

func validatePathStringValueCondition(condition Condition) error {
	if err := validatePathCondition(condition); err != nil {
		return err
	}
	return validateStringValueCondition(condition)
}

func evaluateExitCodeCondition(condition Condition, result *commandResult, _ string) error {
	expected, err := intFromValue(condition.Value)
	if err != nil {
		return err
	}
	if result.ExitCode != expected {
		return fmt.Errorf("expected exit code %d, got %d", expected, result.ExitCode)
	}
	return nil
}

func evaluateExitCodeInCondition(condition Condition, result *commandResult, _ string) error {
	allowed := make([]int, 0, len(condition.Values))
	for _, value := range condition.Values {
		exitCode, err := intFromValue(value)
		if err != nil {
			return err
		}
		allowed = append(allowed, exitCode)
		if result.ExitCode == exitCode {
			return nil
		}
	}
	return fmt.Errorf("expected exit code in %v, got %d", allowed, result.ExitCode)
}

func evaluateStdoutContainsCondition(condition Condition, result *commandResult, _ string) error {
	value, ok := condition.Value.(string)
	if !ok {
		return fmt.Errorf("stdout_contains condition requires a string value, got %T", condition.Value)
	}
	return evaluateStdoutContains(value, result.Stdout)
}

func evaluateStdoutNotContainsCondition(condition Condition, result *commandResult, _ string) error {
	value, ok := condition.Value.(string)
	if !ok {
		return fmt.Errorf("stdout_not_contains condition requires a string value, got %T", condition.Value)
	}
	return evaluateStdoutNotContains(value, result.Stdout)
}

func evaluateFileExistsCondition(condition Condition, _ *commandResult, workingDir string) error {
	return evaluateFileExists(condition.Path, workingDir)
}

func evaluateFileNotExistsCondition(condition Condition, _ *commandResult, workingDir string) error {
	return evaluateFileNotExists(condition.Path, workingDir)
}

func evaluateFileContainsCondition(condition Condition, _ *commandResult, workingDir string) error {
	value, ok := condition.Value.(string)
	if !ok {
		return fmt.Errorf("file_contains condition requires a string value, got %T", condition.Value)
	}
	return evaluateFileContains(condition.Path, value, workingDir)
}

func evaluateFileNotContainsCondition(condition Condition, _ *commandResult, workingDir string) error {
	value, ok := condition.Value.(string)
	if !ok {
		return fmt.Errorf("file_not_contains condition requires a string value, got %T", condition.Value)
	}
	return evaluateFileNotContains(condition.Path, value, workingDir)
}

func evaluateStdoutContains(expected, stdout string) error {
	if !strings.Contains(stdout, expected) {
		return fmt.Errorf("expected stdout to contain %q", expected)
	}
	return nil
}

func evaluateStdoutNotContains(unexpected, stdout string) error {
	if strings.Contains(stdout, unexpected) {
		return fmt.Errorf("expected stdout not to contain %q", unexpected)
	}
	return nil
}

func evaluateFileExists(path, workingDir string) error {
	resolvedPath := resolvePath(workingDir, path)
	if _, err := os.Stat(resolvedPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("expected file %q to exist", path)
		}
		return fmt.Errorf("failed to stat file %q: %w", path, err)
	}
	return nil
}

func evaluateFileNotExists(path, workingDir string) error {
	resolvedPath := resolvePath(workingDir, path)
	if _, err := os.Stat(resolvedPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat file %q: %w", path, err)
	}
	return fmt.Errorf("expected file %q not to exist", path)
}

func evaluateFileContains(path, expected, workingDir string) error {
	resolvedPath := resolvePath(workingDir, path)
	// #nosec G304 -- task verification intentionally reads template-defined files relative to the working directory.
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", path, err)
	}
	if !strings.Contains(string(content), expected) {
		return fmt.Errorf("expected file %q to contain %q", path, expected)
	}
	return nil
}

func evaluateFileNotContains(path, unexpected, workingDir string) error {
	resolvedPath := resolvePath(workingDir, path)
	// #nosec G304 -- task verification intentionally reads template-defined files relative to the working directory.
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", path, err)
	}
	if strings.Contains(string(content), unexpected) {
		return fmt.Errorf("expected file %q not to contain %q", path, unexpected)
	}
	return nil
}
