package taskruns

// PersistedRunSnapshot is the latest persisted snapshot of one task run.
type PersistedRunSnapshot struct {
	RunID                   string                       `json:"runId"`
	TemplateID              string                       `json:"templateId"`
	TemplateName            string                       `json:"templateName"`
	Status                  string                       `json:"status"`
	Phase                   string                       `json:"phase"`
	WorkflowReady           bool                         `json:"workflowReady"`
	LastFailureMessage      string                       `json:"lastFailureMessage,omitempty"`
	ExplicitFailReason      string                       `json:"explicitFailReason,omitempty"`
	LastActivityAtUnixMilli int64                        `json:"lastActivityAtUnixMilli,omitempty"`
	ExpiresAtUnixMilli      int64                        `json:"expiresAtUnixMilli,omitempty"`
	Onboarding              *PersistedOnboardingArtifact `json:"onboarding,omitempty"`
	Planning                *PersistedPlanningArtifact   `json:"planning,omitempty"`
	SelectedTemplate        PersistedTemplateSnapshot    `json:"selectedTemplate"`
	RunnableTemplate        *PersistedTemplateSnapshot   `json:"runnableTemplate,omitempty"`
	Steps                   []PersistedStepStateSnapshot `json:"steps,omitempty"`
}

type PersistedOnboardingArtifact struct {
	TaskSummary    string                           `json:"taskSummary"`
	ArtifactMap    []PersistedOnboardingArtifactRef `json:"artifactMap,omitempty"`
	CommonCommands []PersistedOnboardingCommand     `json:"commonCommands,omitempty"`
	Constraints    []string                         `json:"constraints,omitempty"`
	OpenQuestions  []string                         `json:"openQuestions,omitempty"`
}

type PersistedOnboardingArtifactRef struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Notes string `json:"notes,omitempty"`
}

type PersistedOnboardingCommand struct {
	Command string `json:"command"`
	Purpose string `json:"purpose"`
}

type PersistedPlanningArtifact struct {
	PlanSummary   string            `json:"planSummary,omitempty"`
	SelectedFiles []string          `json:"selectedFiles,omitempty"`
	Parameters    map[string]string `json:"parameters,omitempty"`
	Invariants    []string          `json:"invariants,omitempty"`
}

type PersistedTemplateSnapshot struct {
	Version          string                             `json:"version"`
	Task             PersistedTaskSnapshot              `json:"task"`
	Parameters       []PersistedTemplateParameter       `json:"parameters,omitempty"`
	Workflow         *PersistedWorkflowSnapshot         `json:"workflow"`
	CompiledWorkflow *PersistedCompiledWorkflowSnapshot `json:"compiledWorkflow,omitempty"`
}

type PersistedTaskSnapshot struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions,omitempty"`
}

type PersistedWorkflowSnapshot struct {
	Onboarding  *PersistedLifecycleNodeSpec  `json:"onboarding"`
	Planning    *PersistedPlanningNodeSpec   `json:"planning"`
	Scaffolding []PersistedExecutionNodeSpec `json:"scaffolding,omitempty"`
	Execution   []PersistedExecutionNodeSpec `json:"execution"`
}

type PersistedLifecycleNodeSpec struct {
	Instructions string                   `json:"instructions,omitempty"`
	AllowedTools []string                 `json:"toolsAllowed,omitempty"`
	Checkpoint   *PersistedCheckpointHint `json:"checkpoint,omitempty"`
}

type PersistedPlanningNodeSpec struct {
	Instructions   string                   `json:"instructions,omitempty"`
	AllowedTools   []string                 `json:"toolsAllowed,omitempty"`
	Checkpoint     *PersistedCheckpointHint `json:"checkpoint,omitempty"`
	EditableFields []string                 `json:"editableFields,omitempty"`
	RequiredInputs []string                 `json:"requiredInputs,omitempty"`
	Next           string                   `json:"next,omitempty"`
}

type PersistedExecutionNodeSpec struct {
	ID           string                       `json:"id"`
	Kind         string                       `json:"kind,omitempty"`
	Name         string                       `json:"name,omitempty"`
	Description  string                       `json:"description,omitempty"`
	Instructions string                       `json:"instructions,omitempty"`
	AllowedTools []string                     `json:"toolsAllowed,omitempty"`
	Checkpoint   *PersistedCheckpointHint     `json:"checkpoint,omitempty"`
	Checks       []PersistedCheck             `json:"checks,omitempty"`
	Invariants   []PersistedInvariant         `json:"invariants,omitempty"`
	Next         string                       `json:"next,omitempty"`
	SubSteps     []PersistedExecutionNodeSpec `json:"subSteps,omitempty"`
}

type PersistedCheckpointHint struct {
	Enabled bool `json:"enabled,omitempty"`
}

type PersistedCheck struct {
	ID             string               `json:"id"`
	Description    string               `json:"description,omitempty"`
	Command        string               `json:"command"`
	PreConditions  []PersistedCondition `json:"pre_conditions,omitempty"`
	PostConditions []PersistedCondition `json:"post_conditions,omitempty"`
}

type PersistedInvariant struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Command     string `json:"command"`
}

type PersistedCondition struct {
	Type   string `json:"type"`
	Value  any    `json:"value,omitempty"`
	Values []any  `json:"values,omitempty"`
	Path   string `json:"path,omitempty"`
}

type PersistedTemplateParameter struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type PersistedCompiledWorkflowSnapshot struct {
	Nodes               map[string]PersistedWorkflowNodeSnapshot `json:"nodes,omitempty"`
	OnboardingPath      string                                   `json:"onboardingPath,omitempty"`
	PlanningPath        string                                   `json:"planningPath,omitempty"`
	FirstExecutablePath string                                   `json:"firstExecutablePath,omitempty"`
	WorkflowSteps       []PersistedCompiledStepSnapshot          `json:"workflowSteps,omitempty"`
}

type PersistedWorkflowNodeSnapshot struct {
	Path                   string                   `json:"path"`
	Kind                   string                   `json:"kind"`
	ParentPath             string                   `json:"parentPath,omitempty"`
	NextPath               string                   `json:"nextPath,omitempty"`
	StepNumber             int                      `json:"stepNumber,omitempty"`
	StepID                 string                   `json:"stepId,omitempty"`
	Name                   string                   `json:"name,omitempty"`
	Description            string                   `json:"description,omitempty"`
	Instructions           string                   `json:"instructions,omitempty"`
	AllowedTools           []string                 `json:"allowedTools,omitempty"`
	Checkpoint             *PersistedCheckpointHint `json:"checkpoint,omitempty"`
	EditableFields         []string                 `json:"editableFields,omitempty"`
	RequiredPlanningInputs []string                 `json:"requiredPlanningInputs,omitempty"`
}

type PersistedCompiledStepSnapshot struct {
	ID           string                   `json:"id"`
	Path         string                   `json:"path"`
	ParentPath   string                   `json:"parentPath,omitempty"`
	NextPath     string                   `json:"nextPath,omitempty"`
	Name         string                   `json:"name,omitempty"`
	Description  string                   `json:"description,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	AllowedTools []string                 `json:"allowedTools,omitempty"`
	Checkpoint   *PersistedCheckpointHint `json:"checkpoint,omitempty"`
	Checks       []PersistedCheck         `json:"checks"`
	Invariants   []PersistedInvariant     `json:"invariants,omitempty"`
}

type PersistedStepStateSnapshot struct {
	ID                 string            `json:"id"`
	Path               string            `json:"path"`
	Status             string            `json:"status"`
	InvariantBaselines map[string]string `json:"invariantBaselines,omitempty"`
}
