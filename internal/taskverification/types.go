package taskverification

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
	Onboarding *LifecycleNodeSpec  `yaml:"onboarding" json:"onboarding"`
	Planning   *PlanningNodeSpec   `yaml:"planning" json:"planning"`
	Execution  []ExecutionNodeSpec `yaml:"execution" json:"execution"`
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

// OnboardingArtifact stores reusable project/environment discovery context.
type OnboardingArtifact struct {
	ProjectSummary string                  `json:"projectSummary"`
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
	WorkflowNodeKindOnboarding         WorkflowNodeKind = "onboarding"
	WorkflowNodeKindPlanning           WorkflowNodeKind = "planning"
	WorkflowNodeKindExecution          WorkflowNodeKind = "execution"
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
	ExecutionSteps      []Step                     `json:"-"`
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
	TaskStatusActive    TaskStatus = "active"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// TaskPhase enumerates the workflow position of a task run.
type TaskPhase string

const (
	TaskPhaseOnboarding         TaskPhase = "onboarding"
	TaskPhasePlanning           TaskPhase = "planning"
	TaskPhaseExecution          TaskPhase = "execution"
	TaskPhaseWaitingForApproval TaskPhase = "waiting_for_approval"
)

// StepStatus enumerates the per-step lifecycle states.
type StepStatus string

const (
	StepStatusPending StepStatus = "pending"
	StepStatusActive  StepStatus = "active"
	StepStatusPassed  StepStatus = "passed"
	StepStatusFailed  StepStatus = "failed"
)

// StepFailureKind classifies the high-level reason a step failed.
type StepFailureKind string

const (
	StepFailureKindCheck            StepFailureKind = "check"
	StepFailureKindInvariant        StepFailureKind = "invariant"
	StepFailureKindCommandExecution StepFailureKind = "command_execution"
)

// StepFailurePhase identifies which verification stage produced the failure.
type StepFailurePhase string

const (
	StepFailurePhasePrecondition     StepFailurePhase = "precondition"
	StepFailurePhasePostcondition    StepFailurePhase = "postcondition"
	StepFailurePhaseInvariantCapture StepFailurePhase = "invariant_capture"
	StepFailurePhaseInvariantVerify  StepFailurePhase = "invariant_verify"
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
	TemplateID         string              `json:"templateId"`
	SelectedTemplate   Template            `json:"-"`
	DraftParameters    map[string]string   `json:"draftParameters"`
	Status             TaskStatus          `json:"status"`
	Phase              TaskPhase           `json:"phase"`
	Onboarding         *OnboardingArtifact `json:"onboarding,omitempty"`
	Planning           *PlanningArtifact   `json:"planning,omitempty"`
	ExecutionReady     bool                `json:"executionReady"`
	ExecutionTemplate  *Template           `json:"-"`
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
