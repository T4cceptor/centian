package taskverification

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
)

type commandResult struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
}

const outputSnippetLimit = 240
const (
	startStepRecoveryTool    = "centian.task_start_step"
	completeStepRecoveryTool = "centian.task_complete_step"
)

type stepFailureDetails struct {
	kind            StepFailureKind
	phase           StepFailurePhase
	failedCheckID   string
	failedCheckDesc string
	failedInvariant string
	summary         string
	exitCode        *int
	stdoutSnippet   string
	stderrSnippet   string
}

// StartStep validates the next step, runs its preconditions, and captures invariant baselines.
func (s *Service) StartStep(ctx context.Context, run *RunState, stepNumber int) (*StepResult, error) {
	stepIndex, step, err := workflowStepRequest(run, stepNumber)
	if err != nil {
		return nil, err
	}
	if handled, result, err := s.completePreviousStepIfNeeded(ctx, run, stepIndex); err != nil {
		return nil, err
	} else if handled {
		if persistErr := s.persistRunSnapshot(ctx, run); persistErr != nil {
			return result, persistErr
		}
		return result, err
	}
	if err := ensureStepCanStart(run, stepIndex); err != nil {
		return nil, err
	}

	if result, failed := s.runStepChecks(ctx, run, step, stepIndex, stepNumber, StepFailurePhasePrecondition, preConditionsForCheck); failed {
		attachRetryRecovery(result, startStepRecoveryTool)
		if persistErr := s.persistRunSnapshot(ctx, run); persistErr != nil {
			return result, persistErr
		}
		return result, nil
	}
	baselines, result, failed := s.captureInvariantBaselines(ctx, run, step, stepIndex, stepNumber)
	if failed {
		attachRetryRecovery(result, startStepRecoveryTool)
		if persistErr := s.persistRunSnapshot(ctx, run); persistErr != nil {
			return result, persistErr
		}
		return result, nil
	}

	result = startWorkflowStep(run, stepIndex, stepNumber, step.ID, baselines)
	if persistErr := s.persistRunSnapshot(ctx, run); persistErr != nil {
		return result, persistErr
	}
	return result, nil
}

// CompleteStep runs postconditions and invariant verification for an active step.
func (s *Service) CompleteStep(ctx context.Context, run *RunState, stepNumber int) (*StepResult, error) {
	stepIndex, _, err := workflowStepRequest(run, stepNumber)
	if err != nil {
		return nil, err
	}
	return s.completeWorkflowStep(ctx, run, stepIndex)
}

func (s *Service) completeWorkflowStep(ctx context.Context, run *RunState, stepIndex int) (*StepResult, error) {
	step, err := activeWorkflowStep(run, stepIndex)
	if err != nil {
		return nil, err
	}
	if run.Steps[stepIndex].Status != StepStatusActive {
		return nil, fmt.Errorf("step %d (%s) is not active", stepIndex+1, run.Steps[stepIndex].ID)
	}

	if result, failed := s.runStepChecks(ctx, run, step, stepIndex, stepIndex+1, StepFailurePhasePostcondition, postConditionsForCheck); failed {
		attachRetryRecovery(result, completeStepRecoveryTool)
		if persistErr := s.persistRunSnapshot(ctx, run); persistErr != nil {
			return result, persistErr
		}
		return result, nil
	}
	if result, failed := s.verifyInvariants(ctx, run, step, stepIndex); failed {
		attachRetryRecovery(result, completeStepRecoveryTool)
		if persistErr := s.persistRunSnapshot(ctx, run); persistErr != nil {
			return result, persistErr
		}
		return result, nil
	}

	result := completeWorkflowStep(run, stepIndex, step)
	if persistErr := s.persistRunSnapshot(ctx, run); persistErr != nil {
		return result, persistErr
	}
	return result, nil
}

func startWorkflowStep(run *RunState, stepIndex, stepNumber int, stepID string, baselines map[string]string) *StepResult {
	run.LastFailureMessage = ""
	run.Steps[stepIndex].Status = StepStatusActive
	run.Steps[stepIndex].InvariantBaselines = baselines
	return &StepResult{
		Passed:     true,
		Message:    fmt.Sprintf("step %d (%s) started", stepNumber, stepID),
		Summary:    fmt.Sprintf("step %d (%s) started", stepNumber, stepID),
		Step:       stepNumber,
		StepID:     stepID,
		Status:     run.Status,
		Phase:      run.Phase,
		StepStatus: run.Steps[stepIndex].Status,
	}
}

func completeWorkflowStep(run *RunState, stepIndex int, step *Step) *StepResult {
	run.LastFailureMessage = ""
	run.Steps[stepIndex].Status = StepStatusPassed
	run.Steps[stepIndex].InvariantBaselines = make(map[string]string)

	message := fmt.Sprintf("step %d (%s) completed", stepIndex+1, step.ID)
	if step.NextPath == "" {
		run.Status = TaskStatusCompleted
		run.ExpiresAt = 0
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
	}
}

