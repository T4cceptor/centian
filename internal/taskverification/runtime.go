package taskverification

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type commandResult struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
}

const outputSnippetLimit = 240

type stepFailureDetails struct {
	kind            StepFailureKind
	phase           StepFailurePhase
	failedCheckID   string
	failedInvariant string
	summary         string
	exitCode        *int
	stdoutSnippet   string
	stderrSnippet   string
}

// StartStep validates the next step, runs its preconditions, and captures invariant baselines.
func (s *Service) StartStep(run *RunState, stepNumber int) (*StepResult, error) {
	if err := validateTaskExecutable(run); err != nil {
		return nil, err
	}
	stepIndex, err := validateStepRequest(run, stepNumber)
	if err != nil {
		return nil, err
	}
	if handled, result, err := s.completePreviousStepIfNeeded(run, stepIndex); err != nil {
		return nil, err
	} else if handled {
		return result, err
	}
	if err := ensureStepCanStart(run, stepIndex); err != nil {
		return nil, err
	}

	step := executionStep(run, stepIndex)
	if result, failed := s.runPhaseChecks(run, step, stepIndex, stepNumber, StepFailurePhasePrecondition, func(check *Check) []Condition {
		return check.PreConditions
	}); failed {
		return result, nil
	}
	baselines, result, failed := s.captureInvariantBaselines(run, step, stepIndex, stepNumber)
	if failed {
		return result, nil
	}

	run.LastFailureMessage = ""
	run.Steps[stepIndex].Status = StepStatusActive
	run.Steps[stepIndex].InvariantBaselines = baselines

	return &StepResult{
		Passed:     true,
		Message:    fmt.Sprintf("step %d (%s) started", stepNumber, step.ID),
		Summary:    fmt.Sprintf("step %d (%s) started", stepNumber, step.ID),
		Step:       stepNumber,
		StepID:     step.ID,
		Status:     run.Status,
		Phase:      run.Phase,
		StepStatus: run.Steps[stepIndex].Status,
	}, nil
}

// CompleteStep runs postconditions and invariant verification for an active step.
func (s *Service) CompleteStep(run *RunState, stepNumber int) (*StepResult, error) {
	if err := validateTaskExecutable(run); err != nil {
		return nil, err
	}
	stepIndex, err := validateStepRequest(run, stepNumber)
	if err != nil {
		return nil, err
	}
	return s.completeStep(run, stepIndex)
}

func (s *Service) completeStep(run *RunState, stepIndex int) (*StepResult, error) {
	if err := validateActiveStep(run, stepIndex); err != nil {
		return nil, err
	}
	if run.Steps[stepIndex].Status != StepStatusActive {
		return nil, fmt.Errorf("step %d (%s) is not active", stepIndex+1, run.Steps[stepIndex].ID)
	}

	step := executionStep(run, stepIndex)
	if result, failed := s.runPhaseChecks(run, step, stepIndex, stepIndex+1, StepFailurePhasePostcondition, func(check *Check) []Condition {
		return check.PostConditions
	}); failed {
		return result, nil
	}
	if result, failed := s.verifyInvariants(run, step, stepIndex); failed {
		return result, nil
	}

	run.LastFailureMessage = ""
	run.Steps[stepIndex].Status = StepStatusPassed
	run.Steps[stepIndex].InvariantBaselines = make(map[string]string)

	message := fmt.Sprintf("step %d (%s) completed", stepIndex+1, step.ID)
	if step.NextPath == "" {
		run.Status = TaskStatusCompleted
		message = fmt.Sprintf("%s; task completed", message)
	} else {
		run.Phase = step.NextPath
	}

	return &StepResult{
		Passed:     true,
		Message:    message,
		Summary:    message,
		Step:       stepIndex + 1,
		StepID:     step.ID,
		Status:     run.Status,
		Phase:      run.Phase,
		StepStatus: run.Steps[stepIndex].Status,
	}, nil
}

func validateStepRequest(run *RunState, stepNumber int) (int, error) {
	if run == nil {
		return 0, fmt.Errorf("task is not registered")
	}
	if run.ExecutionTemplate == nil {
		return 0, fmt.Errorf("task has no execution contract")
	}
	if run.ExecutionTemplate.CompiledWorkflow == nil {
		return 0, fmt.Errorf("task has no compiled execution workflow")
	}
	if stepNumber < 1 || stepNumber > len(run.ExecutionTemplate.CompiledWorkflow.ExecutionSteps) {
		return 0, fmt.Errorf("step %d is out of range", stepNumber)
	}
	return stepNumber - 1, nil
}

