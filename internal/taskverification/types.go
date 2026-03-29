package taskverification

import "encoding/json"

// Template defines one task verification template loaded from YAML.
type Template struct {
	Version          string              `yaml:"version" json:"version"`
	Task             Task                `yaml:"task" json:"task"`
	Parameters       []TemplateParameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Workflow         *Workflow           `yaml:"workflow" json:"workflow"`
	CompiledWorkflow *CompiledWorkflow   `yaml:"-" json:"-"`
}

// Task describes the human-facing identity of a template.
type Task struct {
	ID           string `yaml:"id" json:"id"`
	Name         string `yaml:"name" json:"name"`
	Description  string `yaml:"description" json:"description"`
	Instructions string `yaml:"instructions,omitempty" json:"instructions,omitempty"`
}

// Workflow describes the declarative task lifecycle.
type Workflow struct {
	Onboarding  *LifecycleNodeSpec  `yaml:"onboarding" json:"onboarding"`
	Planning    *PlanningNodeSpec   `yaml:"planning" json:"planning"`
	Scaffolding []ExecutionNodeSpec `yaml:"scaffolding,omitempty" json:"scaffolding,omitempty"`
	Execution   []ExecutionNodeSpec `yaml:"execution" json:"execution"`
}

// LifecycleNodeSpec describes a non-execution workflow node.
type LifecycleNodeSpec struct {
	Instructions string          `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	AllowedTools []string        `yaml:"tools_allowed,omitempty" json:"toolsAllowed,omitempty"`
	Checkpoint   *CheckpointHint `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
}

// PlanningNodeSpec describes the planning workflow node.
type PlanningNodeSpec struct {
	Instructions    string          `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	AllowedTools    []string        `yaml:"tools_allowed,omitempty" json:"toolsAllowed,omitempty"`
	Checkpoint      *CheckpointHint `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
	EditableFields  []string        `yaml:"editable_fields,omitempty" json:"editableFields,omitempty"`
	RequiredOutputs []string        `yaml:"required_outputs,omitempty" json:"requiredOutputs,omitempty"`
	Next            string          `yaml:"next,omitempty" json:"next,omitempty"`
}

// ExecutionNodeSpec describes one author-facing execution or approval node.
type ExecutionNodeSpec struct {
	ID           string              `yaml:"id" json:"id"`
	Kind         WorkflowNodeKind    `yaml:"kind,omitempty" json:"kind,omitempty"`
	Name         string              `yaml:"name,omitempty" json:"name,omitempty"`
	Description  string              `yaml:"description,omitempty" json:"description,omitempty"`
	Instructions string              `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	AllowedTools []string            `yaml:"tools_allowed,omitempty" json:"toolsAllowed,omitempty"`
	Checkpoint   *CheckpointHint     `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
	Checks       []Check             `yaml:"checks,omitempty" json:"checks,omitempty"`
	Invariants   []Invariant         `yaml:"invariants,omitempty" json:"invariants,omitempty"`
	Next         string              `yaml:"next,omitempty" json:"next,omitempty"`
	SubSteps     []ExecutionNodeSpec `yaml:"sub_steps,omitempty" json:"subSteps,omitempty"`
}

// CheckpointHint stores declarative checkpoint metadata for later runtime use.
type CheckpointHint struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// Step defines one compiled executable workflow node.
type Step struct {
	ID           string          `json:"id"`
	Path         TaskPhase       `json:"path"`
	ParentPath   TaskPhase       `json:"parentPath,omitempty"`
	NextPath     TaskPhase       `json:"nextPath,omitempty"`
	Name         string          `json:"name,omitempty"`
	Description  string          `json:"description,omitempty"`
	Instructions string          `json:"instructions,omitempty"`
	AllowedTools []string        `json:"allowedTools,omitempty"`
	Checkpoint   *CheckpointHint `json:"checkpoint,omitempty"`
	Checks       []Check         `json:"checks"`
	Invariants   []Invariant     `json:"invariants,omitempty"`
}

// Check defines one command plus its pre/post conditions.
type Check struct {
	ID             string      `yaml:"id" json:"id"`
	Command        string      `yaml:"command" json:"command"`
	PreConditions  []Condition `yaml:"pre_conditions,omitempty" json:"pre_conditions,omitempty"`
	PostConditions []Condition `yaml:"post_conditions,omitempty" json:"post_conditions,omitempty"`
}

