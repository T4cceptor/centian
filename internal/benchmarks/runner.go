package benchmarks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/persistence"
	"gopkg.in/yaml.v3"
)

const (
	defaultTimeout                 = 15 * time.Minute
	defaultSessionLabel            = "run"
	copySeedResetMode              = "copy_seed"
	sessionFileName                = "session.json"
	runFileName                    = "run.json"
	configuredSharedEventStoreMode = "configured_shared"
	runStatusCompleted             = "completed"
)

// TemplateVariant identifies one template tree used for a benchmark run variant.
type TemplateVariant struct {
	Name      string `json:"name"`
	SourceDir string `json:"sourceDir"`
}

// AgentModels contains optional per-agent model overrides.
type AgentModels struct {
	Claude string `json:"claude,omitempty"`
	Gemini string `json:"gemini,omitempty"`
	Codex  string `json:"codex,omitempty"`
}

// RunOptions configures one benchmark invocation.
type RunOptions struct {
	SuitePath         string
	CaseIDs           []string
	Agents            []string
	Repeat            int
	TemplateVariants  []TemplateVariant
	OutputRoot        string
	Timeout           time.Duration
	CentianBinaryPath string
	Models            AgentModels
	SessionLabel      string
	OnCentianReady    func(*RunManifest)
	AfterRun          func(*RunManifest) error
}

// SessionManifest describes one benchmark invocation and all concrete runs.
type SessionManifest struct {
	SuiteID          string                    `json:"suiteId"`
	TemplateID       string                    `json:"templateId"`
	SuitePath        string                    `json:"suitePath"`
	InvocationDir    string                    `json:"invocationDir"`
	OutputRoot       string                    `json:"outputRoot"`
	StartedAt        time.Time                 `json:"startedAt"`
	EndedAt          time.Time                 `json:"endedAt"`
	Status           string                    `json:"status"`
	Repeat           int                       `json:"repeat"`
	Agents           []string                  `json:"agents"`
	CaseIDs          []string                  `json:"caseIds"`
	TemplateVariants []TemplateVariant         `json:"templateVariants"`
	Runs             []SessionRunManifestEntry `json:"runs"`
}

// SessionRunManifestEntry summarizes one concrete run under a session.
type SessionRunManifestEntry struct {
	CaseID              string `json:"caseId"`
	AgentID             string `json:"agentId"`
	TemplateVariant     string `json:"templateVariant"`
	Attempt             int    `json:"attempt"`
	RelativeRunDir      string `json:"relativeRunDir"`
	Status              string `json:"status"`
	LatestTaskRunID     string `json:"latestTaskRunId,omitempty"`
	LatestTaskRunStatus string `json:"latestTaskRunStatus,omitempty"`
	ErrorSummary        string `json:"errorSummary,omitempty"`
}

// RunManifest describes one concrete benchmark run.
type RunManifest struct {
	SuiteID             string           `json:"suiteId"`
	CaseID              string           `json:"caseId"`
	TemplateID          string           `json:"templateId"`
	TemplateVariant     TemplateVariant  `json:"templateVariant"`
	AgentID             string           `json:"agentId"`
	Attempt             int              `json:"attempt"`
	SelectedModel       string           `json:"selectedModel,omitempty"`
	StartedAt           time.Time        `json:"startedAt"`
	EndedAt             time.Time        `json:"endedAt"`
	Status              string           `json:"status"`
	UIPublicURL         string           `json:"uiPublicUrl,omitempty"`
	CentianPID          int              `json:"centianPid,omitempty"`
	LatestTaskRunID     string           `json:"latestTaskRunId,omitempty"`
	LatestTaskRunStatus string           `json:"latestTaskRunStatus,omitempty"`
	LinkedTaskRunIDs    []string         `json:"linkedTaskRunIds,omitempty"`
	ArtifactPaths       RunArtifactPaths `json:"artifactPaths"`
	ErrorSummary        string           `json:"errorSummary,omitempty"`
}

// RunArtifactPaths lists the raw artifacts captured for one run.
type RunArtifactPaths struct {
	RunDir               string `json:"runDir"`
	ProjectDir           string `json:"projectDir"`
	LogsDir              string `json:"logsDir"`
	AgentDir             string `json:"agentDir"`
	ConfigPath           string `json:"configPath"`
	EventStoreMode       string `json:"eventStoreMode,omitempty"`
	EventStorePath       string `json:"eventStorePath"`
	RequestLogPath       string `json:"requestLogPath,omitempty"`
	SelectedTemplatePath string `json:"selectedTemplatePath,omitempty"`
}