func validateActiveStep(run *RunState, stepIndex int) error {
	if err := validateTaskExecutable(run); err != nil {
		return err
	}
	if stepIndex < 0 || stepIndex >= len(run.ExecutionTemplate.CompiledWorkflow.ExecutionSteps) {
		return fmt.Errorf("step %d is out of range", stepIndex+1)
	}
	if executionStep(run, stepIndex).Path != run.Phase {
		return fmt.Errorf("task is currently at %s; step %d (%s) is not the active workflow node", run.Phase, stepIndex+1, run.Steps[stepIndex].ID)
	}
	return nil
}

func validateTaskExecutable(run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}
	switch run.Status {
	case TaskStatusActive:
	case TaskStatusCompleted:
		return fmt.Errorf("task is already completed")
	case TaskStatusFailed:
		return fmt.Errorf("task is failed; restart or register a new task")
	default:
		return fmt.Errorf("task is %s", run.Status)
	}
	node, exists := run.CurrentNode()
	if !exists {
		return fmt.Errorf("task is in unknown workflow phase %s", run.Phase)
	}
	if node.Kind != WorkflowNodeKindExecution {
		return fmt.Errorf("task is in %s phase; step execution is only allowed in execution nodes", run.Phase)
	}
	if !run.ExecutionReady || run.ExecutionTemplate == nil {
		return fmt.Errorf("task has no execution contract")
	}
	return nil
}

func (s *Service) completePreviousStepIfNeeded(run *RunState, stepIndex int) (bool, *StepResult, error) {
	if stepIndex == 0 || run.Steps[stepIndex-1].Status != StepStatusActive {
		return false, nil, nil
	}
	result, err := s.completeStep(run, stepIndex-1)
	if err != nil {
		return true, nil, err
	}
	if !result.Passed {
		return true, result, nil
	}
	return false, nil, nil
}

func (s *Service) runPhaseChecks(run *RunState, step *Step, stepIndex, stepNumber int, phase StepFailurePhase, conditions func(check *Check) []Condition) (*StepResult, bool) {
	for checkIndex := range step.Checks {
		check := &step.Checks[checkIndex]
		result, err := s.runCommand(context.Background(), check.Command)
		if err != nil {
			return s.executionFailure(run, step, stepIndex, stepNumber, check.ID, result, err), true
		}
		if err := evaluateConditions(conditions(check), result, s.WorkingDir); err != nil {
			details := failureDetailsFromCommand(
				StepFailureKindCheck,
				phase,
				result,
				fmt.Sprintf("step %d (%s) %s failed for check %s", stepNumber, step.ID, phase, check.ID),
			)
			details.failedCheckID = check.ID
			details.summary = fmt.Sprintf("%s: %v", details.summary, err)
			return failureResult(run, stepIndex, stepNumber, step.ID, details), true
		}
	}
	return nil, false
}

func (s *Service) captureInvariantBaselines(run *RunState, step *Step, stepIndex, stepNumber int) (map[string]string, *StepResult, bool) {
	baselines := make(map[string]string, len(step.Invariants))
	for _, invariant := range step.Invariants {
		result, err := s.runCommand(context.Background(), invariant.Command)
		if err != nil {
			return nil, s.invariantExecutionFailure(run, step, stepIndex, stepNumber, invariant.ID, StepFailurePhaseInvariantCapture, result, err), true
		}
		if result.ExitCode != 0 {
			run.Steps[stepIndex].Status = StepStatusFailed
			details := failureDetailsFromCommand(
				StepFailureKindInvariant,
				StepFailurePhaseInvariantCapture,
				result,
				fmt.Sprintf("step %d (%s) invariant %s failed to capture baseline", stepNumber, step.ID, invariant.ID),
			)
			details.failedInvariant = invariant.ID
			details.summary = fmt.Sprintf("%s: expected exit code 0, got %d", details.summary, result.ExitCode)
			return nil, failureResult(run, stepIndex, stepNumber, step.ID, details), true
		}
		baselines[invariant.ID] = result.Stdout
	}
	return baselines, nil, false
}