// Invariant defines one command whose stdout must remain stable across a step.
type Invariant struct {
	ID      string `yaml:"id" json:"id"`
	Command string `yaml:"command" json:"command"`
}

// Condition defines one assertion evaluated against a command result or file.
type Condition struct {
	Type   string `yaml:"type" json:"type"`
	Value  any    `yaml:"value,omitempty" json:"value,omitempty"`
	Values []any  `yaml:"values,omitempty" json:"values,omitempty"`
	Path   string `yaml:"path,omitempty" json:"path,omitempty"`
}

// TemplateParameter describes one agent-provided parameter expected by a template.
type TemplateParameter struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// OnboardingArtifact stores reusable task/environment discovery context.
type OnboardingArtifact struct {
	TaskSummary    string                  `json:"taskSummary"`
	ArtifactMap    []OnboardingArtifactRef `json:"artifactMap,omitempty"`
	CommonCommands []OnboardingCommand     `json:"commonCommands,omitempty"`
	Constraints    []string                `json:"constraints,omitempty"`
	OpenQuestions  []string                `json:"openQuestions,omitempty"`
}

// OnboardingArtifactRef describes a project artifact discovered during onboarding.
type OnboardingArtifactRef struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Notes string `json:"notes,omitempty"`
}

// OnboardingCommand describes a useful project command discovered during onboarding.
type OnboardingCommand struct {
	Command string `json:"command"`
	Purpose string `json:"purpose"`
}

// PlanningArtifact stores the frozen execution inputs produced during planning.
type PlanningArtifact struct {
	SelectedFiles        []string `json:"selectedFiles,omitempty"`
	TestTarget           string   `json:"testTarget,omitempty"`
	LintCommand          string   `json:"lintCommand,omitempty"`
	ExpectedFailure      string   `json:"expectedFailure,omitempty"`
	ImplementationTarget string   `json:"implementationTarget,omitempty"`
	Invariants           []string `json:"invariants,omitempty"`
}

// WorkflowNodeKind identifies the semantic meaning of one compiled workflow node.
type WorkflowNodeKind string

const (
	// WorkflowNodeKindOnboarding marks the onboarding workflow node.
	WorkflowNodeKindOnboarding WorkflowNodeKind = "onboarding"
	// WorkflowNodeKindPlanning marks the planning workflow node.
	WorkflowNodeKindPlanning WorkflowNodeKind = "planning"
	// WorkflowNodeKindScaffolding marks an additive setup workflow node.
	WorkflowNodeKindScaffolding WorkflowNodeKind = "scaffolding"
	// WorkflowNodeKindExecution marks an executable workflow node.
	WorkflowNodeKindExecution WorkflowNodeKind = "execution"
	// WorkflowNodeKindWaitingForApproval marks a pause/approval workflow node.
	WorkflowNodeKindWaitingForApproval WorkflowNodeKind = "waiting_for_approval"
)

// WorkflowNode is the normalized runtime representation of one workflow node.
type WorkflowNode struct {
	Path                    TaskPhase        `json:"path"`
	Kind                    WorkflowNodeKind `json:"kind"`
	ParentPath              TaskPhase        `json:"parentPath,omitempty"`
	NextPath                TaskPhase        `json:"nextPath,omitempty"`
	StepNumber              int              `json:"stepNumber,omitempty"`
	StepID                  string           `json:"stepId,omitempty"`
	Name                    string           `json:"name,omitempty"`
	Description             string           `json:"description,omitempty"`
	Instructions            string           `json:"instructions,omitempty"`
	AllowedTools            []string         `json:"allowedTools,omitempty"`
	Checkpoint              *CheckpointHint  `json:"checkpoint,omitempty"`
	EditableFields          []string         `json:"editableFields,omitempty"`
	RequiredPlanningOutputs []string         `json:"requiredPlanningOutputs,omitempty"`
}

// CompiledWorkflow stores normalized workflow nodes derived from the template schema.
type CompiledWorkflow struct {
	Nodes               map[TaskPhase]WorkflowNode `json:"-"`
	OnboardingPath      TaskPhase                  `json:"-"`
	PlanningPath        TaskPhase                  `json:"-"`
	FirstExecutablePath TaskPhase                  `json:"-"`
	WorkflowSteps       []Step                     `json:"-"`
}

// TemplateSummary is the lightweight view returned to MCP clients.
type TemplateSummary struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Instructions string              `json:"instructions,omitempty"`
	Parameters   []TemplateParameter `json:"parameters"`
	StepCount    int                 `json:"stepCount"`
	Steps        []StepSummary       `json:"steps"`
}