// Runner executes benchmark suites locally.
type Runner struct {
	Now                  func() time.Time
	AllocatePort         func() (string, error)
	StartCentian         func(context.Context, StartCentianOptions) (*StartedCentian, error)
	LaunchAgent          func(context.Context, *agentrunner.RunOptions) (*agentrunner.RunResult, error)
	FetchTaskRuns        func(string) ([]persistence.TaskRunSummary, error)
	FetchTaskRunEvents   func(string, string) ([]persistence.TaskRunEvent, error)
	FindLatestRequestLog func(string) (string, error)
	PersistSession       func(context.Context, string, *persistence.BenchmarkSessionRecord) error
	PersistRun           func(context.Context, string, *persistence.BenchmarkRunRecord) error
}

// StartCentianOptions configures one benchmark-local Centian child process.
type StartCentianOptions struct {
	BinaryPath string
	ConfigPath string
	ProjectDir string
	LogsDir    string
	BaseURL    string
	MCPURL     string
}

// StartedCentian describes a running child Centian process.
type StartedCentian struct {
	PID  int
	Stop func() error
}

type runSpec struct {
	CaseRef         SuiteCaseRef
	CaseDef         *CaseDefinition
	CaseRoot        string
	Prompt          *PromptDefinition
	TemplateVariant TemplateVariant
	Agent           string
	Attempt         int
}

// NewRunner returns a benchmark runner with the default local execution hooks.
func NewRunner() *Runner {
	return &Runner{
		Now:                  time.Now,
		AllocatePort:         allocateFreePort,
		StartCentian:         startCentianProcess,
		LaunchAgent:          agentrunner.Run,
		FetchTaskRuns:        fetchTaskRuns,
		FetchTaskRunEvents:   fetchTaskRunEvents,
		FindLatestRequestLog: findLatestRequestLog,
		PersistSession:       persistBenchmarkSession,
		PersistRun:           persistBenchmarkRun,
	}
}

// ResolveDefaultTemplateVariants uses the repo's integrated templates as the implicit current variant.
func ResolveDefaultTemplateVariants(start string) ([]TemplateVariant, error) {
	repoRoot, err := FindRepoRoot(start)
	if err != nil {
		return nil, err
	}
	sourceDir := filepath.Join(repoRoot, "task-templates", "integrated")
	if _, err := os.Stat(sourceDir); err != nil {
		return nil, fmt.Errorf("default template dir %q is not available: %w", sourceDir, err)
	}
	return []TemplateVariant{{
		Name:      "current",
		SourceDir: sourceDir,
	}}, nil
}

// ResolveDefaultOutputRoot uses the taskverification tmp benchmark area under the repo root.
func ResolveDefaultOutputRoot(start string) (string, error) {
	repoRoot, err := FindRepoRoot(start)
	if err != nil {
		return "", err
	}
	return filepath.Join(repoRoot, "tests", "integrationtests", "taskverification", ".tmp", "benchmarks"), nil
}

// FindRepoRoot walks upward from start until it finds a repo root containing go.mod.
func FindRepoRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve start path %q: %w", start, err)
	}
	info, err := os.Stat(current)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("failed to locate repo root from %q", start)
		}
		current = parent
	}
}

