package taskverification

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
	"gopkg.in/yaml.v3"
)

var placeholderPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

// freezeRunnableContract resolves planning parameters into an executable template snapshot.
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

// newWorkflowStepStates allocates pending workflow state for each compiled execution step.
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

// loadTemplateFile reads one task template from disk and validates it.
func loadTemplateFile(path string) (*Template, error) {
	// #nosec G304 -- templates are intentionally read from the configured template directory.
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read task template %s: %w", path, err)
	}
	return loadTemplateContent(content, path)
}

// loadTemplateFSFile reads one embedded task template and validates it.
func loadTemplateFSFile(templateFS fs.FS, path, source string) (*Template, error) {
	content, err := fs.ReadFile(templateFS, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read task template %s: %w", source, err)
	}
	return loadTemplateContent(content, source)
}

// loadTemplateContent parses raw YAML and compiles the template workflow.
func loadTemplateContent(content []byte, source string) (*Template, error) {
	var template Template
	if err := yaml.Unmarshal(content, &template); err != nil {
		return nil, fmt.Errorf("failed to parse task template %s: %w", source, err)
	}
	if err := template.Validate(); err != nil {
		return nil, fmt.Errorf("invalid task template %s: %w", source, err)
	}
	return &template, nil
}

// registerTemplate inserts one template into the registry and lets filesystem overrides replace built-ins.
func registerTemplate(registry map[string]loadedTemplate, template *Template, source string, builtin bool) error {
	if template == nil {
		return fmt.Errorf("task template %s is nil", source)
	}

	existing, exists := registry[template.Task.ID]
	if exists {
		if existing.builtin && !builtin {
			registry[template.Task.ID] = loadedTemplate{
				template: template,
				source:   source,
				builtin:  false,
			}
			return nil
		}
		return fmt.Errorf("duplicate task template id %q in %s and %s", template.Task.ID, existing.source, source)
	}

	registry[template.Task.ID] = loadedTemplate{
		template: template,
		source:   source,
		builtin:  builtin,
	}
	return nil
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

// validateParameters checks parameter definitions and placeholder coverage rules.
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

// validatePlaceholderUsage forbids parameter placeholders in metadata and non-execution workflow sections.
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
		generic, err := common.JSONGenericValue(check.value)
		if err != nil {
			return fmt.Errorf("failed to convert %s into generic JSON: %w", check.location, err)
		}
		if unresolved := findUnresolvedPlaceholder(generic); unresolved != "" {
			return fmt.Errorf("%s must not reference template parameter placeholder %q", check.location, unresolved)
		}
	}
	return nil
}

// validateStep validates one execution step and enforces unique step ids.
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

// validateChecks validates all checks attached to one step.
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

// validateCheck validates one check definition inside a workflow step.
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

// validateInvariants validates invariant definitions declared on one step.
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

// validateConditions validates one list of conditions and annotates errors with step context.
func validateConditions(stepID, checkID, phase string, conditions []Condition) error {
	for index, condition := range conditions {
		if err := validateCondition(condition); err != nil {
			return fmt.Errorf("step %q check %q %s[%d]: %w", stepID, checkID, phase, index, err)
		}
	}
	return nil
}

// validateCondition validates one condition using the registered condition handler.
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

// collectPlaceholdersFromValue converts a typed value into generic JSON data before scanning it.
func collectPlaceholdersFromValue(value any, params map[string]struct{}) {
	generic, err := common.JSONGenericValue(value)
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

	generic, err := common.JSONGenericValue(t)
	if err != nil {
		return Template{}, fmt.Errorf("failed to convert task template into generic JSON: %w", err)
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

// collectPlaceholders walks a JSON-like value tree and records `${name}` placeholders from string leaves.
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

// substitutePlaceholders recursively replaces `${name}` placeholders in string leaves.
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

// findUnresolvedPlaceholder returns the first placeholder that remains after substitution.
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

// transitionTaskPhase applies lifecycle guards before moving the run into the next phase.
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

// validateOnboardingArtifact checks the required onboarding snapshot fields.
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

// cloneOnboardingArtifact copies onboarding data so callers can safely retain the original input.
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

// validatePlanningParameters ensures planning input names match the template contract.
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

// orderedPlanningInputs returns stable parameter names for planning validation errors and prompts.
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

// clonePlanningArtifact copies planning data so runtime state is isolated from caller mutation.
func clonePlanningArtifact(artifact *PlanningArtifact) PlanningArtifact {
	if artifact == nil {
		return PlanningArtifact{}
	}
	return PlanningArtifact{
		PlanSummary:   artifact.PlanSummary,
		SelectedFiles: append([]string(nil), artifact.SelectedFiles...),
		Parameters:    common.CloneStringMap(artifact.Parameters),
		Invariants:    append([]string(nil), artifact.Invariants...),
	}
}

// validateUniqueTrimmedStrings enforces non-blank, duplicate-free string lists for planning artifacts.
func validateUniqueTrimmedStrings(field string, values []string) error {
	return common.ValidateUniqueTrimmedStrings(field, values)
}