// StepSummary is the lightweight view of one compiled execution step returned to MCP clients.
type StepSummary struct {
	Step         int       `json:"step"`
	ID           string    `json:"id"`
	Path         TaskPhase `json:"path"`
	Name         string    `json:"name,omitempty"`
	Description  string    `json:"description,omitempty"`
	Instructions string    `json:"instructions,omitempty"`
}

// TaskStatus enumerates the overall condition of a task run.
type TaskStatus string

const (
	// TaskStatusActive indicates a task run can continue executing.
	TaskStatusActive TaskStatus = "active"
	// TaskStatusCompleted indicates a task run finished successfully.
	TaskStatusCompleted TaskStatus = "completed"
	// TaskStatusFailed indicates a task run ended in failure.
	TaskStatusFailed TaskStatus = "failed"
)

// TaskPhase enumerates the workflow position of a task run.
type TaskPhase string

const (
	// TaskPhaseOnboarding is the onboarding workflow path.
	TaskPhaseOnboarding TaskPhase = "onboarding"
	// TaskPhasePlanning is the planning workflow path.
	TaskPhasePlanning TaskPhase = "planning"
	// TaskPhaseScaffolding is the reserved scaffolding root path.
	TaskPhaseScaffolding TaskPhase = "scaffolding"
	// TaskPhaseExecution is the reserved execution root path.
	TaskPhaseExecution TaskPhase = "execution"
	// TaskPhaseWaitingForApproval is the reserved approval root path.
	TaskPhaseWaitingForApproval TaskPhase = "waiting_for_approval"
)

// StepStatus enumerates the per-step lifecycle states.
type StepStatus string

const (
	// StepStatusPending indicates the step has not started.
	StepStatusPending StepStatus = "pending"
	// StepStatusActive indicates the step is currently in progress.
	StepStatusActive StepStatus = "active"
	// StepStatusPassed indicates the step completed successfully.
	StepStatusPassed StepStatus = "passed"
	// StepStatusFailed indicates the step failed verification.
	StepStatusFailed StepStatus = "failed"
)

// StepFailureKind classifies the high-level reason a step failed.
type StepFailureKind string

const (
	// StepFailureKindCheck indicates a failed check or condition.
	StepFailureKindCheck StepFailureKind = "check"
	// StepFailureKindInvariant indicates invariant capture or verification failed.
	StepFailureKindInvariant StepFailureKind = "invariant"
	// StepFailureKindCommandExecution indicates the command itself failed to execute as expected.
	StepFailureKindCommandExecution StepFailureKind = "command_execution"
)

// StepFailurePhase identifies which verification stage produced the failure.
type StepFailurePhase string

const (
	// StepFailurePhasePrecondition marks a precondition failure.
	StepFailurePhasePrecondition StepFailurePhase = "precondition"
	// StepFailurePhasePostcondition marks a postcondition failure.
	StepFailurePhasePostcondition StepFailurePhase = "postcondition"
	// StepFailurePhaseInvariantCapture marks invariant baseline capture failure.
	StepFailurePhaseInvariantCapture StepFailurePhase = "invariant_capture"
	// StepFailurePhaseInvariantVerify marks invariant drift detection failure.
	StepFailurePhaseInvariantVerify StepFailurePhase = "invariant_verify"
	// StepFailurePhaseCommandExecution marks an underlying command execution error.
	StepFailurePhaseCommandExecution StepFailurePhase = "command_execution"
)

// StepState stores mutable runtime state for a single step.
type StepState struct {
	ID                 string            `json:"id"`
	Path               TaskPhase         `json:"path"`
	Status             StepStatus        `json:"status"`
	InvariantBaselines map[string]string `json:"-"`
}

// RunState stores the mutable session-scoped state of one registered task.
type RunState struct {
	RunID              string              `json:"taskRunId"`
	TemplateID         string              `json:"templateId"`
	SelectedTemplate   Template            `json:"-"`
	DraftParameters    map[string]string   `json:"draftParameters"`
	Status             TaskStatus          `json:"status"`
	Phase              TaskPhase           `json:"phase"`
	Onboarding         *OnboardingArtifact `json:"onboarding,omitempty"`
	Planning           *PlanningArtifact   `json:"planning,omitempty"`
	WorkflowReady      bool                `json:"executionReady"`
	RunnableTemplate   *Template           `json:"-"`
	Steps              []StepState         `json:"steps,omitempty"`
	LastFailureMessage string              `json:"lastFailureMessage,omitempty"`
	ExplicitFailReason string              `json:"explicitFailReason,omitempty"`
}

