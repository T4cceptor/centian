package common

import (
	"fmt"
	"strings"
)

// NormalizeSlug lowercases free-form labels into stable filesystem-safe segments.
func NormalizeSlug(value string) string {
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

// ValidateUniqueTrimmedStrings rejects blank values and duplicates after trimming whitespace.
func ValidateUniqueTrimmedStrings(field string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("%s[%d] is required", field, index)
		}
		if _, exists := seen[trimmed]; exists {
			return fmt.Errorf("%s contains duplicate value %q", field, trimmed)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}
