package taskverification

import "fmt"

// ConditionTechnicalDescription explains the built-in meaning of a condition in concise prose.
func ConditionTechnicalDescription(condition Condition) string {
	switch condition.Type {
	case "exit_code":
		return "Checks that the command exits with the expected exit code."
	case "exit_code_in":
		return "Checks that the command exits with one of the allowed exit codes."
	case "stdout_contains":
		return "Checks that stdout includes the expected substring."
	case "stdout_not_contains":
		return "Checks that stdout does not include the forbidden substring."
	case "output_contains":
		return "Checks that combined stdout and stderr include the expected substring."
	case "output_not_contains":
		return "Checks that combined stdout and stderr do not include the forbidden substring."
	case "file_exists":
		return fmt.Sprintf("Checks that a file is available at %q.", condition.Path)
	case "file_not_exists":
		return fmt.Sprintf("Checks that no file is available at %q.", condition.Path)
	case "file_contains":
		return fmt.Sprintf("Checks that %q includes the expected substring.", condition.Path)
	case "file_not_contains":
		return fmt.Sprintf("Checks that %q does not include the forbidden substring.", condition.Path)
	default:
		return ""
	}
}

// InvariantTechnicalDescription explains how Centian enforces an invariant.
func InvariantTechnicalDescription(_ Invariant) string {
	return "Captures the command output when the step starts and requires it to remain unchanged until the step completes."
}
