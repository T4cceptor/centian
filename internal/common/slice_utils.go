package common

import "strings"

// NormalizeCSVList splits comma-separated values, trims blanks, and preserves first-seen order.
func NormalizeCSVList(values []string) []string {
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
