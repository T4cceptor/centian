package common

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WritePIDFile writes one process ID to disk with a trailing newline.
func WritePIDFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

// FirstNonEmpty returns the first non-blank string in values.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// SortedSetValues returns deterministic, sorted values from a string set.
func SortedSetValues(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// BestTime prefers primary when set and otherwise falls back to the alternate timestamp.
func BestTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary
	}
	return fallback
}

// TimePointerMillis converts a non-zero timestamp into a nullable unix-millis pointer.
func TimePointerMillis(value time.Time) *int64 {
	if value.IsZero() {
		return nil
	}
	millis := value.UnixMilli()
	return &millis
}

// MedianFloat returns the median of values, or zero when the slice is empty.
func MedianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}
