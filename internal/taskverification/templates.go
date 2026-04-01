package taskverification

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultCommandTimeout is the per-command timeout applied to template-defined shell commands.
const DefaultCommandTimeout = 30 * time.Second

var placeholderPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

// Service loads templates and manages the task verification runtime.
type Service struct {
	TemplateDir    string
	WorkingDir     string
	EventStore     EventStore
	CommandTimeout time.Duration
}

// NewService creates a task verification service rooted at the given directories.
func NewService(templateDir, workingDir string) *Service {
	return &Service{
		TemplateDir:    templateDir,
		WorkingDir:     workingDir,
		EventStore:     NewInMemoryEventStore(),
		CommandTimeout: DefaultCommandTimeout,
	}
}

// ListTemplates returns the task templates currently available on disk.
func (s *Service) ListTemplates() ([]TemplateSummary, error) {
	templates, err := s.loadTemplates()
	if err != nil {
		return nil, err
	}

	summaries := make([]TemplateSummary, 0, len(templates))
	for _, template := range templates {
		summaries = append(summaries, TemplateSummary{
			ID:           template.Task.ID,
			Name:         template.Task.Name,
			Description:  template.Task.Description,
			Instructions: template.Task.Instructions,
			Parameters:   template.ParameterDefinitions(),
			StepCount:    len(template.CompiledWorkflow.WorkflowSteps),
			Steps:        template.StepSummaries(),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ID < summaries[j].ID
	})
	return summaries, nil
}

// RegisterTask creates a shell task run from the selected template.
func (s *Service) RegisterTask(templateID string) (*RunState, error) {
	template, err := s.loadTemplateByID(templateID)
	if err != nil {
		return nil, err
	}

	return &RunState{
		RunID:            newTaskRunID(),
		TemplateID:       template.Task.ID,
		SelectedTemplate: *template,
		Status:           TaskStatusActive,
		Phase:            template.CompiledWorkflow.OnboardingPath,
		WorkflowReady:    false,
	}, nil
}

// CompleteOnboarding validates and persists onboarding context, then advances to planning.
func (s *Service) CompleteOnboarding(run *RunState, artifact *OnboardingArtifact) error {
	if err := validateOnboardingArtifact(artifact); err != nil {
		return err
	}
	if err := transitionTaskPhase(run, TaskPhasePlanning, TaskPhaseOnboarding); err != nil {
		return err
	}

	artifactCopy := cloneOnboardingArtifact(artifact)
	run.Onboarding = &artifactCopy
	run.LastFailureMessage = ""
	return nil
}

// CompletePlanning validates and freezes planning context, then enters execution.
func (s *Service) CompletePlanning(run *RunState, artifact *PlanningArtifact) error {
	if err := validatePlanningArtifact(&run.SelectedTemplate, artifact); err != nil {
		return err
	}
	artifactCopy := clonePlanningArtifact(artifact)
	resolved, stepStates, err := freezeRunnableContract(run, &artifactCopy)
	if err != nil {
		return err
	}
	planningNode, exists := resolved.CompiledWorkflow.Nodes[resolved.CompiledWorkflow.PlanningPath]
	if !exists {
		return fmt.Errorf("planning has no compiled workflow node")
	}
	nextPath := planningNode.NextPath
	if nextPath == "" {
		return fmt.Errorf("planning has no configured next workflow node")
	}
	if err := transitionTaskPhase(run, nextPath, run.SelectedTemplate.CompiledWorkflow.PlanningPath); err != nil {
		return err
	}

	run.Planning = &artifactCopy
	run.WorkflowReady = true
	run.RunnableTemplate = &resolved
	run.Steps = stepStates
	run.LastFailureMessage = ""
	return nil
}

// RestartTask resets an existing task run back to its onboarding shell state.
func (s *Service) RestartTask(run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}

	run.Status = TaskStatusActive
	run.Phase = run.SelectedTemplate.CompiledWorkflow.OnboardingPath
	run.Planning = nil
	run.WorkflowReady = false
	run.RunnableTemplate = nil
	run.Steps = nil
	run.LastFailureMessage = ""
	run.ExplicitFailReason = ""
	run.LastActivityAt = 0
	run.ExpiresAt = 0
	return nil
}

