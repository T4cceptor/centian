package benchmarks

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
)

const (
	suiteFileName = "suite.yaml"
	caseFileName  = "case.yaml"
)

// SuiteDefinition is the repo-tracked top-level benchmark suite definition.
type SuiteDefinition struct {
	Version string         `yaml:"version"`
	Suite   SuiteMetadata  `yaml:"suite"`
	Cases   []SuiteCaseRef `yaml:"cases"`
}

// SuiteMetadata describes one benchmark suite.
type SuiteMetadata struct {
	ID                       string `yaml:"id"`
	Name                     string `yaml:"name"`
	Description              string `yaml:"description"`
	TemplateID               string `yaml:"templateId"`
	LocalOnly                bool   `yaml:"localOnly"`
	ScoringSchemaVersion     string `yaml:"scoringSchemaVersion"`
	ExecutionProtocolVersion string `yaml:"executionProtocolVersion"`
}

// SuiteCaseRef points to one benchmark case directory relative to the suite root.
type SuiteCaseRef struct {
	ID   string `yaml:"id"`
	Path string `yaml:"path"`
}

// CaseDefinition is the repo-tracked definition of one benchmark case.
type CaseDefinition struct {
	Version      string            `yaml:"version"`
	Case         CaseMetadata      `yaml:"case"`
	PromptFile   string            `yaml:"promptFile"`
	Fixture      FixtureDefinition `yaml:"fixture"`
	Expectations CaseExpectations  `yaml:"expectations"`
	Constraints  CaseConstraints   `yaml:"constraints"`
}

// CaseMetadata describes one benchmark case.
type CaseMetadata struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// FixtureDefinition describes the seeded starting state of a benchmark case.
type FixtureDefinition struct {
	SeedPath                 string `yaml:"seedPath"`
	ResetMode                string `yaml:"resetMode"`
	StartingRepoStateSummary string `yaml:"startingRepoStateSummary"`
}

// CaseExpectations defines the contract the benchmark runner later verifies.
type CaseExpectations struct {
	SelectedCommand       string            `yaml:"selectedCommand"`
	RedSignal             SignalExpectation `yaml:"redSignal"`
	GreenConditionSummary string            `yaml:"greenConditionSummary"`
}

// SignalExpectation describes the expected red baseline signal.
type SignalExpectation struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
}

// CaseConstraints describes path-level restrictions or allowances for the case.
type CaseConstraints struct {
	LockedPaths            []string `yaml:"lockedPaths"`
	AllowedAdditionalPaths []string `yaml:"allowedAdditionalPaths"`
}

// PromptDefinition is the user-style prompt for one benchmark case.
type PromptDefinition = common.PromptDefinition

// LoadSuite loads and validates a benchmark suite rooted at the given directory.
func LoadSuite(root string) (*SuiteDefinition, error) {
	suitePath := filepath.Join(root, suiteFileName)
	var suite SuiteDefinition
	if err := common.ReadYAMLFile(suitePath, &suite); err != nil {
		return nil, err
	}
	if err := ValidateSuite(root, &suite); err != nil {
		return nil, err
	}
	return &suite, nil
}

