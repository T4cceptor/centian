package taskverification

// Template defines one task verification template loaded from YAML.
type Template struct {
	Version    string              `yaml:"version" json:"version"`
	Task       Task                `yaml:"task" json:"task"`
	Parameters []TemplateParameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Steps      []Step              `yaml:"steps" json:"steps"`
}

// Task describes the human-facing identity of a template.
type Task struct {
	ID           string `yaml:"id" json:"id"`
	Name         string `yaml:"name" json:"name"`
	Description  string `yaml:"description" json:"description"`
	Instructions string `yaml:"instructions,omitempty" json:"instructions,omitempty"`
}

// Step defines one fixed unit of progress in a task template.
type Step struct {
	ID           string      `yaml:"id" json:"id"`
	Name         string      `yaml:"name,omitempty" json:"name,omitempty"`
	Description  string      `yaml:"description,omitempty" json:"description,omitempty"`
	Instructions string      `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	Checks       []Check     `yaml:"checks" json:"checks"`
	Invariants   []Invariant `yaml:"invariants,omitempty" json:"invariants,omitempty"`
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

// StepSummary is the lightweight view of one template step returned to MCP clients.
type StepSummary struct {
	Step         int    `json:"step"`
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// TaskStatus enumerates the overall condition of a task run.
type TaskStatus string

// TaskStatus values represent the overall run condition.
const (
	TaskStatusActive    TaskStatus = "active"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// TaskPhase enumerates the workflow position of a task run.
type TaskPhase string

// TaskPhase values represent the current workflow phase.
const (
	TaskPhaseRegistered TaskPhase = "registered"
	TaskPhaseOnboarding TaskPhase = "onboarding"
	TaskPhasePlanning   TaskPhase = "planning"
	TaskPhaseExecution  TaskPhase = "execution"
)

// StepStatus enumerates the per-step lifecycle states.
type StepStatus string

// StepStatus values represent the lifecycle of one step within a task run.
const (
	StepStatusPending StepStatus = "pending"
	StepStatusActive  StepStatus = "active"
	StepStatusPassed  StepStatus = "passed"
	StepStatusFailed  StepStatus = "failed"
)

// StepState stores mutable runtime state for a single step.
type StepState struct {
	ID                 string            `json:"id"`
	Status             StepStatus        `json:"status"`
	InvariantBaselines map[string]string `json:"-"`
}

// RunState stores the mutable session-scoped state of one registered task.
type RunState struct {
	TemplateID         string            `json:"templateId"`
	SelectedTemplate   Template          `json:"-"`
	DraftParameters    map[string]string `json:"draftParameters"`
	Status             TaskStatus        `json:"status"`
	Phase              TaskPhase         `json:"phase"`
	ExecutionReady     bool              `json:"executionReady"`
	ExecutionTemplate  *Template         `json:"-"`
	Steps              []StepState       `json:"steps,omitempty"`
	LastFailureMessage string            `json:"lastFailureMessage,omitempty"`
	ExplicitFailReason string            `json:"explicitFailReason,omitempty"`
}

// StepResult is the MCP-facing outcome of starting or completing a step.
type StepResult struct {
	Passed     bool       `json:"passed"`
	Message    string     `json:"message"`
	Step       int        `json:"step"`
	StepID     string     `json:"stepId"`
	Status     TaskStatus `json:"status"`
	Phase      TaskPhase  `json:"phase"`
	StepStatus StepStatus `json:"stepStatus"`
}