// StepResult is the MCP-facing outcome of starting or completing a step.
type StepResult struct {
	Passed            bool             `json:"passed"`
	Message           string           `json:"message"`
	Step              int              `json:"step"`
	StepID            string           `json:"stepId"`
	Status            TaskStatus       `json:"status"`
	Phase             TaskPhase        `json:"phase"`
	StepStatus        StepStatus       `json:"stepStatus"`
	FailureKind       StepFailureKind  `json:"failureKind,omitempty"`
	FailurePhase      StepFailurePhase `json:"failurePhase,omitempty"`
	FailedCheckID     string           `json:"failedCheckId,omitempty"`
	FailedInvariantID string           `json:"failedInvariantId,omitempty"`
	Summary           string           `json:"summary,omitempty"`
	ExitCode          *int             `json:"exitCode,omitempty"`
	StdoutSnippet     string           `json:"stdoutSnippet,omitempty"`
	StderrSnippet     string           `json:"stderrSnippet,omitempty"`
}

// TaskEventType identifies one task lifecycle event.
type TaskEventType string

const (
	// TaskEventTypeRegistered records task registration.
	TaskEventTypeRegistered TaskEventType = "task_registered"
	// TaskEventTypeOnboardingCompleted records onboarding completion.
	TaskEventTypeOnboardingCompleted TaskEventType = "onboarding_completed"
	// TaskEventTypePlanningCompleted records planning completion.
	TaskEventTypePlanningCompleted TaskEventType = "planning_completed"
	// TaskEventTypeStepStarted records step start.
	TaskEventTypeStepStarted TaskEventType = "step_started"
	// TaskEventTypeStepCompleted records step completion.
	TaskEventTypeStepCompleted TaskEventType = "step_completed"
	// TaskEventTypeRestarted records task restart.
	TaskEventTypeRestarted TaskEventType = "task_restarted"
	// TaskEventTypeFailed records explicit task failure.
	TaskEventTypeFailed TaskEventType = "task_failed"
	// TaskEventTypeApprovalWaitEntered records entry into an approval wait node.
	TaskEventTypeApprovalWaitEntered TaskEventType = "approval_wait_entered"
)

// TaskEventOutcome captures whether a lifecycle operation succeeded.
type TaskEventOutcome string

const (
	// TaskEventOutcomeSucceeded indicates the lifecycle event succeeded.
	TaskEventOutcomeSucceeded TaskEventOutcome = "succeeded"
	// TaskEventOutcomeFailed indicates the lifecycle event failed.
	TaskEventOutcomeFailed TaskEventOutcome = "failed"
)

// TaskEvent is the append-only lifecycle record for one task run.
type TaskEvent struct {
	ID                     string           `json:"id"`
	SchemaVersion          int              `json:"schemaVersion"`
	CreatedAtUnixMilli     int64            `json:"createdAtUnixMilli"`
	TaskRunID              string           `json:"taskRunId"`
	SessionID              string           `json:"sessionId,omitempty"`
	TemplateID             string           `json:"templateId"`
	PrincipalID            string           `json:"principalId,omitempty"`
	PhasePath              TaskPhase        `json:"phasePath"`
	NodeKind               WorkflowNodeKind `json:"nodeKind,omitempty"`
	ResultingPhasePath     TaskPhase        `json:"resultingPhasePath"`
	ResultingNodeKind      WorkflowNodeKind `json:"resultingNodeKind,omitempty"`
	EventType              TaskEventType    `json:"eventType"`
	Outcome                TaskEventOutcome `json:"outcome"`
	RelatedActionRequestID string           `json:"relatedActionRequestId,omitempty"`
	Payload                json.RawMessage  `json:"payload,omitempty"`
}

// ActionEventTaskContext associates an action event with the active task snapshot.
type ActionEventTaskContext struct {
	RequestID           string           `json:"requestId"`
	TaskRunID           string           `json:"taskRunId"`
	InvocationPhasePath TaskPhase        `json:"invocationPhasePath"`
	InvocationNodeKind  WorkflowNodeKind `json:"invocationNodeKind,omitempty"`
	CreatedAtUnixMilli  int64            `json:"createdAtUnixMilli"`
}
