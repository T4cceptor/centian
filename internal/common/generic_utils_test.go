package common

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWritePIDFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "centian.pid")
	if err := WritePIDFile(path, 12345); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "12345" {
		t.Fatalf("expected pid 12345, got %q", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", " ", "value", "other"); got != "value" {
		t.Fatalf("expected first non-empty value, got %q", got)
	}
}

func TestLastNonEmpty(t *testing.T) {
	if got := LastNonEmpty([]string{"", " a ", " ", "b"}); got != "b" {
		t.Fatalf("unexpected last non-empty value: %q", got)
	}
	if got := LastNonEmpty([]string{"", " "}); got != "" {
		t.Fatalf("expected empty last non-empty value, got %q", got)
	}
}

func TestNonEmptyStrings(t *testing.T) {
	if got := NonEmptyStrings("", " a ", " ", "b"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected non-empty strings: %#v", got)
	}
	if got := NonEmptyStrings("", " "); got != nil {
		t.Fatalf("expected nil for empty inputs, got %#v", got)
	}
}

func TestSortedSetValues(t *testing.T) {
	set := map[string]struct{}{"b": {}, "a": {}, "c": {}}
	if got := SortedSetValues(set); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("unexpected sorted set values: %#v", got)
	}
}

func TestJoinTrimmed(t *testing.T) {
	if got := JoinTrimmed(" agent ", " model ", "::"); got != "agent::model" {
		t.Fatalf("unexpected joined value: %q", got)
	}
	if got := JoinTrimmedIfRight(" left ", " ", "::"); got != "left" {
		t.Fatalf("unexpected conditional join without right value: %q", got)
	}
	if got := JoinTrimmedIfRight(" left ", " right ", "::"); got != "left::right" {
		t.Fatalf("unexpected conditional join with right value: %q", got)
	}
}

func TestBestTimeAndTimePointerMillis(t *testing.T) {
	fallback := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	primary := fallback.Add(time.Minute)
	if got := BestTime(primary, fallback); !got.Equal(primary) {
		t.Fatalf("expected primary time, got %v", got)
	}
	if got := BestTime(time.Time{}, fallback); !got.Equal(fallback) {
		t.Fatalf("expected fallback time, got %v", got)
	}
	if got := TimePointerMillis(time.Time{}); got != nil {
		t.Fatalf("expected nil millis pointer, got %#v", got)
	}
	if got := TimePointerMillis(primary); got == nil || *got != primary.UnixMilli() {
		t.Fatalf("unexpected millis pointer: %#v", got)
	}
	if got := LaterTime(time.Time{}, fallback); !got.Equal(fallback) {
		t.Fatalf("expected fallback as later time, got %v", got)
	}
	if got := LaterTime(fallback, primary); !got.Equal(primary) {
		t.Fatalf("expected primary as later time, got %v", got)
	}
}

func TestMedianFloat(t *testing.T) {
	if got := MedianFloat(nil); got != 0 {
		t.Fatalf("expected zero median for empty slice, got %f", got)
	}
	if got := MedianFloat([]float64{3, 1, 2}); got != 2 {
		t.Fatalf("expected odd median 2, got %f", got)
	}
	if got := MedianFloat([]float64{4, 1, 2, 3}); got != 2.5 {
		t.Fatalf("expected even median 2.5, got %f", got)
	}
}

func TestRatio(t *testing.T) {
	if got := Ratio(1, 0); got != 0 {
		t.Fatalf("expected zero ratio for empty denominator, got %f", got)
	}
	if got := Ratio(1, 4); got != 0.25 {
		t.Fatalf("expected ratio 0.25, got %f", got)
	}
}

func TestCountBy(t *testing.T) {
	values := []int{1, 2, 3, 4, 5}
	if got := CountBy(values, func(value int) bool { return value%2 == 0 }); got != 2 {
		t.Fatalf("expected even count 2, got %d", got)
	}
}

func TestSumIntsAndMedians(t *testing.T) {
	if got := SumInts([]int{1, 2, 3}); got != 6 {
		t.Fatalf("expected sum 6, got %d", got)
	}
	if got := MedianInt(nil); got != 0 {
		t.Fatalf("expected zero median for empty ints, got %d", got)
	}
	if got := MedianInt([]int{5, 1, 3}); got != 3 {
		t.Fatalf("expected odd int median 3, got %d", got)
	}
	if got := MedianInt([]int{1, 2, 3, 4}); got != 2 {
		t.Fatalf("expected even int median 2, got %d", got)
	}
	if got := MedianInt64(nil); got != 0 {
		t.Fatalf("expected zero median for empty int64s, got %d", got)
	}
	if got := MedianInt64([]int64{5, 1, 3}); got != 3 {
		t.Fatalf("expected odd int64 median 3, got %d", got)
	}
	if got := MedianInt64([]int64{1, 2, 3, 4}); got != 2 {
		t.Fatalf("expected even int64 median 2, got %d", got)
	}
}

func TestNowUTCAndUnixMillisHelpers(t *testing.T) {
	if got := NowUTC(); got.Location() != time.UTC {
		t.Fatalf("expected UTC time, got %v", got.Location())
	}

	fallback := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	primary := fallback.Add(2 * time.Minute)
	primaryMillis := primary.UnixMilli()

	if got := TimeFromUnixMillis(0); !got.IsZero() {
		t.Fatalf("expected zero time for empty unix millis, got %v", got)
	}
	if got := TimeFromUnixMillis(primaryMillis); !got.Equal(primary) {
		t.Fatalf("unexpected unix millis conversion: %v", got)
	}
	if got := TimeFromUnixMillisOrFallback(&primaryMillis, fallback.UnixMilli()); !got.Equal(primary) {
		t.Fatalf("unexpected primary unix millis conversion: %v", got)
	}
	if got := TimeFromUnixMillisOrFallback(nil, fallback.UnixMilli()); !got.Equal(fallback) {
		t.Fatalf("unexpected fallback unix millis conversion: %v", got)
	}
	if got := DurationSeconds(ptrInt64(2500), time.Time{}, time.Time{}); got != 2.5 {
		t.Fatalf("expected persisted duration 2.5, got %f", got)
	}
	if got := DurationSeconds(nil, fallback, primary); got != 120 {
		t.Fatalf("expected derived duration 120, got %f", got)
	}
}

func TestCloneStringMap(t *testing.T) {
	if got := CloneStringMap(nil); len(got) != 0 {
		t.Fatalf("expected empty map for nil input, got %#v", got)
	}

	source := map[string]string{"a": "1", "b": "2"}
	cloned := CloneStringMap(source)
	if !reflect.DeepEqual(cloned, source) {
		t.Fatalf("unexpected cloned map: %#v", cloned)
	}
	cloned["a"] = "3"
	if source["a"] != "1" {
		t.Fatalf("expected source map to remain unchanged, got %#v", source)
	}
}

func ptrInt64(value int64) *int64 {
	return &value
}