func attachRetryRecovery(result *StepResult, tool string) {
	if result == nil || result.Passed {
		return
	}
	summary := fmt.Sprintf("Fix the failed check in workspaceRoot, then retry %s for step %d.", tool, result.Step)
	result.Retryable = true
	result.RestartRequired = false
	result.RecoveryActions = []RecoveryAction{{
		Kind:    "retry_tool",
		Summary: summary,
		Tool:    tool,
		Arguments: map[string]any{
			"step": result.Step,
		},
	}}
}

func workflowStepRequest(run *RunState, stepNumber int) (int, *Step, error) {
	if err := validateTaskExecutable(run); err != nil {
		return 0, nil, err
	}
	if run == nil {
		return 0, nil, fmt.Errorf("task is not registered")
	}
	if run.RunnableTemplate == nil {
		return 0, nil, fmt.Errorf("task has no execution contract")
	}
	if run.RunnableTemplate.CompiledWorkflow == nil {
		return 0, nil, fmt.Errorf("task has no compiled execution workflow")
	}
	if stepNumber < 1 || stepNumber > len(run.RunnableTemplate.CompiledWorkflow.WorkflowSteps) {
		return 0, nil, fmt.Errorf("step %d is out of range", stepNumber)
	}
	stepIndex := stepNumber - 1
	return stepIndex, workflowStep(run, stepIndex), nil
}

func activeWorkflowStep(run *RunState, stepIndex int) (*Step, error) {
	if err := validateTaskExecutable(run); err != nil {
		return nil, err
	}
	if stepIndex < 0 || stepIndex >= len(run.RunnableTemplate.CompiledWorkflow.WorkflowSteps) {
		return nil, fmt.Errorf("step %d is out of range", stepIndex+1)
	}
	step := workflowStep(run, stepIndex)
	if step.Path != run.Phase {
		return nil, fmt.Errorf("task is currently at %s; step %d (%s) is not the active workflow node", run.Phase, stepIndex+1, run.Steps[stepIndex].ID)
	}
	return step, nil
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
	case TaskStatusTimedOut:
		return fmt.Errorf("task is timed out; resume or restart the task")
	default:
		return fmt.Errorf("task is %s", run.Status)
	}
	node, exists := run.CurrentNode()
	if !exists {
		return fmt.Errorf("task is in unknown workflow phase %s", run.Phase)
	}
	if node.Kind != WorkflowNodeKindScaffolding && node.Kind != WorkflowNodeKindExecution {
		return fmt.Errorf("task is in %s phase; step execution is only allowed in scaffolding or execution nodes", run.Phase)
	}
	if !run.WorkflowReady || run.RunnableTemplate == nil {
		return fmt.Errorf("task has no execution contract")
	}
	return nil
}

func (s *Service) completePreviousStepIfNeeded(ctx context.Context, run *RunState, stepIndex int) (bool, *StepResult, error) {
	if stepIndex == 0 || run.Steps[stepIndex-1].Status != StepStatusActive {
		return false, nil, nil
	}
	result, err := s.completeWorkflowStep(ctx, run, stepIndex-1)
	if err != nil {
		return true, nil, err
	}
	if !result.Passed {
		return true, result, nil
	}
	return false, nil, nil
}

func (s *Service) runStepChecks(ctx context.Context, run *RunState, step *Step, stepIndex, stepNumber int, phase StepFailurePhase, conditions func(check *Check) []Condition) (*StepResult, bool) {
	for checkIndex := range step.Checks {
		check := &step.Checks[checkIndex]
		cmdCtx, cancel := context.WithTimeout(ctx, s.CommandTimeout)
		result, err := s.runCommand(cmdCtx, check.Command)
		cancel()
		if err != nil {
			return s.executionFailure(run, step, stepIndex, stepNumber, check, result, err), true
		}
		if err := evaluateConditions(conditions(check), result, s.WorkingDir); err != nil {
			details := failureDetailsFromCommand(
				StepFailureKindCheck,
				phase,
				result,
				fmt.Sprintf("step %d (%s) %s failed for check %s", stepNumber, step.ID, phase, check.ID),
			)
			details.failedCheckID = check.ID
			details.failedCheckDesc = strings.TrimSpace(check.Description)
			details.summary = fmt.Sprintf("%s: %v", details.summary, err)
			return failureResult(run, stepIndex, stepNumber, step.ID, &details), true
		}
	}
	return nil, false
}

func preConditionsForCheck(check *Check) []Condition {
	return check.PreConditions
}

func postConditionsForCheck(check *Check) []Condition {
	return check.PostConditions
}

