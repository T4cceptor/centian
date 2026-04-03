package taskverification

import "encoding/json"

// Template defines one task verification template loaded from YAML.
type Template struct {
	// Version is the template schema version declared in the YAML document.
	Version string `yaml:"version" json:"version"`
	// Task is the human-facing identity and description block for the template.
	Task Task `yaml:"task" json:"task"`
	// Parameters lists the agent-supplied placeholder values the template expects before planning can complete.
	Parameters []TemplateParameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	// Workflow is the declarative lifecycle definition authored in the template YAML.
	Workflow *Workflow `yaml:"workflow" json:"workflow"`
	// CompiledWorkflow is the normalized internal workflow graph derived from the authored template and is not serialized.
	CompiledWorkflow *CompiledWorkflow `yaml:"-" json:"-"`
}

// Task describes the human-facing identity of a template.
type Task struct {
	// ID is the stable template identifier used when registering a task.
	ID string `yaml:"id" json:"id"`
	// Name is the display name shown to agents and UIs.
	Name string `yaml:"name" json:"name"`
	// Description is the short summary of what the template is meant to accomplish.
	Description string `yaml:"description" json:"description"`
	// Instructions provides template-level guidance that agents should treat as the workflow source of truth.
	Instructions string `yaml:"instructions,omitempty" json:"instructions,omitempty"`
}

// Workflow describes the declarative task lifecycle.
type Workflow struct {
	// Onboarding configures the discovery phase that runs before planning.
	Onboarding *LifecycleNodeSpec `yaml:"onboarding" json:"onboarding"`
	// Planning configures the phase that freezes execution inputs before steps begin.
	Planning *PlanningNodeSpec `yaml:"planning" json:"planning"`
	// Scaffolding defines additive setup nodes that run before the main execution steps.
	Scaffolding []ExecutionNodeSpec `yaml:"scaffolding,omitempty" json:"scaffolding,omitempty"`
	// Execution defines the main executable workflow nodes that drive task completion.
	Execution []ExecutionNodeSpec `yaml:"execution" json:"execution"`
}

// LifecycleNodeSpec describes a non-execution workflow node.
type LifecycleNodeSpec struct {
	// Instructions tells the agent what to accomplish during this lifecycle phase.
	Instructions string `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	// AllowedTools is the tool allowlist pattern set active while this node is current.
	AllowedTools []string `yaml:"tools_allowed,omitempty" json:"toolsAllowed,omitempty"`
	// Checkpoint holds optional metadata about whether this node should be checkpointed later.
	Checkpoint *CheckpointHint `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
}

// PlanningNodeSpec describes the planning workflow node.
type PlanningNodeSpec struct {
	// Instructions tells the agent what planning contract it must produce before execution begins.
	Instructions string `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	// AllowedTools is the tool allowlist pattern set active during planning.
	AllowedTools []string `yaml:"tools_allowed,omitempty" json:"toolsAllowed,omitempty"`
	// Checkpoint holds optional metadata about whether planning should emit a checkpoint hint.
	Checkpoint *CheckpointHint `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
	// EditableFields lists which planning.parameters entries the planning step may revise.
	EditableFields []string `yaml:"editable_fields,omitempty" json:"editableFields,omitempty"`
	// RequiredInputs lists the planning inputs that must be present before execution can start.
	RequiredInputs []string `yaml:"required_inputs,omitempty" json:"requiredInputs,omitempty"`
	// Next optionally overrides the default next workflow path after planning completes.
	Next string `yaml:"next,omitempty" json:"next,omitempty"`
}

// ExecutionNodeSpec describes one author-facing execution or approval node.
type ExecutionNodeSpec struct {
	// ID is the template-authored node identifier used to build the workflow path.
	ID string `yaml:"id" json:"id"`
	// Kind optionally overrides whether this node is executable or a waiting-for-approval node.
	Kind WorkflowNodeKind `yaml:"kind,omitempty" json:"kind,omitempty"`
	// Name is the display label shown for this node in task summaries and UIs.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Description is a short human-facing summary of the node's purpose.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Instructions tells the agent what this node expects before it can be completed.
	Instructions string `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	// AllowedTools is the tool allowlist pattern set active while this node is current.
	AllowedTools []string `yaml:"tools_allowed,omitempty" json:"toolsAllowed,omitempty"`
	// Checkpoint holds optional metadata about whether this node should surface a checkpoint hint.
	Checkpoint *CheckpointHint `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
	// Checks defines the command-driven assertions Centian will evaluate for this node.
	Checks []Check `yaml:"checks,omitempty" json:"checks,omitempty"`
	// Invariants defines command outputs that must remain stable across the node's lifetime.
	Invariants []Invariant `yaml:"invariants,omitempty" json:"invariants,omitempty"`
	// Next optionally overrides the default next workflow path after this node passes.
	Next string `yaml:"next,omitempty" json:"next,omitempty"`
	// SubSteps nests additional execution nodes beneath this logical node in author-facing YAML.
	SubSteps []ExecutionNodeSpec `yaml:"sub_steps,omitempty" json:"subSteps,omitempty"`
}

