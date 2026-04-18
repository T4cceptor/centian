package benchmarks

import "fmt"

// suiteContext caches the loaded suite plus resolved case definitions needed for scoring.
type suiteContext struct {
	suite    *SuiteDefinition
	caseDefs map[string]scoreRunContext
}

// loadSuiteContext loads the suite once and expands all case contexts used during scoring.
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
