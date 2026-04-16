package benchmarks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/common"
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
	Claude             string `json:"claude,omitempty"`
	Gemini             string `json:"gemini,omitempty"`
	Codex              string `json:"codex,omitempty"`
	CodexOllamaProfile string `json:"codexOllamaProfile,omitempty"`
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
	CodexConfigPath   string
	CentianConfigPath string
	SessionLabel      string
	OnCentianReady    func(*RunManifest)
	AfterRun          func(*RunManifest) error
}

// SessionManifest describes one benchmark invocation and all concrete runs.
type SessionManifest struct {
	SuiteID          string                    `json:"suiteId"`
	SuiteName        string                    `json:"suiteName,omitempty"`
	TemplateID       string                    `json:"templateId"`
	TemplateName     string                    `json:"templateName,omitempty"`
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
	CaseName            string `json:"caseName,omitempty"`
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
	SuiteName           string           `json:"suiteName,omitempty"`
	CaseID              string           `json:"caseId"`
	CaseName            string           `json:"caseName,omitempty"`
	TemplateID          string           `json:"templateId"`
	TemplateName        string           `json:"templateName,omitempty"`
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
	PersistRunScore      func(context.Context, string, *persistence.BenchmarkRunScoreRecord) error
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

// runSpec is one expanded benchmark matrix cell ready for execution.
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
		AllocatePort:         common.AllocateFreePort,
		StartCentian:         startCentianProcess,
		LaunchAgent:          agentrunner.Run,
		FetchTaskRuns:        fetchTaskRuns,
		FetchTaskRunEvents:   fetchTaskRunEvents,
		FindLatestRequestLog: findLatestRequestLog,
		PersistSession:       persistBenchmarkSession,
		PersistRun:           persistBenchmarkRun,
		PersistRunScore:      persistBenchmarkRunScore,
	}
}

// ResolveDefaultTemplateVariants uses task-templates/integrated under the working
// directory as the implicit current variant, with a repo-root fallback for
// backwards compatibility.
func ResolveDefaultTemplateVariants(start string) ([]TemplateVariant, error) {
	searchRoot, err := filepath.Abs(start)
	if err != nil {
		return nil, fmt.Errorf("resolve template search root %q: %w", start, err)
	}
	info, err := os.Stat(searchRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		searchRoot = filepath.Dir(searchRoot)
	}
	candidates := []string{
		filepath.Join(searchRoot, "task-templates", "integrated"),
	}
	if repoRoot, rootErr := FindRepoRoot(searchRoot); rootErr == nil {
		candidates = append(candidates, filepath.Join(repoRoot, "task-templates", "integrated"))
	}
	var sourceDir string
	for _, candidate := range candidates {
		candidateInfo, candidateErr := os.Stat(candidate)
		if candidateErr != nil || !candidateInfo.IsDir() {
			continue
		}
		sourceDir = candidate
		break
	}
	if sourceDir == "" {
		return nil, fmt.Errorf("default template dir was not found; use --template-dir, --centian-config, or create task-templates/integrated under %q", searchRoot)
	}
	return []TemplateVariant{{
		Name:      "current",
		SourceDir: sourceDir,
	}}, nil
}

// ResolveTemplateVariantsFromCentianConfig uses a custom Centian config as the
// implicit current template variant when it declares a concrete templates path.
func ResolveTemplateVariantsFromCentianConfig(configPath string) ([]TemplateVariant, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return nil, nil
	}
	cfg, err := config.LoadConfigFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("load centian config %q: %w", configPath, err)
	}
	cfg.ResolveProjects()
	project := benchmarkDefaultProject(cfg)
	if project == nil {
		return nil, nil
	}
	templatesPath := project.TaskVerificationCapability().GetTemplatesPath()
	if templatesPath == "" || strings.Contains(templatesPath, "__TEMPLATES_DIR__") {
		return nil, nil
	}
	if !filepath.IsAbs(templatesPath) {
		templatesPath = filepath.Join(filepath.Dir(configPath), templatesPath)
	}
	info, err := os.Stat(templatesPath)
	if err != nil {
		return nil, fmt.Errorf("template dir from centian config %q is not available: %w", templatesPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("template dir from centian config %q must be a directory", templatesPath)
	}
	return []TemplateVariant{{
		Name:      "current",
		SourceDir: templatesPath,
	}}, nil
}