// LoadCase loads and validates one benchmark case referenced by the suite.
func LoadCase(suiteRoot string, ref SuiteCaseRef) (*CaseDefinition, error) {
	caseRoot, err := common.ResolveExistingDirUnderRoot(suiteRoot, ref.Path, "case path", "suite root")
	if err != nil {
		return nil, err
	}

	casePath := filepath.Join(caseRoot, caseFileName)
	var def CaseDefinition
	if err := common.ReadYAMLFile(casePath, &def); err != nil {
		return nil, err
	}
	if err := validateCase(caseRoot, ref, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

// LoadPrompt loads and validates the prompt file for one benchmark case.
func LoadPrompt(caseRoot, promptFile string) (*PromptDefinition, error) {
	promptPath, err := common.ResolveExistingFileUnderRoot(caseRoot, promptFile, "prompt file", "suite root")
	if err != nil {
		return nil, err
	}
	return common.LoadPromptDefinition(promptPath)
}

// ValidateSuite validates a suite and all referenced cases structurally.
func ValidateSuite(root string, suite *SuiteDefinition) error {
	if suite == nil {
		return fmt.Errorf("suite definition is required")
	}
	if strings.TrimSpace(suite.Version) == "" {
		return fmt.Errorf("suite version is required")
	}
	if strings.TrimSpace(suite.Suite.ID) == "" {
		return fmt.Errorf("suite id is required")
	}
	if strings.TrimSpace(suite.Suite.Name) == "" {
		return fmt.Errorf("suite name is required")
	}
	if strings.TrimSpace(suite.Suite.TemplateID) == "" {
		return fmt.Errorf("suite templateId is required")
	}
	if strings.TrimSpace(suite.Suite.ScoringSchemaVersion) == "" {
		return fmt.Errorf("suite scoringSchemaVersion is required")
	}
	if strings.TrimSpace(suite.Suite.ExecutionProtocolVersion) == "" {
		return fmt.Errorf("suite executionProtocolVersion is required")
	}
	if len(suite.Cases) == 0 {
		return fmt.Errorf("suite must define at least one case")
	}

	seenCaseIDs := make(map[string]struct{}, len(suite.Cases))
	for _, ref := range suite.Cases {
		caseID := strings.TrimSpace(ref.ID)
		if caseID == "" {
			return fmt.Errorf("suite case id is required")
		}
		if _, exists := seenCaseIDs[caseID]; exists {
			return fmt.Errorf("duplicate suite case id %q", caseID)
		}
		seenCaseIDs[caseID] = struct{}{}

		if _, err := LoadCase(root, ref); err != nil {
			return fmt.Errorf("suite case %q is invalid: %w", caseID, err)
		}
	}

	return nil
}

// validateCase enforces one case's required files and prompt/fixture contract.
func validateCase(caseRoot string, ref SuiteCaseRef, def *CaseDefinition) error {
	if def == nil {
		return fmt.Errorf("case definition is required")
	}
	if strings.TrimSpace(def.Version) == "" {
		return fmt.Errorf("case version is required")
	}
	if strings.TrimSpace(def.Case.ID) == "" {
		return fmt.Errorf("case id is required")
	}
	if strings.TrimSpace(def.Case.Name) == "" {
		return fmt.Errorf("case name is required")
	}
	if strings.TrimSpace(def.Case.Description) == "" {
		return fmt.Errorf("case description is required")
	}
	if expected := strings.TrimSpace(ref.ID); expected != "" && def.Case.ID != expected {
		return fmt.Errorf("case id %q does not match suite case id %q", def.Case.ID, expected)
	}
	if strings.TrimSpace(def.PromptFile) == "" {
		return fmt.Errorf("case promptFile is required")
	}
	if strings.TrimSpace(def.Fixture.SeedPath) == "" {
		return fmt.Errorf("case fixture.seedPath is required")
	}
	if strings.TrimSpace(def.Fixture.ResetMode) == "" {
		return fmt.Errorf("case fixture.resetMode is required")
	}
	if strings.TrimSpace(def.Fixture.StartingRepoStateSummary) == "" {
		return fmt.Errorf("case fixture.startingRepoStateSummary is required")
	}
	if strings.TrimSpace(def.Expectations.SelectedCommand) == "" {
		return fmt.Errorf("case expectations.selectedCommand is required")
	}
	if strings.TrimSpace(def.Expectations.RedSignal.Type) == "" {
		return fmt.Errorf("case expectations.redSignal.type is required")
	}
	if strings.TrimSpace(def.Expectations.RedSignal.Value) == "" {
		return fmt.Errorf("case expectations.redSignal.value is required")
	}
	if strings.TrimSpace(def.Expectations.GreenConditionSummary) == "" {
		return fmt.Errorf("case expectations.greenConditionSummary is required")
	}
	if len(def.Constraints.LockedPaths) == 0 {
		return fmt.Errorf("case constraints.lockedPaths must define at least one path")
	}

	if _, err := LoadPrompt(caseRoot, def.PromptFile); err != nil {
		return err
	}
	fixtureRoot, err := common.ResolveExistingDirUnderRoot(caseRoot, def.Fixture.SeedPath, "fixture seedPath", "suite root")
	if err != nil {
		return err
	}
	for _, lockedPath := range def.Constraints.LockedPaths {
		if err := common.EnsureExistingPathUnderRoot(fixtureRoot, lockedPath, "locked path", "suite root"); err != nil {
			return err
		}
	}
	for _, allowedPath := range def.Constraints.AllowedAdditionalPaths {
		if err := common.EnsureExistingPathUnderRoot(fixtureRoot, allowedPath, "allowedAdditionalPaths entry", "suite root"); err != nil {
			return err
		}
	}

	return nil
}
