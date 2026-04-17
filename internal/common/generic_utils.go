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

// LastNonEmpty returns the last non-blank string in values.
func LastNonEmpty(values []string) string {
	for idx := len(values) - 1; idx >= 0; idx-- {
		if trimmed := strings.TrimSpace(values[idx]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// NonEmptyStrings returns trimmed values and drops blank entries.
func NonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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

// LaterTime keeps the later of two timestamps while tolerating zero current values.
func LaterTime(current, candidate time.Time) time.Time {
	if current.IsZero() {
		return candidate
	}
	if candidate.After(current) {
		return candidate
	}
	return current
}

// JoinTrimmed concatenates two trimmed values with separator.
func JoinTrimmed(left, right, separator string) string {
	return strings.TrimSpace(left) + separator + strings.TrimSpace(right)
}

// JoinTrimmedIfRight concatenates two trimmed values when right is non-empty.
func JoinTrimmedIfRight(left, right, separator string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if right == "" {
		return left
	}
	return left + separator + right
}

// TimePointerMillis converts a non-zero timestamp into a nullable unix-millis pointer.
func TimePointerMillis(value time.Time) *int64 {
	if value.IsZero() {
		return nil
	}
	millis := value.UnixMilli()
	return &millis
}

// NowUTC returns the current time in UTC.
func NowUTC() time.Time {
	return time.Now().UTC()
}

// TimeFromUnixMillis converts persisted unix milliseconds into UTC time.
func TimeFromUnixMillis(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

// TimeFromUnixMillisOrFallback resolves a nullable unix-millis field with a required fallback timestamp.
func TimeFromUnixMillisOrFallback(primary *int64, fallback int64) time.Time {
	if primary != nil && *primary > 0 {
		return time.UnixMilli(*primary).UTC()
	}
	return time.UnixMilli(fallback).UTC()
}

// DurationSeconds prefers persisted duration millis and falls back to wall-clock timestamps.
func DurationSeconds(durationMillis *int64, start, end time.Time) float64 {
	if durationMillis != nil && *durationMillis > 0 {
		return float64(*durationMillis) / 1000.0
	}
	return end.Sub(start).Seconds()
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

// Ratio returns numerator divided by denominator, or zero when denominator is empty.
func Ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

// CountBy returns the number of values matching predicate.
func CountBy[T any](values []T, predicate func(T) bool) int {
	total := 0
	for _, value := range values {
		if predicate(value) {
			total++
		}
	}
	return total
}

// SumInts returns the total of values.
func SumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

// MedianInt returns the median integer value, or zero for empty slices.
func MedianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}
	return sorted[middle]
}

// MedianInt64 returns the median int64 value, or zero for empty slices.
func MedianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}
	return sorted[middle]
}

// CloneStringMap returns a shallow copy of parameters and preserves nil/empty as an empty map.
func CloneStringMap(parameters map[string]string) map[string]string {
	if len(parameters) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(parameters))
	for key, value := range parameters {
		cloned[key] = value
	}
	return cloned
}

// SortedStringsCopy returns a sorted copy of values and preserves empty input as nil.
func SortedStringsCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
	return cloned
}