// ResolveDefaultOutputRoot uses a local .centian benchmark workspace under the
// working directory instead of the repo test fixture tree.
func ResolveDefaultOutputRoot(start string) (string, error) {
	root, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve benchmark output root base %q: %w", start, err)
	}
	info, err := os.Stat(root)
	if err == nil && !info.IsDir() {
		root = filepath.Dir(root)
	}
	return filepath.Join(root, ".centian", "benchmarks"), nil
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
	agents := common.NormalizeCSVList(opts.Agents)
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
		SuiteName:        strings.TrimSpace(suite.Suite.Name),
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
	sessionStorePath := ""
	for _, spec := range specs {
		manifest, runErr := r.executeRun(ctx, session, suite, spec, opts)
		entry := SessionRunManifestEntry{
			CaseID:              spec.CaseRef.ID,
			CaseName:            manifest.CaseName,
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
		session.TemplateName = firstNonEmpty(session.TemplateName, manifest.TemplateName)
		sessionStorePath = firstNonEmpty(sessionStorePath, manifest.ArtifactPaths.EventStorePath)
		if runErr != nil {
			anyFailure = true
		}
	}
	session.EndedAt = r.Now()
	if anyFailure {
		session.Status = "failed"
	}
	if err := common.WriteJSONFile(filepath.Join(sessionDir, sessionFileName), session); err != nil {
		return nil, err
	}
	if record, err := buildSessionRecord(session); err != nil {
		return session, err
	} else {
		storePath := strings.TrimSpace(sessionStorePath)
		if storePath == "" {
			var pathErr error
			storePath, pathErr = config.ResolveEventStorePath(nil)
			if pathErr != nil {
				return session, pathErr
			}
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

// withDefaults fills nil runner hooks with the default local implementations.
func (r *Runner) withDefaults() *Runner {
	if r == nil {
		return NewRunner()
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.AllocatePort == nil {
		r.AllocatePort = common.AllocateFreePort
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
	if r.PersistRunScore == nil {
		r.PersistRunScore = persistBenchmarkRunScore
	}
	return r
}

// executeRun executes one benchmark matrix cell and persists its raw artifacts and score snapshot.
func (r *Runner) executeRun(
	ctx context.Context,
	session *SessionManifest,
	suite *SuiteDefinition,
	spec runSpec,
	opts *RunOptions,
) (*RunManifest, error) {
	// TODO: refactor to reduce length
	if session == nil {
		return nil, fmt.Errorf("session manifest is required")
	}
	sessionDir := strings.TrimSpace(session.InvocationDir)
	runDir := filepath.Join(
		sessionDir,
		"runs",
		benchmarkRunDirName(spec),
	)
	projectDir := filepath.Join(runDir, "project")
	logsDir := filepath.Join(runDir, "logs")
	agentDir := filepath.Join(runDir, "agent")
	configPath := filepath.Join(runDir, "centian.config.json")
	selectedTemplatePath := filepath.Join(runDir, "selected-template.yaml")
	runtimeDir := filepath.Join(runDir, ".runtime")
	runtimeTemplatesDir := filepath.Join(runtimeDir, "templates")
	defaultEventStorePath, err := config.ResolveEventStorePath(nil)
	if err != nil {
		manifest := &RunManifest{
			SuiteID:         suite.Suite.ID,
			SuiteName:       strings.TrimSpace(suite.Suite.Name),
			CaseID:          spec.CaseRef.ID,
			CaseName:        strings.TrimSpace(spec.CaseDef.Case.Name),
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
		SuiteName:       strings.TrimSpace(suite.Suite.Name),
		CaseID:          spec.CaseRef.ID,
		CaseName:        strings.TrimSpace(spec.CaseDef.Case.Name),
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
			EventStorePath:       defaultEventStorePath,
			SelectedTemplatePath: selectedTemplatePath,
		},
	}
	flushManifest := func() error {
		runPath := filepath.Join(runDir, runFileName)
		if err := common.WriteJSONFile(runPath, manifest); err != nil {
			return err
		}
		if strings.TrimSpace(manifest.ArtifactPaths.EventStorePath) == "" {
			return nil
		}
		record, err := buildRunRecord(manifest)
		if err != nil {
			return err
		}
		if err := r.PersistRun(ctx, manifest.ArtifactPaths.EventStorePath, record); err != nil {
			return err
		}
		if manifest.EndedAt.IsZero() || r.PersistRunScore == nil {
			return nil
		}
		sessionRecord, err := buildSessionRecord(session)
		if err != nil {
			return err
		}
		scoreRecord, err := buildPersistedRunScoreRecord(ctx, manifest.ArtifactPaths.EventStorePath, sessionRecord, record, r.Now)
		if err != nil {
			return err
		}
		return r.PersistRunScore(ctx, manifest.ArtifactPaths.EventStorePath, scoreRecord)
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
	if err := common.CopyDir(fixtureRoot, projectDir); err != nil {
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
	if err := common.CopyFile(selectedTemplateSourcePath, selectedTemplatePath); err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	manifest.TemplateName = firstNonEmpty(readTemplateName(selectedTemplatePath), manifest.TemplateName)
	if err := common.CopyFile(selectedTemplateSourcePath, filepath.Join(runtimeTemplatesDir, filepath.Base(selectedTemplateSourcePath))); err != nil {
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
	renderedConfig, err := renderCentianConfig(opts.CentianConfigPath, runtimeTemplatesDir, projectDir, filepath.Join(logsDir, "internal.log"), defaultEventStorePath, port)
	if err != nil {
		manifest.ErrorSummary = err.Error()
		manifest.EndedAt = r.Now()
		_ = flushManifest()
		return manifest, err
	}
	manifest.ArtifactPaths.EventStorePath = renderedConfig.EffectiveEventStorePath
	if err := os.WriteFile(configPath, renderedConfig.Content, 0o644); err != nil {
		err = fmt.Errorf("write benchmark config: %w", err)
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
	runResult, agentErr := r.LaunchAgent(runCtx, &agentrunner.RunOptions{
		Agent:              spec.Agent,
		ArtifactRoot:       agentDir,
		WorkspacePath:      projectDir,
		MCPURL:             mcpURL,
		Prompt:             strings.TrimSpace(spec.Prompt.Prompt),
		Timeout:            opts.Timeout,
		ClaudeModel:        opts.Models.Claude,
		GeminiModel:        opts.Models.Gemini,
		CodexModel:         opts.Models.Codex,
		CodexOllamaProfile: opts.Models.CodexOllamaProfile,
		CodexConfigPath:    opts.CodexConfigPath,
	})
	if runResult != nil && strings.TrimSpace(runResult.SelectedModel) != "" {
		manifest.SelectedModel = runResult.SelectedModel
	}

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

// captureRunArtifacts records the request log and newly created task runs for one benchmark run.
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

// buildRunSpecs expands cases, template variants, agents, and attempts into executable runs.
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

// selectCaseRefs keeps suite order while validating the requested case IDs.
func selectCaseRefs(suite *SuiteDefinition, caseIDs []string) ([]SuiteCaseRef, error) {
	if suite == nil {
		return nil, fmt.Errorf("suite definition is required")
	}
	if len(caseIDs) == 0 {
		return append([]SuiteCaseRef(nil), suite.Cases...), nil
	}
	selected := common.NormalizeCSVList(caseIDs)
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

// normalizeTemplateVariants validates names, deduplicates variants, and resolves source dirs.
func normalizeTemplateVariants(variants []TemplateVariant) ([]TemplateVariant, error) {
	if len(variants) == 0 {
		return nil, fmt.Errorf("at least one template variant is required")
	}
	seen := map[string]struct{}{}
	normalized := make([]TemplateVariant, 0, len(variants))
	for _, variant := range variants {
		name := common.NormalizeSlug(variant.Name)
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

// readTemplateName extracts task.name from a template file when available.
func readTemplateName(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	var payload struct {
		Task struct {
			Name string `yaml:"name"`
		} `yaml:"task"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Task.Name)
}

// resolveSelectedTemplateFile finds the one template file whose task.id matches templateID.
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

// normalizeList splits comma-separated values, trims blanks, and preserves first-seen order.
// benchmarkRunDirName builds the stable on-disk directory name for one run.
func benchmarkRunDirName(spec runSpec) string {
	parts := []string{
		common.NormalizeSlug(spec.TemplateVariant.Name),
		common.NormalizeSlug(spec.Agent),
		common.NormalizeSlug(spec.CaseRef.ID),
		fmt.Sprintf("attempt_%03d", spec.Attempt),
	}
	return strings.Join(parts, "_")
}

// renderedCentianConfig contains the final per-run Centian config and resolved store path.
type renderedCentianConfig struct {
	Content                 []byte
	EffectiveEventStorePath string
}

// renderCentianConfig materializes the run-local Centian config and resolves effective paths.
func renderCentianConfig(baseConfigPath string, templatesDir string, projectDir string, internalLog string, defaultEventStorePath string, port string) (*renderedCentianConfig, error) {
	content, err := loadBenchmarkCentianConfigTemplate(baseConfigPath)
	if err != nil {
		return nil, err
	}
	resolved := strings.NewReplacer(
		"__PORT__", port,
		"__TEMPLATES_DIR__", templatesDir,
		"__PROJECT_DIR__", projectDir,
		"__INTERNAL_LOG__", internalLog,
		"__EVENT_STORE_PATH__", defaultEventStorePath,
	).Replace(string(content))
	var payload config.GlobalConfig
	if err := json.Unmarshal([]byte(resolved), &payload); err != nil {
		return nil, fmt.Errorf("parse rendered benchmark config: %w", err)
	}
	payload.ResolveProjects()
	if payload.Proxy != nil && strings.TrimSpace(payload.Proxy.LogFile) != "" {
		payload.Proxy.LogFile = resolveBenchmarkBaseConfigPath(baseConfigPath, payload.Proxy.LogFile)
	}
	effectiveEventStorePath := defaultEventStorePath
	if project := benchmarkDefaultProject(&payload); project != nil {
		if taskVerification := project.TaskVerificationCapability(); taskVerification != nil && strings.TrimSpace(taskVerification.TemplatesPath) != "" {
			taskVerification.TemplatesPath = resolveBenchmarkBaseConfigPath(baseConfigPath, taskVerification.TemplatesPath)
		}
		if settings := project.EventStorageCapability(); settings != nil && strings.TrimSpace(settings.Path) != "" {
			settings.Path = resolveBenchmarkBaseConfigPath(baseConfigPath, settings.Path)
			effectiveEventStorePath = settings.Path
		}
	}
	effectiveEventStorePath = firstNonEmpty(effectiveEventStorePath, defaultEventStorePath)
	effectiveEventStorePath = resolveBenchmarkConfigPath(projectDir, effectiveEventStorePath)
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode rendered benchmark config: %w", err)
	}
	return &renderedCentianConfig{
		Content:                 encoded,
		EffectiveEventStorePath: effectiveEventStorePath,
	}, nil
}

// loadBenchmarkCentianConfigTemplate loads either the embedded default config or a caller-supplied base file.
func loadBenchmarkCentianConfigTemplate(baseConfigPath string) ([]byte, error) {
	baseConfigPath = strings.TrimSpace(baseConfigPath)
	if baseConfigPath == "" {
		content, err := asset("centian_config.json")
		if err != nil {
			return nil, err
		}
		return []byte(content), nil
	}
	content, err := os.ReadFile(baseConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read centian config %q: %w", baseConfigPath, err)
	}
	return content, nil
}

// resolveBenchmarkBaseConfigPath resolves configured paths relative to the chosen base config file.
func resolveBenchmarkBaseConfigPath(baseConfigPath string, configuredPath string) string {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" || filepath.IsAbs(configuredPath) {
		return configuredPath
	}
	baseConfigPath = strings.TrimSpace(baseConfigPath)
	if baseConfigPath == "" {
		return configuredPath
	}
	return filepath.Join(filepath.Dir(baseConfigPath), configuredPath)
}

// benchmarkDefaultProject returns the effective project config used for a benchmark run.
func benchmarkDefaultProject(cfg *config.GlobalConfig) *config.ProjectConfig {
	if cfg == nil {
		return nil
	}
	if project := cfg.GetDefaultProject(); project != nil {
		return project
	}
	if len(cfg.Projects) != 1 {
		return nil
	}
	for _, project := range cfg.Projects {
		return project
	}
	return nil
}

// resolveBenchmarkConfigPath resolves non-absolute configured paths relative to the run workspace.
func resolveBenchmarkConfigPath(workingDir string, configuredPath string) string {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" || filepath.IsAbs(configuredPath) {
		return configuredPath
	}
	return filepath.Join(workingDir, configuredPath)
}

// createSessionDir creates a unique timestamped session directory under the suite output root.
func createSessionDir(outputRoot string, suiteID string, label string, now time.Time) (string, error) {
	if label = common.NormalizeSlug(label); label == "" {
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

// relativeRunDir returns the run path relative to the enclosing session directory when possible.
func relativeRunDir(sessionDir string, runDir string) string {
	rel, err := filepath.Rel(sessionDir, runDir)
	if err != nil {
		return runDir
	}
	return rel
}

// selectedModel returns the agent-specific model override selected for the run.
func selectedModel(agent string, models AgentModels) string {
	switch agent {
	case agentrunner.AgentClaude:
		return models.Claude
	case agentrunner.AgentGemini:
		return models.Gemini
	case agentrunner.AgentCodex:
		return models.Codex
	case agentrunner.AgentCodexOllama:
		return models.CodexOllamaProfile
	default:
		return ""
	}
}

// taskRunIDs extracts run IDs in order from fetched task-run summaries.
func taskRunIDs(runs []persistence.TaskRunSummary) []string {
	result := make([]string, 0, len(runs))
	for _, run := range runs {
		result = append(result, run.RunID)
	}
	return result
}

// taskRunIDSet builds a membership set for fetched task runs.
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

// filterNewTaskRuns removes runs that already existed before the benchmark agent started.
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

// latestTaskRun returns the most recently started task run from the observed set.
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

// startCentianProcess launches a run-local Centian child process and waits for its API/MCP endpoints.
func startCentianProcess(ctx context.Context, opts StartCentianOptions) (*StartedCentian, error) {
	// This stays package-local because the benchmark runner needs custom lifecycle,
	// readiness, and shutdown behavior that differs from the demo flow.
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

// waitForCentian polls API and MCP endpoints until the child server is ready.
func waitForCentian(baseURL string, mcpURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	apiURL := baseURL + "/api/task-runs"
	err := common.WaitForReadiness(client, 45*time.Second, 500*time.Millisecond, nil, func(client *http.Client) bool {
		return common.IsEndpointReachable(client, mcpURL) &&
			common.IsJSONEndpointReady(client, apiURL)
	})
	if errors.Is(err, common.ErrReadinessTimeout) {
		return fmt.Errorf("centian did not become ready in time")
	}
	return err
}

// fetchTaskRuns reads the current task-run list from the run-local Centian API.
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

// fetchTaskRunEvents reads raw task-run events from the run-local Centian API.
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

// findLatestRequestLog returns the most recent request log created for the run.
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