// FailTask marks a task run as failed without running additional checks.
func (s *Service) FailTask(run *RunState, reason string) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}

	run.Status = TaskStatusFailed
	run.ExplicitFailReason = strings.TrimSpace(reason)
	run.LastFailureMessage = run.ExplicitFailReason
	run.ExpiresAt = 0
	return nil
}

// TimeoutTask marks an active task run as timed out without changing its phase or steps.
func (s *Service) TimeoutTask(run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}
	if run.Status != TaskStatusActive {
		return fmt.Errorf("task is %s", run.Status)
	}

	run.Status = TaskStatusTimedOut
	return nil
}

// ResumeTask reactivates a timed-out task run without resetting its workflow progress.
func (s *Service) ResumeTask(run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}
	if run.Status != TaskStatusTimedOut {
		return fmt.Errorf("task is %s", run.Status)
	}

	run.Status = TaskStatusActive
	run.LastFailureMessage = ""
	run.ExpiresAt = 0
	return nil
}

func freezeRunnableContract(run *RunState, planning *PlanningArtifact) (Template, []StepState, error) {
	if run == nil {
		return Template{}, nil, fmt.Errorf("task is not registered")
	}
	if run.Status != TaskStatusActive {
		return Template{}, nil, fmt.Errorf("task is %s", run.Status)
	}
	if planning == nil {
		return Template{}, nil, fmt.Errorf("planning artifact is required")
	}

	resolved, err := run.SelectedTemplate.Resolve(planning.Parameters)
	if err != nil {
		return Template{}, nil, err
	}
	return resolved, newWorkflowStepStates(resolved.CompiledWorkflow), nil
}

func newWorkflowStepStates(compiled *CompiledWorkflow) []StepState {
	if compiled == nil || len(compiled.WorkflowSteps) == 0 {
		return nil
	}
	stepStates := make([]StepState, 0, len(compiled.WorkflowSteps))
	for idx := range compiled.WorkflowSteps {
		step := &compiled.WorkflowSteps[idx]
		stepStates = append(stepStates, StepState{
			ID:                 step.ID,
			Path:               step.Path,
			Status:             StepStatusPending,
			InvariantBaselines: make(map[string]string),
		})
	}
	return stepStates
}

func (s *Service) loadTemplateByID(templateID string) (*Template, error) {
	templates, err := s.loadTemplates()
	if err != nil {
		return nil, err
	}
	for _, template := range templates {
		if template.Task.ID == templateID {
			templateCopy := *template
			return &templateCopy, nil
		}
	}
	return nil, fmt.Errorf("task template %q not found", templateID)
}

func (s *Service) loadTemplates() ([]*Template, error) {
	entries, err := os.ReadDir(s.TemplateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read task template directory: %w", err)
	}

	templates := make([]*Template, 0)
	seenIDs := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(s.TemplateDir, entry.Name())
		template, err := loadTemplateFile(path)
		if err != nil {
			return nil, err
		}
		if previous, exists := seenIDs[template.Task.ID]; exists {
			return nil, fmt.Errorf("duplicate task template id %q in %s and %s", template.Task.ID, previous, path)
		}
		seenIDs[template.Task.ID] = path
		templates = append(templates, template)
	}

	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Task.ID < templates[j].Task.ID
	})
	return templates, nil
}

func loadTemplateFile(path string) (*Template, error) {
	// #nosec G304 -- templates are intentionally read from the configured template directory.
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read task template %s: %w", path, err)
	}

	var template Template
	if err := yaml.Unmarshal(content, &template); err != nil {
		return nil, fmt.Errorf("failed to parse task template %s: %w", path, err)
	}
	if err := template.Validate(); err != nil {
		return nil, fmt.Errorf("invalid task template %s: %w", path, err)
	}
	return &template, nil
}

// Validate checks whether the template is structurally valid before registration.
func (t *Template) Validate() error {
	return t.validate(true)
}