func (s *Service) captureInvariantBaselines(ctx context.Context, run *RunState, step *Step, stepIndex, stepNumber int) (map[string]string, *StepResult, bool) {
	baselines := make(map[string]string, len(step.Invariants))
	for _, invariant := range step.Invariants {
		cmdCtx, cancel := context.WithTimeout(ctx, s.CommandTimeout)
		result, err := s.runCommand(cmdCtx, invariant.Command)
		cancel()
		if err != nil {
			return nil, s.invariantExecutionFailure(run, step, stepIndex, stepNumber, invariant.ID, StepFailurePhaseInvariantCapture, result, err), true
		}
		if result.ExitCode != 0 {
			details := failureDetailsFromCommand(
				StepFailureKindInvariant,
				StepFailurePhaseInvariantCapture,
				result,
				fmt.Sprintf("step %d (%s) invariant %s failed to capture baseline", stepNumber, step.ID, invariant.ID),
			)
			details.failedInvariant = invariant.ID
			details.summary = fmt.Sprintf("%s: expected exit code 0, got %d", details.summary, result.ExitCode)
			return nil, failureResult(run, stepIndex, stepNumber, step.ID, &details), true
		}
		baselines[invariant.ID] = result.Stdout
	}
	return baselines, nil, false
}

func (s *Service) verifyInvariants(ctx context.Context, run *RunState, step *Step, stepIndex int) (*StepResult, bool) {
	stepNumber := stepIndex + 1
	for _, invariant := range step.Invariants {
		cmdCtx, cancel := context.WithTimeout(ctx, s.CommandTimeout)
		result, err := s.runCommand(cmdCtx, invariant.Command)
		cancel()
		if err != nil {
			return s.invariantExecutionFailure(run, step, stepIndex, stepNumber, invariant.ID, StepFailurePhaseInvariantVerify, result, err), true
		}
		if failure := verifyInvariantResult(run, step, stepIndex, result, invariant); failure != nil {
			return failure, true
		}
	}
	return nil, false
}

func (s *Service) executionFailure(run *RunState, step *Step, stepIndex, stepNumber int, check *Check, result *commandResult, err error) *StepResult {
	checkID := ""
	checkDescription := ""
	if check != nil {
		checkID = check.ID
		checkDescription = strings.TrimSpace(check.Description)
	}
	details := failureDetailsFromCommand(
		StepFailureKindCommandExecution,
		StepFailurePhaseCommandExecution,
		result,
		fmt.Sprintf("step %d (%s) could not execute check %s", stepNumber, step.ID, checkID),
	)
	details.failedCheckID = checkID
	details.failedCheckDesc = checkDescription
	details.summary = fmt.Sprintf("%s: %v", details.summary, err)
	return failureResult(run, stepIndex, stepNumber, step.ID, &details)
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
	details := failureDetailsFromCommand(
		StepFailureKindCommandExecution,
		StepFailurePhaseCommandExecution,
		result,
		fmt.Sprintf("step %d (%s) could not %s invariant %s", stepNumber, step.ID, phaseActionLabel(phase), invariantID),
	)
	details.failedInvariant = invariantID
	details.summary = fmt.Sprintf("%s: %v", details.summary, err)
	return failureResult(run, stepIndex, stepNumber, step.ID, &details)
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
		return failureResult(run, stepIndex, stepNumber, step.ID, &details)
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
		return failureResult(run, stepIndex, stepNumber, step.ID, &details)
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
		return failureResult(run, stepIndex, stepNumber, step.ID, &details)
	}
	return nil
}

func failureResult(run *RunState, stepIndex, stepNumber int, stepID string, details *stepFailureDetails) *StepResult {
	if details == nil {
		details = &stepFailureDetails{}
	}
	message := details.summary
	run.LastFailureMessage = message
	result := &StepResult{
		Passed:                 false,
		Message:                message,
		Summary:                details.summary,
		Step:                   stepNumber,
		StepID:                 stepID,
		Status:                 run.Status,
		Phase:                  run.Phase,
		StepStatus:             run.Steps[stepIndex].Status,
		FailureKind:            details.kind,
		FailurePhase:           details.phase,
		FailedCheckID:          details.failedCheckID,
		FailedCheckDescription: details.failedCheckDesc,
		FailedInvariantID:      details.failedInvariant,
		ExitCode:               details.exitCode,
		StdoutSnippet:          details.stdoutSnippet,
		StderrSnippet:          details.stderrSnippet,
	}
	return result
}

func ensureStepCanStart(run *RunState, stepIndex int) error {
	if stepIndex < 0 || stepIndex >= len(run.Steps) {
		return fmt.Errorf("step %d is out of range", stepIndex+1)
	}
	if workflowStep(run, stepIndex).Path != run.Phase {
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

func workflowStep(run *RunState, stepIndex int) *Step {
	return &run.RunnableTemplate.CompiledWorkflow.WorkflowSteps[stepIndex]
}

func (s *Service) runCommand(ctx context.Context, command string) (*commandResult, error) {
	// #nosec G204 -- task verification intentionally executes template-defined commands.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	configureCommandCancellation(cmd)
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
	return common.TruncateString(trimmed, outputSnippetLimit)
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
		handler, exists := conditionRegistry[condition.Type]
		if !exists {
			return fmt.Errorf("unsupported condition type %q", condition.Type)
		}
		if err := handler.evaluate(condition, result, workingDir); err != nil {
			return err
		}
	}
	return nil
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
