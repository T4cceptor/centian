package taskverification

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tasktemplates "github.com/T4cceptor/centian/task-templates"
)

// DefaultCommandTimeout is the per-command timeout applied to template-defined shell commands.
const DefaultCommandTimeout = 30 * time.Second

// Service loads templates and manages the task verification runtime.
type Service struct {
	TemplateDir    string
	WorkingDir     string
	EventStore     EventStore
	RunStore       RunStore
	CommandTimeout time.Duration
	builtinFS      fs.FS
}

// ServiceOptions configures non-runtime dependencies for the task verification service.
type ServiceOptions struct {
	BuiltinTemplates fs.FS
}

const builtinTemplatesDir = "integrated"

// NewService creates a task verification service rooted at the given directories.
func NewService(templateDir, workingDir string) *Service {
	return NewServiceWithOptions(templateDir, workingDir, ServiceOptions{
		BuiltinTemplates: tasktemplates.FS,
	})
}

// NewServiceWithOptions creates a task verification service with explicit loader configuration.
func NewServiceWithOptions(templateDir, workingDir string, options ServiceOptions) *Service {
	return &Service{
		TemplateDir:    templateDir,
		WorkingDir:     workingDir,
		EventStore:     NewInMemoryEventStore(),
		RunStore:       noopRunStore{},
		CommandTimeout: DefaultCommandTimeout,
		builtinFS:      options.BuiltinTemplates,
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
func (s *Service) RegisterTask(ctx context.Context, templateID string) (*RunState, error) {
	template, err := s.loadTemplateByID(templateID)
	if err != nil {
		return nil, err
	}

	run := &RunState{
		RunID:            newTaskRunID(),
		TemplateID:       template.Task.ID,
		SelectedTemplate: *template,
		Status:           TaskStatusActive,
		Phase:            template.CompiledWorkflow.OnboardingPath,
		WorkflowReady:    false,
	}
	if err := s.persistRunSnapshot(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// CompleteOnboarding validates and persists onboarding context, then advances to planning.
func (s *Service) CompleteOnboarding(ctx context.Context, run *RunState, artifact *OnboardingArtifact) error {
	if err := validateOnboardingArtifact(artifact); err != nil {
		return err
	}
	if err := transitionTaskPhase(run, TaskPhasePlanning, TaskPhaseOnboarding); err != nil {
		return err
	}

	artifactCopy := cloneOnboardingArtifact(artifact)
	run.Onboarding = &artifactCopy
	run.LastFailureMessage = ""
	return s.persistRunSnapshot(ctx, run)
}

// CompletePlanning validates and freezes planning context, then enters execution.
func (s *Service) CompletePlanning(ctx context.Context, run *RunState, artifact *PlanningArtifact) error {
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
	return s.persistRunSnapshot(ctx, run)
}

// RestartTask resets an existing task run back to its onboarding shell state.
func (s *Service) RestartTask(ctx context.Context, run *RunState) error {
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
	return s.persistRunSnapshot(ctx, run)
}

// FailTask marks a task run as failed without running additional checks.
func (s *Service) FailTask(ctx context.Context, run *RunState, reason string) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}

	run.Status = TaskStatusFailed
	run.ExplicitFailReason = strings.TrimSpace(reason)
	run.LastFailureMessage = run.ExplicitFailReason
	run.ExpiresAt = 0
	return s.persistRunSnapshot(ctx, run)
}

// TimeoutTask marks an active task run as timed out without changing its phase or steps.
func (s *Service) TimeoutTask(ctx context.Context, run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}
	if run.Status != TaskStatusActive {
		return fmt.Errorf("task is %s", run.Status)
	}

	run.Status = TaskStatusTimedOut
	return s.persistRunSnapshot(ctx, run)
}

// ResumeTask reactivates a timed-out task run without resetting its workflow progress.
func (s *Service) ResumeTask(ctx context.Context, run *RunState) error {
	if run == nil {
		return fmt.Errorf("task is not registered")
	}
	if run.Status != TaskStatusTimedOut {
		return fmt.Errorf("task is %s", run.Status)
	}

	run.Status = TaskStatusActive
	run.LastFailureMessage = ""
	run.ExpiresAt = 0
	return s.persistRunSnapshot(ctx, run)
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
	registry := make(map[string]loadedTemplate)

	if err := s.loadTemplatesFromFS(registry); err != nil {
		return nil, err
	}
	if err := s.loadTemplatesFromDir(registry); err != nil {
		return nil, err
	}

	templates := make([]*Template, 0, len(registry))
	for _, record := range registry {
		templates = append(templates, record.template)
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Task.ID < templates[j].Task.ID
	})
	return templates, nil
}

type loadedTemplate struct {
	template *Template
	source   string
	builtin  bool
}

func (s *Service) loadTemplatesFromFS(registry map[string]loadedTemplate) error {
	if s.builtinFS == nil {
		return nil
	}

	entries, err := fs.ReadDir(s.builtinFS, builtinTemplatesDir)
	if err != nil {
		return fmt.Errorf("failed to read built-in task templates: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(builtinTemplatesDir, entry.Name())
		source := "embedded:" + path
		template, err := loadTemplateFSFile(s.builtinFS, path, source)
		if err != nil {
			return err
		}
		if err := registerTemplate(registry, template, source, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) loadTemplatesFromDir(registry map[string]loadedTemplate) error {
	entries, err := os.ReadDir(s.TemplateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read task template directory: %w", err)
	}

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
			return err
		}
		if err := registerTemplate(registry, template, path, false); err != nil {
			return err
		}
	}
	return nil
}