func (t *Template) validate(checkParameterCoverage bool) error {
	if t == nil {
		return fmt.Errorf("template is required")
	}
	if strings.TrimSpace(t.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if strings.TrimSpace(t.Task.ID) == "" {
		return fmt.Errorf("task.id is required")
	}
	if strings.TrimSpace(t.Task.Name) == "" {
		return fmt.Errorf("task.name is required")
	}
	if strings.TrimSpace(t.Task.Description) == "" {
		return fmt.Errorf("task.description is required")
	}

	if err := t.validateParameters(checkParameterCoverage); err != nil {
		return err
	}
	compiled, err := t.compileWorkflow()
	if err != nil {
		return err
	}
	t.CompiledWorkflow = compiled
	return nil
}

func (t *Template) validateParameters(checkCoverage bool) error {
	definedParams := make(map[string]struct{}, len(t.Parameters))
	for index, parameter := range t.Parameters {
		if strings.TrimSpace(parameter.Name) == "" {
			return fmt.Errorf("parameters[%d].name is required", index)
		}
		if _, exists := definedParams[parameter.Name]; exists {
			return fmt.Errorf("duplicate parameter name %q", parameter.Name)
		}
		definedParams[parameter.Name] = struct{}{}
	}
	if err := t.validatePlaceholderUsage(); err != nil {
		return err
	}

	if !checkCoverage {
		return nil
	}

	requiredParams := t.RequiredParameterNames()
	requiredParamSet := make(map[string]struct{}, len(requiredParams))
	for _, name := range requiredParams {
		requiredParamSet[name] = struct{}{}
	}
	for _, parameter := range t.Parameters {
		if _, exists := requiredParamSet[parameter.Name]; !exists {
			return fmt.Errorf("parameter %q is defined but not used by any placeholder", parameter.Name)
		}
	}
	for _, name := range requiredParams {
		if _, exists := definedParams[name]; len(t.Parameters) > 0 && !exists {
			return fmt.Errorf("parameter %q is used by a placeholder but missing from parameters", name)
		}
	}
	return nil
}

func (t *Template) validatePlaceholderUsage() error {
	if t == nil || t.Workflow == nil {
		return nil
	}
	checks := []struct {
		location string
		value    any
	}{
		{location: "task", value: t.Task},
		{location: "parameters", value: t.Parameters},
		{location: "workflow.onboarding", value: t.Workflow.Onboarding},
		{location: "workflow.planning", value: t.Workflow.Planning},
	}
	for _, check := range checks {
		if check.value == nil {
			continue
		}
		generic, err := genericValue(check.value)
		if err != nil {
			return err
		}
		if unresolved := findUnresolvedPlaceholder(generic); unresolved != "" {
			return fmt.Errorf("%s must not reference template parameter placeholder %q", check.location, unresolved)
		}
	}
	return nil
}

func validateStep(stepIndex int, step *Step, stepIDs map[string]struct{}) error {
	if strings.TrimSpace(step.ID) == "" {
		return fmt.Errorf("steps[%d].id is required", stepIndex)
	}
	if _, exists := stepIDs[step.ID]; exists {
		return fmt.Errorf("duplicate step id %q", step.ID)
	}
	stepIDs[step.ID] = struct{}{}

	if err := validateChecks(step); err != nil {
		return err
	}
	return validateInvariants(step)
}

func validateChecks(step *Step) error {
	checkIDs := make(map[string]struct{}, len(step.Checks))
	for checkIndex := range step.Checks {
		check := &step.Checks[checkIndex]
		if err := validateCheck(step.ID, checkIndex, check, checkIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateCheck(stepID string, checkIndex int, check *Check, checkIDs map[string]struct{}) error {
	if strings.TrimSpace(check.ID) == "" {
		return fmt.Errorf("step %q checks[%d].id is required", stepID, checkIndex)
	}
	if _, exists := checkIDs[check.ID]; exists {
		return fmt.Errorf("step %q has duplicate check id %q", stepID, check.ID)
	}
	checkIDs[check.ID] = struct{}{}
	if strings.TrimSpace(check.Command) == "" {
		return fmt.Errorf("step %q check %q command is required", stepID, check.ID)
	}
	if check.Description != "" && strings.TrimSpace(check.Description) == "" {
		return fmt.Errorf("step %q check %q description must not be blank", stepID, check.ID)
	}
	if err := validateConditions(stepID, check.ID, "pre_conditions", check.PreConditions); err != nil {
		return err
	}
	return validateConditions(stepID, check.ID, "post_conditions", check.PostConditions)
}

func validateInvariants(step *Step) error {
	invariantIDs := make(map[string]struct{}, len(step.Invariants))
	for invariantIndex, invariant := range step.Invariants {
		if strings.TrimSpace(invariant.ID) == "" {
			return fmt.Errorf("step %q invariants[%d].id is required", step.ID, invariantIndex)
		}
		if _, exists := invariantIDs[invariant.ID]; exists {
			return fmt.Errorf("step %q has duplicate invariant id %q", step.ID, invariant.ID)
		}
		invariantIDs[invariant.ID] = struct{}{}
		if strings.TrimSpace(invariant.Command) == "" {
			return fmt.Errorf("step %q invariant %q command is required", step.ID, invariant.ID)
		}
		if invariant.Description != "" && strings.TrimSpace(invariant.Description) == "" {
			return fmt.Errorf("step %q invariant %q description must not be blank", step.ID, invariant.ID)
		}
	}
	return nil
}

func validateConditions(stepID, checkID, phase string, conditions []Condition) error {
	for index, condition := range conditions {
		if err := validateCondition(condition); err != nil {
			return fmt.Errorf("step %q check %q %s[%d]: %w", stepID, checkID, phase, index, err)
		}
	}
	return nil
}

func validateCondition(condition Condition) error {
	if strings.TrimSpace(condition.Type) == "" {
		return fmt.Errorf("type is required")
	}
	handler, exists := conditionRegistry[condition.Type]
	if !exists {
		return fmt.Errorf("unsupported condition type %q", condition.Type)
	}
	return handler.validate(condition)
}

// RequiredParameterNames returns the placeholder names referenced by the template.
func (t *Template) RequiredParameterNames() []string {
	if t == nil || t.Workflow == nil {
		return nil
	}

	params := make(map[string]struct{})
	collectPlaceholdersFromValue(t.Workflow.Scaffolding, params)
	collectPlaceholdersFromValue(t.Workflow.Execution, params)

	result := make([]string, 0, len(params))
	for name := range params {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func collectPlaceholdersFromValue(value any, params map[string]struct{}) {
	generic, err := genericValue(value)
	if err != nil {
		return
	}
	collectPlaceholders(generic, params)
}

// ParameterDefinitions returns placeholder metadata in stable name order.
func (t *Template) ParameterDefinitions() []TemplateParameter {
	required := t.RequiredParameterNames()
	if len(required) == 0 {
		return nil
	}

	byName := make(map[string]TemplateParameter, len(t.Parameters))
	for _, parameter := range t.Parameters {
		byName[parameter.Name] = parameter
	}

	definitions := make([]TemplateParameter, 0, len(required))
	for _, name := range required {
		parameter, exists := byName[name]
		if !exists {
			parameter = TemplateParameter{Name: name}
		}
		definitions = append(definitions, parameter)
	}
	return definitions
}

// StepSummaries returns step metadata in template order.
func (t *Template) StepSummaries() []StepSummary {
	if t == nil || t.CompiledWorkflow == nil || len(t.CompiledWorkflow.WorkflowSteps) == 0 {
		return nil
	}

	summaries := make([]StepSummary, 0, len(t.CompiledWorkflow.WorkflowSteps))
	for index := range t.CompiledWorkflow.WorkflowSteps {
		step := &t.CompiledWorkflow.WorkflowSteps[index]
		summaries = append(summaries, StepSummary{
			Step:         index + 1,
			ID:           step.ID,
			Path:         step.Path,
			Name:         step.Name,
			Description:  step.Description,
			Instructions: step.Instructions,
		})
	}
	return summaries
}

// Resolve substitutes the provided parameter values into the template.
func (t *Template) Resolve(parameters map[string]string) (Template, error) {
	if t == nil {
		return Template{}, fmt.Errorf("template is required")
	}

	required := t.RequiredParameterNames()
	requiredSet := make(map[string]struct{}, len(required))
	for _, name := range required {
		requiredSet[name] = struct{}{}
		if _, exists := parameters[name]; !exists {
			return Template{}, fmt.Errorf("missing required task parameter %q", name)
		}
	}
	for name := range parameters {
		if _, exists := requiredSet[name]; !exists {
			return Template{}, fmt.Errorf("unknown task parameter %q", name)
		}
	}

	generic, err := genericValue(t)
	if err != nil {
		return Template{}, err
	}
	substituted := substitutePlaceholders(generic, parameters)
	if unresolved := findUnresolvedPlaceholder(substituted); unresolved != "" {
		return Template{}, fmt.Errorf("unresolved task parameter placeholder %q", unresolved)
	}

	content, err := json.Marshal(substituted)
	if err != nil {
		return Template{}, fmt.Errorf("failed to encode resolved task template: %w", err)
	}

	var resolved Template
	if err := json.Unmarshal(content, &resolved); err != nil {
		return Template{}, fmt.Errorf("failed to decode resolved task template: %w", err)
	}
	if err := resolved.validate(false); err != nil {
		return Template{}, fmt.Errorf("resolved task template is invalid: %w", err)
	}
	return resolved, nil
}

func genericValue(value any) (any, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize template: %w", err)
	}

	var generic any
	if err := json.Unmarshal(content, &generic); err != nil {
		return nil, fmt.Errorf("failed to deserialize template: %w", err)
	}
	return generic, nil
}

func collectPlaceholders(value any, params map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			collectPlaceholders(child, params)
		}
	case []any:
		for _, child := range typed {
			collectPlaceholders(child, params)
		}
	case string:
		matches := placeholderPattern.FindAllStringSubmatch(typed, -1)
		for _, match := range matches {
			params[match[1]] = struct{}{}
		}
	}
}

func substitutePlaceholders(value any, parameters map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = substitutePlaceholders(child, parameters)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, substitutePlaceholders(child, parameters))
		}
		return result
	case string:
		return placeholderPattern.ReplaceAllStringFunc(typed, func(match string) string {
			submatch := placeholderPattern.FindStringSubmatch(match)
			if len(submatch) != 2 {
				return match
			}
			value, exists := parameters[submatch[1]]
			if !exists {
				return match
			}
			return value
		})
	default:
		return value
	}
}