func (s *Service) verifyInvariants(run *RunState, step *Step, stepIndex int) (*StepResult, bool) {
	stepNumber := stepIndex + 1
	for _, invariant := range step.Invariants {
		result, err := s.runCommand(context.Background(), invariant.Command)
		if err != nil {
			return s.invariantExecutionFailure(run, step, stepIndex, stepNumber, invariant.ID, StepFailurePhaseInvariantVerify, result, err), true
		}
		if failure := verifyInvariantResult(run, step, stepIndex, result, invariant); failure != nil {
			return failure, true
		}
	}
	return nil, false
}

func (s *Service) executionFailure(run *RunState, step *Step, stepIndex, stepNumber int, checkID string, result *commandResult, err error) *StepResult {
	run.Steps[stepIndex].Status = StepStatusFailed
	details := failureDetailsFromCommand(
		StepFailureKindCommandExecution,
		StepFailurePhaseCommandExecution,
		result,
		fmt.Sprintf("step %d (%s) could not execute check %s", stepNumber, step.ID, checkID),
	)
	details.failedCheckID = checkID
	details.summary = fmt.Sprintf("%s: %v", details.summary, err)
	return failureResult(run, stepIndex, stepNumber, step.ID, details)
}

func (s *Service) invariantExecutionFailure(
	run *RunState,
	step *Step,
	stepIndex, stepNumber int,
	invariantID string,
	phase StepFailurePhase,
	result *commandResult,
	err error,
) *StepResult {
	run.Steps[stepIndex].Status = StepStatusFailed
	details := failureDetailsFromCommand(
		StepFailureKindCommandExecution,
		StepFailurePhaseCommandExecution,
		result,
		fmt.Sprintf("step %d (%s) could not %s invariant %s", stepNumber, step.ID, phaseActionLabel(phase), invariantID),
	)
	details.failedInvariant = invariantID
	details.summary = fmt.Sprintf("%s: %v", details.summary, err)
	return failureResult(run, stepIndex, stepNumber, step.ID, details)
}

func verifyInvariantResult(run *RunState, step *Step, stepIndex int, result *commandResult, invariant Invariant) *StepResult {
	stepNumber := stepIndex + 1
	if result.ExitCode != 0 {
		details := failureDetailsFromCommand(
			StepFailureKindInvariant,
			StepFailurePhaseInvariantVerify,
			result,
			fmt.Sprintf("step %d (%s) invariant %s failed during verification", stepNumber, step.ID, invariant.ID),
		)
		details.failedInvariant = invariant.ID
		details.summary = fmt.Sprintf("%s: expected exit code 0, got %d", details.summary, result.ExitCode)
		return failureResult(run, stepIndex, stepNumber, step.ID, details)
	}
	baseline, exists := run.Steps[stepIndex].InvariantBaselines[invariant.ID]
	if !exists {
		details := failureDetailsFromCommand(
			StepFailureKindInvariant,
			StepFailurePhaseInvariantVerify,
			result,
			fmt.Sprintf("step %d (%s) invariant %s has no baseline", stepNumber, step.ID, invariant.ID),
		)
		details.failedInvariant = invariant.ID
		return failureResult(run, stepIndex, stepNumber, step.ID, details)
	}
	if baseline != result.Stdout {
		details := failureDetailsFromCommand(
			StepFailureKindInvariant,
			StepFailurePhaseInvariantVerify,
			result,
			fmt.Sprintf("step %d (%s) invariant %s changed", stepNumber, step.ID, invariant.ID),
		)
		details.failedInvariant = invariant.ID
		details.summary = fmt.Sprintf(
			"%s: expected %q, got %q",
			details.summary,
			truncateOutput(baseline),
			truncateOutput(result.Stdout),
		)
		return failureResult(run, stepIndex, stepNumber, step.ID, details)
	}
	return nil
}

func failureResult(run *RunState, stepIndex, stepNumber int, stepID string, details stepFailureDetails) *StepResult {
	message := details.summary
	run.LastFailureMessage = message
	result := &StepResult{
		Passed:            false,
		Message:           message,
		Summary:           details.summary,
		Step:              stepNumber,
		StepID:            stepID,
		Status:            run.Status,
		Phase:             run.Phase,
		StepStatus:        run.Steps[stepIndex].Status,
		FailureKind:       details.kind,
		FailurePhase:      details.phase,
		FailedCheckID:     details.failedCheckID,
		FailedInvariantID: details.failedInvariant,
		ExitCode:          details.exitCode,
		StdoutSnippet:     details.stdoutSnippet,
		StderrSnippet:     details.stderrSnippet,
	}
	return result
}

