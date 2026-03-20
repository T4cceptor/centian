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
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
}

// Step defines one fixed unit of progress in a task template.
type Step struct {
	ID          string      `yaml:"id" json:"id"`
	Name        string      `yaml:"name,omitempty" json:"name,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Checks      []Check     `yaml:"checks" json:"checks"`
	Invariants  []Invariant `yaml:"invariants,omitempty" json:"invariants,omitempty"`
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
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Parameters  []TemplateParameter `json:"parameters"`
	StepCount   int                 `json:"stepCount"`
}

// TaskStatus enumerates the task lifecycle states.
type TaskStatus string

// TaskStatus values represent the overall task lifecycle.
const (
	TaskStatusRegistered TaskStatus = "registered"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
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
	Parameters         map[string]string `json:"parameters"`
	ResolvedTemplate   Template          `json:"template"`
	Status             TaskStatus        `json:"status"`
	Steps              []StepState       `json:"steps"`
	LastFailureMessage string            `json:"lastFailureMessage,omitempty"`
	ExplicitFailReason string            `json:"explicitFailReason,omitempty"`
}

// StepResult is the MCP-facing outcome of starting or completing a step.
type StepResult struct {
	Passed     bool       `json:"passed"`
	Message    string     `json:"message"`
	Step       int        `json:"step"`
	StepID     string     `json:"stepId"`
	TaskStatus TaskStatus `json:"taskStatus"`
	StepStatus StepStatus `json:"stepStatus"`
}