func findUnresolvedPlaceholder(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if unresolved := findUnresolvedPlaceholder(child); unresolved != "" {
				return unresolved
			}
		}
	case []any:
		for _, child := range typed {
			if unresolved := findUnresolvedPlaceholder(child); unresolved != "" {
				return unresolved
			}
		}
	case string:
		match := placeholderPattern.FindString(typed)
		if match != "" {
			return match
		}
	}
	return ""
}

func transitionTaskPhase(run *RunState, next TaskPhase, allowed ...TaskPhase) error {
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

	for _, phase := range allowed {
		if run.Phase == phase {
			run.Phase = next
			return nil
		}
	}
	return fmt.Errorf("task is in %s phase; cannot transition to %s", run.Phase, next)
}

func validateOnboardingArtifact(artifact *OnboardingArtifact) error {
	if artifact == nil {
		return fmt.Errorf("onboarding artifact is required")
	}
	if strings.TrimSpace(artifact.TaskSummary) == "" {
		return fmt.Errorf("onboarding.taskSummary is required")
	}
	for index, ref := range artifact.ArtifactMap {
		if strings.TrimSpace(ref.Path) == "" {
			return fmt.Errorf("onboarding.artifactMap[%d].path is required", index)
		}
		if strings.TrimSpace(ref.Kind) == "" {
			return fmt.Errorf("onboarding.artifactMap[%d].kind is required", index)
		}
	}
	for index, command := range artifact.CommonCommands {
		if strings.TrimSpace(command.Command) == "" {
			return fmt.Errorf("onboarding.commonCommands[%d].command is required", index)
		}
		if strings.TrimSpace(command.Purpose) == "" {
			return fmt.Errorf("onboarding.commonCommands[%d].purpose is required", index)
		}
	}
	return nil
}

