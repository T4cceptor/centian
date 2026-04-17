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

func TestSortedSetValues(t *testing.T) {
	set := map[string]struct{}{"b": {}, "a": {}, "c": {}}
	if got := SortedSetValues(set); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("unexpected sorted set values: %#v", got)
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
