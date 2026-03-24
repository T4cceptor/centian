package taskverification

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var placeholderPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

// Service loads templates and manages the task verification runtime.
type Service struct {
	TemplateDir string
	WorkingDir  string
}

// NewService creates a task verification service rooted at the given directories.
func NewService(templateDir, workingDir string) *Service {
	return &Service{
		TemplateDir: templateDir,
		WorkingDir:  workingDir,
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
			StepCount:    len(template.Steps),
			Steps:        template.StepSummaries(),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ID < summaries[j].ID
	})
	return summaries, nil
}

// RegisterTask creates a shell task run from the selected template.
func (s *Service) RegisterTask(templateID string, parameters map[string]string) (*RunState, error) {
	template, err := s.loadTemplateByID(templateID)
	if err != nil {
		return nil, err
	}
	if err := validateDraftParameters(template, parameters); err != nil {
		return nil, err
	}

	return &RunState{
		TemplateID:       template.Task.ID,
		SelectedTemplate: *template,
		DraftParameters:  cloneParameters(parameters),
		Status:           TaskStatusActive,
		Phase:            TaskPhaseRegistered,
		ExecutionReady:   false,
	}, nil
}

// StartOnboarding enters onboarding or returns the existing onboarding phase.
func (s *Service) StartOnboarding(run *RunState) error {
	return transitionTaskPhase(run, TaskPhaseOnboarding, TaskPhaseRegistered, TaskPhaseOnboarding)
}

// CompleteOnboarding validates and persists onboarding context, then advances to planning.
func (s *Service) CompleteOnboarding(run *RunState, artifact OnboardingArtifact) error {
	if err := transitionTaskPhase(run, TaskPhasePlanning, TaskPhaseOnboarding); err != nil {
		return err
	}
	if err := validateOnboardingArtifact(artifact); err != nil {
		run.Phase = TaskPhaseOnboarding
		return err
	}

	artifactCopy := cloneOnboardingArtifact(artifact)
	run.Onboarding = &artifactCopy
	run.LastFailureMessage = ""
	return nil
}

// RestartTask resets an existing task run back to its registered shell state.
func (s *Service) RestartTask(run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}

	run.Status = TaskStatusActive
	run.Phase = TaskPhaseRegistered
	run.ExecutionReady = false
	run.ExecutionTemplate = nil
	run.Steps = nil
	run.LastFailureMessage = ""
	run.ExplicitFailReason = ""
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
	return nil
}

// PrepareExecution resolves the selected template using the draft parameters
// and initializes execution state for later step verification.
func (s *Service) PrepareExecution(run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}
	if run.Status != TaskStatusActive {
		return fmt.Errorf("task is %s", run.Status)
	}

	resolved, err := run.SelectedTemplate.Resolve(run.DraftParameters)
	if err != nil {
		return err
	}

	stepStates := make([]StepState, 0, len(resolved.Steps))
	for _, step := range resolved.Steps {
		stepStates = append(stepStates, StepState{
			ID:                 step.ID,
			Status:             StepStatusPending,
			InvariantBaselines: make(map[string]string),
		})
	}

	run.ExecutionReady = true
	run.ExecutionTemplate = &resolved
	run.Steps = stepStates
	run.Phase = TaskPhaseExecution
	run.LastFailureMessage = ""
	return nil
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
	if len(t.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	if err := t.validateParameters(checkParameterCoverage); err != nil {
		return err
	}
	return t.validateSteps()
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

func (t *Template) validateSteps() error {
	stepIDs := make(map[string]struct{}, len(t.Steps))
	for stepIndex := range t.Steps {
		step := &t.Steps[stepIndex]
		if err := validateStep(stepIndex, step, stepIDs); err != nil {
			return err
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
	if len(step.Checks) == 0 {
		return fmt.Errorf("step %q must define at least one check", step.ID)
	}

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
	switch condition.Type {
	case "exit_code":
		_, err := intFromValue(condition.Value)
		return err
	case "exit_code_in":
		return validateExitCodeInCondition(condition)
	case "stdout_contains", "stdout_not_contains":
		if _, ok := condition.Value.(string); !ok {
			return fmt.Errorf("value must be a string")
		}
		return nil
	case "file_exists":
		if strings.TrimSpace(condition.Path) == "" {
			return fmt.Errorf("path is required")
		}
		return nil
	case "file_contains":
		if strings.TrimSpace(condition.Path) == "" {
			return fmt.Errorf("path is required")
		}
		if _, ok := condition.Value.(string); !ok {
			return fmt.Errorf("value must be a string")
		}
		return nil
	default:
		return fmt.Errorf("unsupported condition type %q", condition.Type)
	}
}

func validateExitCodeInCondition(condition Condition) error {
	if len(condition.Values) == 0 {
		return fmt.Errorf("values are required")
	}
	for _, value := range condition.Values {
		if _, err := intFromValue(value); err != nil {
			return err
		}
	}
	return nil
}

// RequiredParameterNames returns the placeholder names referenced by the template.
func (t *Template) RequiredParameterNames() []string {
	if t == nil {
		return nil
	}

	generic, err := genericValue(t)
	if err != nil {
		return nil
	}

	params := make(map[string]struct{})
	collectPlaceholders(generic, params)

	result := make([]string, 0, len(params))
	for name := range params {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
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
	if t == nil || len(t.Steps) == 0 {
		return nil
	}

	summaries := make([]StepSummary, 0, len(t.Steps))
	for index, step := range t.Steps {
		summaries = append(summaries, StepSummary{
			Step:         index + 1,
			ID:           step.ID,
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

func cloneParameters(parameters map[string]string) map[string]string {
	if len(parameters) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(parameters))
	for key, value := range parameters {
		cloned[key] = value
	}
	return cloned
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

func validateDraftParameters(template *Template, parameters map[string]string) error {
	if template == nil {
		return fmt.Errorf("template is required")
	}

	defined := make(map[string]struct{}, len(template.ParameterDefinitions()))
	for _, parameter := range template.ParameterDefinitions() {
		defined[parameter.Name] = struct{}{}
	}
	for name := range parameters {
		if _, exists := defined[name]; !exists {
			return fmt.Errorf("unknown task parameter %q", name)
		}
	}
	return nil
}

func validateOnboardingArtifact(artifact OnboardingArtifact) error {
	if strings.TrimSpace(artifact.ProjectSummary) == "" {
		return fmt.Errorf("onboarding.projectSummary is required")
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

func cloneOnboardingArtifact(artifact OnboardingArtifact) OnboardingArtifact {
	cloned := OnboardingArtifact{
		ProjectSummary: artifact.ProjectSummary,
		Constraints:    append([]string(nil), artifact.Constraints...),
		OpenQuestions:  append([]string(nil), artifact.OpenQuestions...),
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