func cloneOnboardingArtifact(artifact *OnboardingArtifact) OnboardingArtifact {
	if artifact == nil {
		return OnboardingArtifact{}
	}
	cloned := OnboardingArtifact{
		TaskSummary:   artifact.TaskSummary,
		Constraints:   append([]string(nil), artifact.Constraints...),
		OpenQuestions: append([]string(nil), artifact.OpenQuestions...),
	}
	if len(artifact.ArtifactMap) > 0 {
		cloned.ArtifactMap = make([]OnboardingArtifactRef, len(artifact.ArtifactMap))
		copy(cloned.ArtifactMap, artifact.ArtifactMap)
	}
	if len(artifact.CommonCommands) > 0 {
		cloned.CommonCommands = make([]OnboardingCommand, len(artifact.CommonCommands))
		copy(cloned.CommonCommands, artifact.CommonCommands)
	}
	return cloned
}

//nolint:gocyclo // Planning validation is kept as one entry point with small helpers for readability.
func validatePlanningArtifact(template *Template, artifact *PlanningArtifact) error {
	if artifact == nil {
		return fmt.Errorf("planning artifact is required")
	}
	if strings.TrimSpace(artifact.PlanSummary) == "" {
		return fmt.Errorf("planning.planSummary is required")
	}
	if err := validatePlanningParameters(template, artifact.Parameters); err != nil {
		return err
	}
	if err := validateUniqueTrimmedStrings("planning.selectedFiles", artifact.SelectedFiles); err != nil {
		return err
	}
	if err := validateUniqueTrimmedStrings("planning.invariants", artifact.Invariants); err != nil {
		return err
	}
	return nil
}

