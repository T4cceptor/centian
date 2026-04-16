package common

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// PromptDefinition is the common YAML-backed prompt file format used by
// demo and benchmark flows.
type PromptDefinition struct {
	Prompt string `yaml:"prompt"`
}

// LoadPromptDefinition reads one YAML prompt file and validates that it
// contains a non-empty prompt body.
func LoadPromptDefinition(path string) (*PromptDefinition, error) {
	// #nosec G304 -- prompt paths come from trusted benchmark/demo configuration and tests.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", path, err)
	}

	var prompt PromptDefinition
	if err := yaml.Unmarshal(data, &prompt); err != nil {
		return nil, fmt.Errorf("failed to parse %q: %w", path, err)
	}
	prompt.Prompt = strings.TrimSpace(prompt.Prompt)
	if prompt.Prompt == "" {
		return nil, fmt.Errorf("prompt file %q must define a non-empty prompt", path)
	}
	return &prompt, nil
}