// RunSuite executes the requested benchmark matrix and preserves raw artifacts for every run.
func (r *Runner) RunSuite(ctx context.Context, opts *RunOptions) (*SessionManifest, error) {
	r = r.withDefaults()

	if opts == nil {
		return nil, fmt.Errorf("run options are required")
	}
	if strings.TrimSpace(opts.SuitePath) == "" {
		return nil, fmt.Errorf("suite path is required")
	}
	if len(opts.Agents) == 0 {
		return nil, fmt.Errorf("at least one agent is required")
	}
	if opts.Repeat <= 0 {
		return nil, fmt.Errorf("repeat must be greater than zero")
	}
	if len(opts.TemplateVariants) == 0 {
		return nil, fmt.Errorf("at least one template variant is required")
	}
	if strings.TrimSpace(opts.OutputRoot) == "" {
		return nil, fmt.Errorf("output root is required")
	}
	if strings.TrimSpace(opts.CentianBinaryPath) == "" {
		return nil, fmt.Errorf("centian binary path is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}

	suiteRoot, err := filepath.Abs(opts.SuitePath)
	if err != nil {
		return nil, fmt.Errorf("resolve suite path: %w", err)
	}
	outputRoot, err := filepath.Abs(opts.OutputRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve output root: %w", err)
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create output root: %w", err)
	}

	suite, err := LoadSuite(suiteRoot)
	if err != nil {
		return nil, err
	}
	selectedRefs, err := selectCaseRefs(suite, opts.CaseIDs)
	if err != nil {
		return nil, err
	}
	templateVariants, err := normalizeTemplateVariants(opts.TemplateVariants)
	if err != nil {
		return nil, err
	}
	agents := normalizeList(opts.Agents)
	caseIDs := make([]string, 0, len(selectedRefs))
	for _, ref := range selectedRefs {
		caseIDs = append(caseIDs, ref.ID)
	}

	sessionDir, err := createSessionDir(outputRoot, suite.Suite.ID, opts.SessionLabel, r.Now())
	if err != nil {
		return nil, err
	}
	session := &SessionManifest{
		SuiteID:          suite.Suite.ID,
		TemplateID:       suite.Suite.TemplateID,
		SuitePath:        suiteRoot,
		InvocationDir:    sessionDir,
		OutputRoot:       outputRoot,
		StartedAt:        r.Now(),
		Status:           runStatusCompleted,
		Repeat:           opts.Repeat,
		Agents:           agents,
		CaseIDs:          caseIDs,
		TemplateVariants: templateVariants,
		Runs:             []SessionRunManifestEntry{},
	}

	specs, err := buildRunSpecs(suiteRoot, selectedRefs, templateVariants, agents, opts.Repeat)
	if err != nil {
		return nil, err
	}

	anyFailure := false
	for _, spec := range specs {
		manifest, runErr := r.executeRun(ctx, sessionDir, suite, spec, opts)
		entry := SessionRunManifestEntry{
			CaseID:              spec.CaseRef.ID,
			AgentID:             spec.Agent,
			TemplateVariant:     spec.TemplateVariant.Name,
			Attempt:             spec.Attempt,
			RelativeRunDir:      relativeRunDir(sessionDir, manifest.ArtifactPaths.RunDir),
			Status:              manifest.Status,
			LatestTaskRunID:     manifest.LatestTaskRunID,
			LatestTaskRunStatus: manifest.LatestTaskRunStatus,
			ErrorSummary:        manifest.ErrorSummary,
		}
		session.Runs = append(session.Runs, entry)
		if runErr != nil {
			anyFailure = true
		}
	}
	session.EndedAt = r.Now()
	if anyFailure {
		session.Status = "failed"
	}
	if err := writeJSONFile(filepath.Join(sessionDir, sessionFileName), session); err != nil {
		return nil, err
	}
	if record, err := buildSessionRecord(session); err != nil {
		return session, err
	} else {
		storePath, pathErr := config.ResolveEventStorePath(nil)
		if pathErr != nil {
			return session, pathErr
		}
		if err := r.PersistSession(ctx, storePath, record); err != nil {
			return session, err
		}
	}
	if anyFailure {
		return session, fmt.Errorf("one or more benchmark runs failed")
	}
	return session, nil
}

func (r *Runner) withDefaults() *Runner {
	if r == nil {
		return NewRunner()
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.AllocatePort == nil {
		r.AllocatePort = allocateFreePort
	}
	if r.StartCentian == nil {
		r.StartCentian = startCentianProcess
	}
	if r.LaunchAgent == nil {
		r.LaunchAgent = agentrunner.Run
	}
	if r.FetchTaskRuns == nil {
		r.FetchTaskRuns = fetchTaskRuns
	}
	if r.FetchTaskRunEvents == nil {
		r.FetchTaskRunEvents = fetchTaskRunEvents
	}
	if r.FindLatestRequestLog == nil {
		r.FindLatestRequestLog = findLatestRequestLog
	}
	if r.PersistSession == nil {
		r.PersistSession = persistBenchmarkSession
	}
	if r.PersistRun == nil {
		r.PersistRun = persistBenchmarkRun
	}
	return r
}

func (r *Runner) executeRun(
	ctx context.Context,
	sessionDir string,
	suite *SuiteDefinition,
	spec runSpec,
	opts *RunOptions,
) (*RunManifest, error) {
	runDir := filepath.Join(
		sessionDir,
		"runs",
		sanitizeName(spec.TemplateVariant.Name),
		sanitizeName(spec.Agent),
		sanitizeName(spec.CaseRef.ID),
		fmt.Sprintf("attempt-%03d", spec.Attempt),
	)
	projectDir := filepath.Join(runDir, "project")
	logsDir := filepath.Join(runDir, "logs")
	agentDir := filepath.Join(runDir, "agent")
	configPath := filepath.Join(runDir, "centian.config.json")
	selectedTemplatePath := filepath.Join(runDir, "selected-template.yaml")
	runtimeDir := filepath.Join(runDir, ".runtime")
	runtimeTemplatesDir := filepath.Join(runtimeDir, "templates")
	eventStorePath, err := config.ResolveEventStorePath(nil)
	if err != nil {
		manifest := &RunManifest{
			SuiteID:         suite.Suite.ID,
			CaseID:          spec.CaseRef.ID,
			TemplateID:      suite.Suite.TemplateID,
			TemplateVariant: spec.TemplateVariant,
			AgentID:         spec.Agent,
			Attempt:         spec.Attempt,
			SelectedModel:   selectedModel(spec.Agent, opts.Models),
			StartedAt:       r.Now(),
			EndedAt:         r.Now(),
			Status:          "failed",
			ErrorSummary:    err.Error(),
			ArtifactPaths: RunArtifactPaths{
				RunDir:               runDir,
				ProjectDir:           projectDir,
				LogsDir:              logsDir,
				AgentDir:             agentDir,
				ConfigPath:           configPath,
				SelectedTemplatePath: selectedTemplatePath,
			},
		}
		return manifest, err
	}
	manifest := &RunManifest{
		SuiteID:         suite.Suite.ID,
		CaseID:          spec.CaseRef.ID,
		TemplateID:      suite.Suite.TemplateID,
		TemplateVariant: spec.TemplateVariant,
		AgentID:         spec.Agent,
		Attempt:         spec.Attempt,
		SelectedModel:   selectedModel(spec.Agent, opts.Models),
		StartedAt:       r.Now(),
		Status:          "failed",
		ArtifactPaths: RunArtifactPaths{
			RunDir:               runDir,
			ProjectDir:           projectDir,
			LogsDir:              logsDir,
			AgentDir:             agentDir,
			ConfigPath:           configPath,
			EventStoreMode:       configuredSharedEventStoreMode,
			EventStorePath:       eventStorePath,
			SelectedTemplatePath: selectedTemplatePath,
		},
	}
	flushManifest := func() error {
		runPath := filepath.Join(runDir, runFileName)
		if err := writeJSONFile(runPath, manifest); err != nil {
			return err
		}
		if strings.TrimSpace(manifest.ArtifactPaths.EventStorePath) == "" {
			return nil
		}
		record, err := buildRunRecord(manifest)
		if err != nil {
			return err
		}
		return r.PersistRun(ctx, manifest.ArtifactPaths.EventStorePath, record)
	}

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		return manifest, err
	}
	for _, dir := range []string{logsDir, agentDir, runtimeTemplatesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			manifest.ErrorSummary = err.Error()
			manifest.EndedAt = r.Now()
			_ = flushManifest()
			return manifest, err
		}
	}

	fixtureRoot := filepath.Join(spec.CaseRoot, spec.CaseDef.Fixture.SeedPath)
	if spec.CaseDef.Fixture.ResetMode != copySeedResetMode {
		err := fmt.Errorf("unsupported fixture reset mode %q", spec.CaseDef.Fixture.ResetMode)
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	if err := copyDir(fixtureRoot, projectDir); err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	selectedTemplateSourcePath, err := resolveSelectedTemplateFile(spec.TemplateVariant.SourceDir, suite.Suite.TemplateID)
	if err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	if err := copyFile(selectedTemplateSourcePath, selectedTemplatePath); err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	if err := copyFile(selectedTemplateSourcePath, filepath.Join(runtimeTemplatesDir, filepath.Base(selectedTemplateSourcePath))); err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}

	port, err := r.AllocatePort()
	if err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	baseURL := "http://127.0.0.1:" + port
	mcpURL := baseURL + "/mcp/taskverification"
	if err := writeCentianConfig(configPath, runtimeTemplatesDir, projectDir, filepath.Join(logsDir, "internal.log"), eventStorePath, port); err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}

	started, err := r.StartCentian(ctx, StartCentianOptions{
		BinaryPath: opts.CentianBinaryPath,
		ConfigPath: configPath,
		ProjectDir: projectDir,
		LogsDir:    logsDir,
		BaseURL:    baseURL,
		MCPURL:     mcpURL,
	})
	if err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	manifest.UIPublicURL = baseURL + "/ui/tasks"
	manifest.CentianPID = started.PID
	if opts.OnCentianReady != nil {
		opts.OnCentianReady(manifest)
	}
	autoStop := opts.AfterRun == nil
	defer func() {
		if autoStop && started != nil && started.Stop != nil {
			_ = started.Stop()
		}
		_ = os.RemoveAll(runtimeDir)
	}()

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	baselineRuns, err := r.FetchTaskRuns(baseURL)
	if err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	_, agentErr := r.LaunchAgent(runCtx, &agentrunner.RunOptions{
		Agent:         spec.Agent,
		ArtifactRoot:  agentDir,
		WorkspacePath: projectDir,
		MCPURL:        mcpURL,
		Prompt:        strings.TrimSpace(spec.Prompt.Prompt),
		Timeout:       opts.Timeout,
		ClaudeModel:   opts.Models.Claude,
		GeminiModel:   opts.Models.Gemini,
		CodexModel:    opts.Models.Codex,
	})

	var captureErrs []string
	if agentErr != nil {
		captureErrs = append(captureErrs, agentErr.Error())
	}
	if err := r.captureRunArtifacts(manifest, logsDir, baseURL, taskRunIDSet(baselineRuns)); err != nil {
		captureErrs = append(captureErrs, err.Error())
	}
	if len(captureErrs) == 0 && manifest.LatestTaskRunStatus == runStatusCompleted {
		manifest.Status = runStatusCompleted
	} else if len(captureErrs) == 0 && manifest.LatestTaskRunStatus == "" {
		captureErrs = append(captureErrs, "no task runs were observed")
	}
	if len(captureErrs) > 0 {
		manifest.ErrorSummary = strings.Join(captureErrs, "; ")
	}
	if opts.AfterRun != nil {
		if err := opts.AfterRun(manifest); err != nil {
			if manifest.ErrorSummary == "" {
				manifest.ErrorSummary = err.Error()
			} else {
				manifest.ErrorSummary += "; " + err.Error()
			}
		}
	}

	manifest.EndedAt = r.Now()
	writeErr := flushManifest()
	if writeErr != nil {
		if manifest.ErrorSummary == "" {
			manifest.ErrorSummary = writeErr.Error()
		} else {
			manifest.ErrorSummary += "; " + writeErr.Error()
		}
		return manifest, writeErr
	}
	if manifest.Status != "completed" {
		return manifest, errors.New(manifest.ErrorSummary)
	}
	return manifest, nil
}