func validatePlanningParameters(template *Template, parameters map[string]string) error {
	if parameters == nil {
		parameters = map[string]string{}
	}
	defined := parameterNameSet(template)
	provided := make([]string, 0, len(parameters))
	unknown := make([]string, 0)
	for name := range parameters {
		provided = append(provided, name)
		if _, exists := defined[name]; !exists {
			unknown = append(unknown, name)
		}
	}
	missing := make([]string, 0)
	for _, name := range orderedPlanningInputs(template) {
		if strings.TrimSpace(parameters[name]) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 || len(unknown) > 0 {
		return newPlanningValidationError(orderedPlanningInputs(template), provided, missing, unknown)
	}
	return nil
}

func orderedPlanningInputs(template *Template) []string {
	if template == nil {
		return nil
	}
	names := make([]string, 0, len(template.Parameters))
	seen := make(map[string]struct{}, len(template.Parameters))
	for _, parameter := range template.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		return append([]string(nil), template.RequiredParameterNames()...)
	}
	sort.Strings(names)
	return names
}

func clonePlanningArtifact(artifact *PlanningArtifact) PlanningArtifact {
	if artifact == nil {
		return PlanningArtifact{}
	}
	return PlanningArtifact{
		PlanSummary:   artifact.PlanSummary,
		SelectedFiles: append([]string(nil), artifact.SelectedFiles...),
		Parameters:    cloneParameterMap(artifact.Parameters),
		Invariants:    append([]string(nil), artifact.Invariants...),
	}
}

func cloneParameterMap(parameters map[string]string) map[string]string {
	if len(parameters) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(parameters))
	for key, value := range parameters {
		cloned[key] = value
	}
	return cloned
}

func validateUniqueTrimmedStrings(field string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("%s[%d] is required", field, index)
		}
		if _, exists := seen[trimmed]; exists {
			return fmt.Errorf("%s contains duplicate value %q", field, trimmed)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}