// CheckpointHint stores declarative checkpoint metadata for later runtime use.
type CheckpointHint struct {
	// Enabled indicates whether this node should advertise checkpoint capability to clients.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// Step defines one compiled executable workflow node.
type Step struct {
	// ID is the author-defined identifier for this compiled executable node.
	ID string `json:"id"`
	// Path is the fully qualified workflow path for this step.
	Path TaskPhase `json:"path"`
	// ParentPath is the logical parent workflow path that contains this step.
	ParentPath TaskPhase `json:"parentPath,omitempty"`
	// NextPath is the workflow path Centian should enter after this step passes.
	NextPath TaskPhase `json:"nextPath,omitempty"`
	// Name is the display label exposed to agents and UIs for this step.
	Name string `json:"name,omitempty"`
	// Description is the short human-facing summary of this step.
	Description string `json:"description,omitempty"`
	// Instructions tells the agent what this step expects before completion.
	Instructions string `json:"instructions,omitempty"`
	// AllowedTools is the tool allowlist pattern set active while this step is current.
	AllowedTools []string `json:"allowedTools,omitempty"`
	// Checkpoint holds optional metadata about whether this step should surface a checkpoint hint.
	Checkpoint *CheckpointHint `json:"checkpoint,omitempty"`
	// Checks contains the command-driven assertions Centian evaluates for the step.
	Checks []Check `json:"checks"`
	// Invariants contains the command outputs that must remain stable while the step is active.
	Invariants []Invariant `json:"invariants,omitempty"`
}

// Check defines one command plus its pre/post conditions.
type Check struct {
	// ID is the author-defined identifier for this check.
	ID string `yaml:"id" json:"id"`
	// Description is the optional human-facing explanation of what this check enforces.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Command is the shell command Centian executes when evaluating the check.
	Command string `yaml:"command" json:"command"`
	// PreConditions are assertions that must pass before the step action is accepted.
	PreConditions []Condition `yaml:"pre_conditions,omitempty" json:"pre_conditions,omitempty"`
	// PostConditions are assertions that must pass after the step action completes.
	PostConditions []Condition `yaml:"post_conditions,omitempty" json:"post_conditions,omitempty"`
}

// Invariant defines one command whose stdout must remain stable across a step.
type Invariant struct {
	// ID is the author-defined identifier for this invariant.
	ID string `yaml:"id" json:"id"`
	// Description is the optional human-facing explanation of what this invariant protects.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Command is the shell command whose output Centian snapshots and compares for drift.
	Command string `yaml:"command" json:"command"`
}

// Condition defines one assertion evaluated against a command result or file.
type Condition struct {
	// Type selects the concrete condition evaluator such as exit code, file existence, or substring matching.
	Type string `yaml:"type" json:"type"`
	// Value is the primary operand for single-value conditions such as exit_code or file_contains.
	Value any `yaml:"value,omitempty" json:"value,omitempty"`
	// Values stores multi-value operands for condition types that compare against several values.
	Values []any `yaml:"values,omitempty" json:"values,omitempty"`
	// Path is the project-relative file path targeted by file-based conditions.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
}

// TemplateParameter describes one agent-provided parameter expected by a template.
type TemplateParameter struct {
	// Name is the placeholder identifier agents must fill in planning.parameters before planning completes.
	Name string `yaml:"name" json:"name"`
	// Description explains what value the agent should determine during onboarding and planning for this parameter.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// OnboardingArtifact stores reusable task/environment discovery context.
type OnboardingArtifact struct {
	// TaskSummary is the agent's concise description of the project and task context discovered during onboarding.
	TaskSummary string `json:"taskSummary"`
	// ArtifactMap records important project artifacts the agent identified during onboarding.
	ArtifactMap []OnboardingArtifactRef `json:"artifactMap,omitempty"`
	// CommonCommands records useful project commands the agent discovered during onboarding.
	CommonCommands []OnboardingCommand `json:"commonCommands,omitempty"`
	// Constraints captures project or workflow constraints the agent believes it must respect.
	Constraints []string `json:"constraints,omitempty"`
	// OpenQuestions captures unresolved uncertainties the agent wants to preserve before planning.
	OpenQuestions []string `json:"openQuestions,omitempty"`
}

// OnboardingArtifactRef describes a project artifact discovered during onboarding.
type OnboardingArtifactRef struct {
	// Path is the project-relative path to the discovered artifact.
	Path string `json:"path"`
	// Kind classifies the artifact, for example source, test, config, or docs.
	Kind string `json:"kind"`
	// Notes stores optional agent commentary about why the artifact matters.
	Notes string `json:"notes,omitempty"`
}

// OnboardingCommand describes a useful project command discovered during onboarding.
type OnboardingCommand struct {
	// Command is the concrete shell command the agent discovered.
	Command string `json:"command"`
	// Purpose explains why the command is useful in this project context.
	Purpose string `json:"purpose"`
}

// PlanningArtifact stores the frozen execution inputs produced during planning.
type PlanningArtifact struct {
	// PlanSummary captures the execution-defining planning summary that is frozen at planning completion.
	PlanSummary string `json:"planSummary,omitempty"`
	// SelectedFiles lists the project-relative files the agent expects to inspect or edit during execution.
	SelectedFiles []string `json:"selectedFiles,omitempty"`
	// Parameters stores the template placeholder values the agent freezes at planning completion.
	Parameters map[string]string `json:"parameters,omitempty"`
	// Invariants lists additional invariants the agent wants frozen as part of the planning contract.
	Invariants []string `json:"invariants,omitempty"`
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
	// Path is the fully qualified workflow path for this normalized node.
	Path TaskPhase `json:"path"`
	// Kind identifies whether the node is onboarding, planning, executable, or waiting for approval.
	Kind WorkflowNodeKind `json:"kind"`
	// ParentPath is the containing workflow path for nested nodes.
	ParentPath TaskPhase `json:"parentPath,omitempty"`
	// NextPath is the workflow path Centian should enter after this node succeeds.
	NextPath TaskPhase `json:"nextPath,omitempty"`
	// StepNumber is the 1-based execution step number when this node is executable.
	StepNumber int `json:"stepNumber,omitempty"`
	// StepID is the author-defined identifier for the executable node backing this workflow node.
	StepID string `json:"stepId,omitempty"`
	// Name is the display label exposed to agents and UIs for this node.
	Name string `json:"name,omitempty"`
	// Description is the short human-facing summary of this workflow node.
	Description string `json:"description,omitempty"`
	// Instructions tells the agent what this normalized node expects before completion.
	Instructions string `json:"instructions,omitempty"`
	// AllowedTools is the tool allowlist pattern set active while this node is current.
	AllowedTools []string `json:"allowedTools,omitempty"`
	// Checkpoint holds optional metadata about whether this node should advertise checkpoint capability.
	Checkpoint *CheckpointHint `json:"checkpoint,omitempty"`
	// EditableFields lists the planning.parameters fields planning may revise while this node is current.
	EditableFields []string `json:"editableFields,omitempty"`
	// RequiredPlanningInputs lists the planning inputs that must be present before leaving planning.
	RequiredPlanningInputs []string `json:"requiredPlanningInputs,omitempty"`
}

// CompiledWorkflow stores normalized workflow nodes derived from the template schema.
type CompiledWorkflow struct {
	// Nodes maps every compiled workflow path to its normalized node definition and is not serialized.
	Nodes map[TaskPhase]WorkflowNode `json:"-"`
	// OnboardingPath is the canonical workflow path for the onboarding node and is not serialized.
	OnboardingPath TaskPhase `json:"-"`
	// PlanningPath is the canonical workflow path for the planning node and is not serialized.
	PlanningPath TaskPhase `json:"-"`
	// FirstExecutablePath is the first compiled executable workflow path and is not serialized.
	FirstExecutablePath TaskPhase `json:"-"`
	// WorkflowSteps is the ordered list of compiled executable steps and is not serialized.
	WorkflowSteps []Step `json:"-"`
}

// TemplateSummary is the lightweight view returned to MCP clients.
type TemplateSummary struct {
	// ID is the stable template identifier exposed to MCP clients.
	ID string `json:"id"`
	// Name is the display name shown in task template listings.
	Name string `json:"name"`
	// Description is the short summary shown in task template listings.
	Description string `json:"description"`
	// Instructions is the template-level guidance agents can inspect before registration.
	Instructions string `json:"instructions,omitempty"`
	// Parameters lists the planning-time parameter contract for this template.
	Parameters []TemplateParameter `json:"parameters"`
	// StepCount is the number of compiled executable steps in the template.
	StepCount int `json:"stepCount"`
	// Steps is the lightweight execution-step summary exposed to MCP clients.
	Steps []StepSummary `json:"steps"`
}

// StepSummary is the lightweight view of one compiled execution step returned to MCP clients.
type StepSummary struct {
	// Step is the 1-based execution step number shown to the agent.
	Step int `json:"step"`
	// ID is the author-defined identifier for this compiled execution step.
	ID string `json:"id"`
	// Path is the fully qualified workflow path for the step.
	Path TaskPhase `json:"path"`
	// Name is the display label shown for the step in listings.
	Name string `json:"name,omitempty"`
	// Description is the short human-facing summary shown for the step.
	Description string `json:"description,omitempty"`
	// Instructions is the step guidance exposed in lightweight MCP summaries.
	Instructions string `json:"instructions,omitempty"`
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
	// TaskStatusTimedOut indicates a task run was paused due to inactivity.
	TaskStatusTimedOut TaskStatus = "timed_out"
)

// TaskPhase enumerates the workflow position of a task run.
type TaskPhase string

const (
	// TaskPhaseInitialization is the workflow path before registering a task.
	TaskPhaseInitialization TaskPhase = "initialization"
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
	// ID is the author-defined identifier for the compiled step this state belongs to.
	ID string `json:"id"`
	// Path is the fully qualified workflow path for the compiled step this state belongs to.
	Path TaskPhase `json:"path"`
	// Status is the mutable lifecycle status for this step within the current run.
	Status StepStatus `json:"status"`
	// InvariantBaselines stores captured invariant outputs for comparison and is intentionally not serialized.
	InvariantBaselines map[string]string `json:"-"`
}

// RunState stores the mutable session-scoped state of one registered task.
type RunState struct {
	// RunID is the stable identifier for this registered task run.
	RunID string `json:"taskRunId"`
	// TemplateID is the identifier of the template this run was registered from.
	TemplateID string `json:"templateId"`
	// SelectedTemplate is the original unresolved template chosen at registration time and is not serialized.
	SelectedTemplate Template `json:"-"`
	// Status is the overall lifecycle status of the task run.
	Status TaskStatus `json:"status"`
	// Phase is the current workflow path the run is positioned in.
	Phase TaskPhase `json:"phase"`
	// Onboarding stores the persisted onboarding artifact once onboarding completes.
	Onboarding *OnboardingArtifact `json:"onboarding,omitempty"`
	// Planning stores the frozen planning artifact once planning completes.
	Planning *PlanningArtifact `json:"planning,omitempty"`
	// WorkflowReady reports whether planning has completed and execution can proceed.
	WorkflowReady bool `json:"executionReady"`
	// RunnableTemplate holds the resolved execution-ready template for this run and is not serialized.
	RunnableTemplate *Template `json:"-"`
	// Steps stores the mutable per-step runtime state for the compiled workflow.
	Steps []StepState `json:"steps,omitempty"`
	// LastFailureMessage stores the most recent human-facing failure explanation for the run.
	LastFailureMessage string `json:"lastFailureMessage,omitempty"`
	// ExplicitFailReason stores the direct reason supplied when the task was explicitly failed.
	ExplicitFailReason string `json:"explicitFailReason,omitempty"`
	// LastActivityAt records the last observed task activity time in Unix milliseconds.
	LastActivityAt int64 `json:"lastActivityAtUnixMilli,omitempty"`
	// ExpiresAt records the inactivity timeout deadline in Unix milliseconds when applicable.
	ExpiresAt int64 `json:"expiresAtUnixMilli,omitempty"`
}

// StepResult is the MCP-facing outcome of starting or completing a step.
type StepResult struct {
	// Passed indicates whether the requested step action succeeded.
	Passed bool `json:"passed"`
	// Message is the primary human-facing explanation for the step outcome.
	Message string `json:"message"`
	// Step is the 1-based execution step number the result refers to.
	Step int `json:"step"`
	// StepID is the author-defined identifier for the step the result refers to.
	StepID string `json:"stepId"`
	// Status is the overall task run status after the step action completed.
	Status TaskStatus `json:"status"`
	// Phase is the current workflow path after the step action completed.
	Phase TaskPhase `json:"phase"`
	// StepStatus is the updated lifecycle status for the referenced step.
	StepStatus StepStatus `json:"stepStatus"`
	// FailureKind classifies the high-level reason the step action failed.
	FailureKind StepFailureKind `json:"failureKind,omitempty"`
	// FailurePhase identifies which verification stage produced the failure.
	FailurePhase StepFailurePhase `json:"failurePhase,omitempty"`
	// FailedCheckID identifies the failing check when a check caused the step to fail.
	FailedCheckID string `json:"failedCheckId,omitempty"`
	// FailedInvariantID identifies the failing invariant when invariant capture or verification failed.
	FailedInvariantID string `json:"failedInvariantId,omitempty"`
	// Summary is an optional concise summary of the step outcome suitable for UIs.
	Summary string `json:"summary,omitempty"`
	// ExitCode is the command exit code when command execution produced one.
	ExitCode *int `json:"exitCode,omitempty"`
	// StdoutSnippet is the truncated stdout excerpt relevant to the step outcome.
	StdoutSnippet string `json:"stdoutSnippet,omitempty"`
	// StderrSnippet is the truncated stderr excerpt relevant to the step outcome.
	StderrSnippet string `json:"stderrSnippet,omitempty"`
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
	// TaskEventTypeTimedOut records automatic inactivity timeout.
	TaskEventTypeTimedOut TaskEventType = "task_timed_out"
	// TaskEventTypeResumed records resuming a timed-out run in place.
	TaskEventTypeResumed TaskEventType = "task_resumed"
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
	// ID is the stable identifier for this persisted task lifecycle event.
	ID string `json:"id"`
	// SchemaVersion is the event schema version used when the event was written.
	SchemaVersion int `json:"schemaVersion"`
	// CreatedAtUnixMilli is the event creation time in Unix milliseconds.
	CreatedAtUnixMilli int64 `json:"createdAtUnixMilli"`
	// TaskRunID identifies which task run this lifecycle event belongs to.
	TaskRunID string `json:"taskRunId"`
	// SessionID records the upstream MCP session associated with the event when available.
	SessionID string `json:"sessionId,omitempty"`
	// TemplateID identifies the template backing the task run that emitted this event.
	TemplateID string `json:"templateId"`
	// PrincipalID records the authenticated principal associated with the event when available.
	PrincipalID string `json:"principalId,omitempty"`
	// ClientName is the MCP client name reported during session initialization.
	ClientName string `json:"clientName,omitempty"`
	// ClientVersion is the MCP client version reported during session initialization.
	ClientVersion string `json:"clientVersion,omitempty"`
	// PhasePath is the workflow path active when the lifecycle event was recorded.
	PhasePath TaskPhase `json:"phasePath"`
	// NodeKind is the semantic workflow node kind active when the event was recorded.
	NodeKind WorkflowNodeKind `json:"nodeKind,omitempty"`
	// ResultingPhasePath is the workflow path after the lifecycle action completed.
	ResultingPhasePath TaskPhase `json:"resultingPhasePath"`
	// ResultingNodeKind is the semantic workflow node kind after the lifecycle action completed.
	ResultingNodeKind WorkflowNodeKind `json:"resultingNodeKind,omitempty"`
	// EventType identifies which lifecycle operation produced this append-only record.
	EventType TaskEventType `json:"eventType"`
	// Outcome records whether the lifecycle operation succeeded or failed.
	Outcome TaskEventOutcome `json:"outcome"`
	// RelatedActionRequestID links this lifecycle event to a proxied action-event request when available.
	RelatedActionRequestID string `json:"relatedActionRequestId,omitempty"`
	// Payload stores event-specific opaque JSON data such as step failure details or submitted artifacts.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ActionEventTaskContext associates an action event with the active task snapshot.
type ActionEventTaskContext struct {
	// RequestID is the proxied action-event request identifier.
	RequestID string `json:"requestId"`
	// TaskRunID identifies the active task run associated with the action event.
	TaskRunID string `json:"taskRunId"`
	// InvocationPhasePath is the workflow path active when the action event was invoked.
	InvocationPhasePath TaskPhase `json:"invocationPhasePath"`
	// InvocationNodeKind is the semantic workflow node kind active when the action event was invoked.
	InvocationNodeKind WorkflowNodeKind `json:"invocationNodeKind,omitempty"`
	// CreatedAtUnixMilli is the timestamp in Unix milliseconds when the task context snapshot was recorded.
	CreatedAtUnixMilli int64 `json:"createdAtUnixMilli"`
}