func (r *Runner) captureRunArtifacts(
	manifest *RunManifest,
	logsDir string,
	baseURL string,
	baselineRunIDs map[string]struct{},
) error {
	runs, err := r.FetchTaskRuns(baseURL)
	if err != nil {
		return err
	}
	runs = filterNewTaskRuns(runs, baselineRunIDs)
	manifest.LinkedTaskRunIDs = taskRunIDs(runs)

	if latest := latestTaskRun(runs); latest != nil {
		manifest.LatestTaskRunID = latest.RunID
		manifest.LatestTaskRunStatus = latest.Status
	}

	requestLogPath, err := r.FindLatestRequestLog(logsDir)
	if err != nil {
		return err
	}
	manifest.ArtifactPaths.RequestLogPath = requestLogPath
	return nil
}

func buildRunSpecs(
	suiteRoot string,
	caseRefs []SuiteCaseRef,
	templateVariants []TemplateVariant,
	agents []string,
	repeat int,
) ([]runSpec, error) {
	specs := make([]runSpec, 0, len(caseRefs)*len(templateVariants)*len(agents)*repeat)
	for _, ref := range caseRefs {
		caseRoot := filepath.Join(suiteRoot, ref.Path)
		caseDef, err := LoadCase(suiteRoot, ref)
		if err != nil {
			return nil, err
		}
		prompt, err := LoadPrompt(caseRoot, caseDef.PromptFile)
		if err != nil {
			return nil, err
		}
		for _, variant := range templateVariants {
			for _, agent := range agents {
				for attempt := 1; attempt <= repeat; attempt++ {
					specs = append(specs, runSpec{
						CaseRef:         ref,
						CaseDef:         caseDef,
						CaseRoot:        caseRoot,
						Prompt:          prompt,
						TemplateVariant: variant,
						Agent:           agent,
						Attempt:         attempt,
					})
				}
			}
		}
	}
	return specs, nil
}

