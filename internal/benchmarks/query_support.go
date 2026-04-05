package benchmarks

import (
	"fmt"
	"path/filepath"
)

type suiteContext struct {
	suite    *SuiteDefinition
	caseDefs map[string]scoreRunContext
}

func loadSuiteContext(suiteRoot string) (*suiteContext, error) {
	suite, err := LoadSuite(suiteRoot)
	if err != nil {
		return nil, fmt.Errorf("load suite for benchmark scoring: %w", err)
	}
	caseDefs, err := loadCaseContexts(suiteRoot, suite)
	if err != nil {
		return nil, err
	}
	return &suiteContext{suite: suite, caseDefs: caseDefs}, nil
}

func loadRunManifest(path string) (*RunManifest, error) {
	var run RunManifest
	if err := readJSONFile(path, &run); err != nil {
		return nil, fmt.Errorf("load run manifest %q: %w", path, err)
	}
	return &run, nil
}

func loadRunManifestFromSession(sessionPath string, entry SessionRunManifestEntry) (*RunManifest, error) {
	return loadRunManifest(filepath.Join(sessionPath, entry.RelativeRunDir, runFileName))
}
