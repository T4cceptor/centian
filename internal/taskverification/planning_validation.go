package taskverification

import (
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
)

// PlanningValidationError captures machine-readable planning contract failures.
type PlanningValidationError struct {
	RequiredParameterNames []string
	ProvidedParameterNames []string
	MissingParameters      []string
	UnknownParameters      []string
}

// Error returns a concise human-facing validation message.
func (e *PlanningValidationError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.MissingParameters) == 1 && len(e.UnknownParameters) == 0 {
		return fmt.Sprintf("planning.parameters.%s is required", e.MissingParameters[0])
	}
	if len(e.UnknownParameters) == 1 && len(e.MissingParameters) == 0 {
		return fmt.Sprintf("planning.parameters.%s is unknown", e.UnknownParameters[0])
	}

	parts := make([]string, 0, 2)
	if len(e.MissingParameters) > 0 {
		parts = append(parts, fmt.Sprintf("missing required planning parameters: %s", strings.Join(e.MissingParameters, ", ")))
	}
	if len(e.UnknownParameters) > 0 {
		parts = append(parts, fmt.Sprintf("unknown planning parameters: %s", strings.Join(e.UnknownParameters, ", ")))
	}
	if len(parts) == 0 {
		return "planning.parameters is invalid"
	}
	return "planning.parameters is invalid: " + strings.Join(parts, "; ")
}

func newPlanningValidationError(required, provided, missing, unknown []string) *PlanningValidationError {
	return &PlanningValidationError{
		RequiredParameterNames: common.SortedStringsCopy(required),
		ProvidedParameterNames: common.SortedStringsCopy(provided),
		MissingParameters:      common.SortedStringsCopy(missing),
		UnknownParameters:      common.SortedStringsCopy(unknown),
	}
}