func selectCaseRefs(suite *SuiteDefinition, caseIDs []string) ([]SuiteCaseRef, error) {
	if suite == nil {
		return nil, fmt.Errorf("suite definition is required")
	}
	if len(caseIDs) == 0 {
		return append([]SuiteCaseRef(nil), suite.Cases...), nil
	}
	selected := normalizeList(caseIDs)
	available := make(map[string]SuiteCaseRef, len(suite.Cases))
	for _, ref := range suite.Cases {
		available[ref.ID] = ref
	}
	refs := make([]SuiteCaseRef, 0, len(selected))
	for _, caseID := range selected {
		ref, ok := available[caseID]
		if !ok {
			return nil, fmt.Errorf("unknown case %q", caseID)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func normalizeTemplateVariants(variants []TemplateVariant) ([]TemplateVariant, error) {
	if len(variants) == 0 {
		return nil, fmt.Errorf("at least one template variant is required")
	}
	seen := map[string]struct{}{}
	normalized := make([]TemplateVariant, 0, len(variants))
	for _, variant := range variants {
		name := sanitizeName(variant.Name)
		if name == "" {
			return nil, fmt.Errorf("template variant name is required")
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate template variant %q", name)
		}
		sourceDir, err := filepath.Abs(strings.TrimSpace(variant.SourceDir))
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(sourceDir)
		if err != nil {
			return nil, fmt.Errorf("template variant %q source dir %q is not available: %w", name, sourceDir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("template variant %q source dir %q must be a directory", name, sourceDir)
		}
		normalized = append(normalized, TemplateVariant{
			Name:      name,
			SourceDir: sourceDir,
		})
		seen[name] = struct{}{}
	}
	return normalized, nil
}

func resolveSelectedTemplateFile(sourceDir, templateID string) (string, error) {
	if strings.TrimSpace(sourceDir) == "" {
		return "", fmt.Errorf("template source dir is required")
	}
	if strings.TrimSpace(templateID) == "" {
		return "", fmt.Errorf("template id is required")
	}
	var selectedPath string
	walkErr := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".yaml", ".yml":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var payload struct {
			Task struct {
				ID string `yaml:"id"`
			} `yaml:"task"`
		}
		if err := yaml.Unmarshal(data, &payload); err != nil {
			return err
		}
		if strings.TrimSpace(payload.Task.ID) != templateID {
			return nil
		}
		if selectedPath != "" {
			return fmt.Errorf("duplicate task template id %q in %s and %s", templateID, selectedPath, path)
		}
		selectedPath = path
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if selectedPath == "" {
		return "", fmt.Errorf("task template %q was not found under %q", templateID, sourceDir)
	}
	return selectedPath, nil
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			result = append(result, value)
			seen[value] = struct{}{}
		}
	}
	return result
}

func writeCentianConfig(configPath string, templatesDir string, projectDir string, internalLog string, eventStorePath string, port string) error {
	content, err := asset("centian_config.json")
	if err != nil {
		return err
	}
	resolved := strings.NewReplacer(
		"__PORT__", port,
		"__TEMPLATES_DIR__", templatesDir,
		"__PROJECT_DIR__", projectDir,
		"__INTERNAL_LOG__", internalLog,
		"__EVENT_STORE_PATH__", eventStorePath,
	).Replace(content)
	var payload any
	if err := json.Unmarshal([]byte(resolved), &payload); err != nil {
		return fmt.Errorf("parse rendered benchmark config: %w", err)
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode rendered benchmark config: %w", err)
	}
	if err := os.WriteFile(configPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write benchmark config: %w", err)
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func createSessionDir(outputRoot string, suiteID string, label string, now time.Time) (string, error) {
	if label = sanitizeName(label); label == "" {
		label = defaultSessionLabel
	}
	root := filepath.Join(outputRoot, suiteID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	baseName := now.Format("20060102150405") + "_" + label
	path := filepath.Join(root, baseName)
	for idx := 1; ; idx++ {
		err := os.Mkdir(path, 0o755)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		path = filepath.Join(root, fmt.Sprintf("%s_%02d", baseName, idx))
	}
}

func relativeRunDir(sessionDir string, runDir string) string {
	rel, err := filepath.Rel(sessionDir, runDir)
	if err != nil {
		return runDir
	}
	return rel
}

func selectedModel(agent string, models AgentModels) string {
	switch agent {
	case agentrunner.AgentClaude:
		return models.Claude
	case agentrunner.AgentGemini:
		return models.Gemini
	case agentrunner.AgentCodex:
		return models.Codex
	default:
		return ""
	}
}

func taskRunIDs(runs []persistence.TaskRunSummary) []string {
	result := make([]string, 0, len(runs))
	for _, run := range runs {
		result = append(result, run.RunID)
	}
	return result
}

func taskRunIDSet(runs []persistence.TaskRunSummary) map[string]struct{} {
	if len(runs) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		if strings.TrimSpace(run.RunID) == "" {
			continue
		}
		result[run.RunID] = struct{}{}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func filterNewTaskRuns(runs []persistence.TaskRunSummary, baseline map[string]struct{}) []persistence.TaskRunSummary {
	if len(baseline) == 0 {
		return runs
	}
	filtered := make([]persistence.TaskRunSummary, 0, len(runs))
	for _, run := range runs {
		if _, exists := baseline[run.RunID]; exists {
			continue
		}
		filtered = append(filtered, run)
	}
	return filtered
}

func latestTaskRun(runs []persistence.TaskRunSummary) *persistence.TaskRunSummary {
	if len(runs) == 0 {
		return nil
	}
	sorted := append([]persistence.TaskRunSummary(nil), runs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartedAt > sorted[j].StartedAt
	})
	return &sorted[0]
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func startCentianProcess(ctx context.Context, opts StartCentianOptions) (*StartedCentian, error) {
	stdoutPath := filepath.Join(opts.LogsDir, "centian.stdout.log")
	stderrPath := filepath.Join(opts.LogsDir, "centian.stderr.log")
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stdoutFile.Close() }()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		_ = stdoutFile.Close()
		return nil, err
	}
	defer func() { _ = stderrFile.Close() }()

	cmd := exec.CommandContext(ctx, opts.BinaryPath, "start", "--config-path", opts.ConfigPath)
	cmd.Dir = opts.ProjectDir
	cmd.Env = append(os.Environ(), "CENTIAN_LOG_DIR="+opts.LogsDir)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start centian: %w", err)
	}
	if err := waitForCentian(opts.BaseURL, opts.MCPURL); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}
	return &StartedCentian{
		PID: cmd.Process.Pid,
		Stop: func() error {
			if cmd.Process == nil {
				return nil
			}
			_ = cmd.Process.Signal(os.Interrupt)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				return err
			case <-time.After(2 * time.Second):
				_ = cmd.Process.Kill()
				return nil
			}
		},
	}, nil
}

func waitForCentian(baseURL string, mcpURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	apiURL := baseURL + "/api/task-runs"
	for time.Now().Before(deadline) {
		if isEndpointReachable(client, mcpURL) && isJSONEndpointReady(client, apiURL) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("centian did not become ready in time")
}

func isEndpointReachable(client *http.Client, endpoint string) bool {
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode < 500
}

func isJSONEndpointReady(client *http.Client, endpoint string) bool {
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func fetchTaskRuns(baseURL string) ([]persistence.TaskRunSummary, error) {
	resp, err := http.Get(baseURL + "/api/task-runs")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("task runs endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var runs []persistence.TaskRunSummary
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		return nil, err
	}
	return runs, nil
}

func fetchTaskRunEvents(baseURL string, runID string) ([]persistence.TaskRunEvent, error) {
	resp, err := http.Get(baseURL + "/api/task-runs/" + runID + "/events")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("task run events endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var events []persistence.TaskRunEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}
	return events, nil
}

func findLatestRequestLog(logDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(logDir, "requests_*.jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no request logs found in %s", logDir)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

func allocateFreePort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("allocate port: %w", err)
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", fmt.Errorf("split host port: %w", err)
	}
	return port, nil
}