func ensureStepCanStart(run *RunState, stepIndex int) error {
	if stepIndex < 0 || stepIndex >= len(run.Steps) {
		return fmt.Errorf("step %d is out of range", stepIndex+1)
	}
	if executionStep(run, stepIndex).Path != run.Phase {
		return fmt.Errorf("task is currently at %s; step %d (%s) is not the active workflow node", run.Phase, stepIndex+1, run.Steps[stepIndex].ID)
	}
	for previous := 0; previous < stepIndex; previous++ {
		if run.Steps[previous].Status != StepStatusPassed {
			return fmt.Errorf("step %d (%s) cannot start before step %d (%s) passes", stepIndex+1, run.Steps[stepIndex].ID, previous+1, run.Steps[previous].ID)
		}
	}
	switch run.Steps[stepIndex].Status {
	case StepStatusPending:
		return nil
	case StepStatusActive:
		return fmt.Errorf("step %d (%s) is already active", stepIndex+1, run.Steps[stepIndex].ID)
	case StepStatusPassed:
		return fmt.Errorf("step %d (%s) already passed", stepIndex+1, run.Steps[stepIndex].ID)
	case StepStatusFailed:
		return fmt.Errorf("step %d (%s) failed; restart the task to continue", stepIndex+1, run.Steps[stepIndex].ID)
	default:
		return fmt.Errorf("step %d (%s) has invalid status %q", stepIndex+1, run.Steps[stepIndex].ID, run.Steps[stepIndex].Status)
	}
}

func executionStep(run *RunState, stepIndex int) *Step {
	return &run.ExecutionTemplate.CompiledWorkflow.ExecutionSteps[stepIndex]
}

func (s *Service) runCommand(ctx context.Context, command string) (*commandResult, error) {
	// #nosec G204 -- task verification intentionally executes template-defined commands.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = s.WorkingDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &commandResult{
		Command: command,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
	}
	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	var exitErr *exec.ExitError
	if ok := errorAs(err, &exitErr); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func failureDetailsFromCommand(kind StepFailureKind, phase StepFailurePhase, result *commandResult, summary string) stepFailureDetails {
	details := stepFailureDetails{
		kind:    kind,
		phase:   phase,
		summary: summary,
	}
	if result == nil {
		return details
	}
	exitCode := result.ExitCode
	details.exitCode = &exitCode
	details.stdoutSnippet = truncateOutput(result.Stdout)
	details.stderrSnippet = truncateOutput(result.Stderr)
	return details
}

func truncateOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= outputSnippetLimit {
		return trimmed
	}
	return string(runes[:outputSnippetLimit]) + "..."
}

func phaseActionLabel(phase StepFailurePhase) string {
	switch phase {
	case StepFailurePhaseInvariantCapture:
		return "capture"
	case StepFailurePhaseInvariantVerify:
		return "verify"
	default:
		return "execute"
	}
}

func evaluateConditions(conditions []Condition, result *commandResult, workingDir string) error {
	for _, condition := range conditions {
		if err := evaluateCondition(condition, result, workingDir); err != nil {
			return err
		}
	}
	return nil
}

func evaluateCondition(condition Condition, result *commandResult, workingDir string) error {
	switch condition.Type {
	case "exit_code":
		return evaluateExitCodeCondition(condition, result)
	case "exit_code_in":
		return evaluateExitCodeInCondition(condition, result)
	case "stdout_contains":
		return evaluateStdoutContains(condition.Value.(string), result.Stdout)
	case "stdout_not_contains":
		return evaluateStdoutNotContains(condition.Value.(string), result.Stdout)
	case "file_exists":
		return evaluateFileExists(condition.Path, workingDir)
	case "file_contains":
		return evaluateFileContains(condition.Path, condition.Value.(string), workingDir)
	default:
		return fmt.Errorf("unsupported condition type %q", condition.Type)
	}
}

func evaluateExitCodeCondition(condition Condition, result *commandResult) error {
	expected, err := intFromValue(condition.Value)
	if err != nil {
		return err
	}
	if result.ExitCode != expected {
		return fmt.Errorf("expected exit code %d, got %d", expected, result.ExitCode)
	}
	return nil
}

func evaluateExitCodeInCondition(condition Condition, result *commandResult) error {
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

func resolvePath(workingDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workingDir, path)
}

func intFromValue(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	case float32:
		return int(typed), nil
	default:
		return 0, fmt.Errorf("value must be numeric")
	}
}

func errorAs(err error, target any) bool {
	return err != nil && execErrorAs(err, target)
}
