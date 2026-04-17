package common

import (
	"encoding/json"
	"testing"
)

func TestParseInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int64
		ok       bool
	}{
		{name: "int", input: 12, expected: 12, ok: true},
		{name: "int64", input: int64(34), expected: 34, ok: true},
		{name: "float64", input: 56.0, expected: 56, ok: true},
		{name: "json number", input: json.Number("78"), expected: 78, ok: true},
		{name: "string", input: "90", expected: 90, ok: true},
		{name: "invalid", input: "x", expected: 0, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, ok := ParseInt64(tt.input)
			if ok != tt.ok {
				t.Fatalf("expected ok=%v, got %v", tt.ok, ok)
			}
			if actual != tt.expected {
				t.Fatalf("expected %d, got %d", tt.expected, actual)
			}
		})
	}
}

func TestJSONValueHelpers(t *testing.T) {
	obj := map[string]any{"x": 1}
	if got := AnyMap(obj); got["x"] != 1 {
		t.Fatalf("expected AnyMap to return input map, got %+v", got)
	}
	if got := AnyMap("x"); got != nil {
		t.Fatalf("expected nil for non-map input, got %+v", got)
	}
	if got := StringValue("abc"); got != "abc" {
		t.Fatalf("expected string value, got %q", got)
	}
	if got := StringValue(1); got != "" {
		t.Fatalf("expected empty string for non-string input, got %q", got)
	}
	if got := IntPtrFromAny("12"); got == nil || *got != 12 {
		t.Fatalf("expected int ptr 12, got %+v", got)
	}
	if got := Int64PtrFromAny(json.Number("34")); got == nil || *got != 34 {
		t.Fatalf("expected int64 ptr 34, got %+v", got)
	}
	if got := Float64PtrFromAny("5.5"); got == nil || *got != 5.5 {
		t.Fatalf("expected float64 ptr 5.5, got %+v", got)
	}

	generic, err := JSONGenericValue(struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}{Name: "demo", Tags: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("JSONGenericValue: %v", err)
	}
	payload, ok := generic.(map[string]any)
	if !ok {
		t.Fatalf("expected generic object map, got %#v", generic)
	}
	if payload["name"] != "demo" {
		t.Fatalf("expected generic name demo, got %#v", payload["name"])
	}
}
