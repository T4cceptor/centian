package common

import "testing"

func TestNormalizeSlug(t *testing.T) {
	if got := NormalizeSlug("  Hello, World!  "); got != "hello_world" {
		t.Fatalf("unexpected slug: %q", got)
	}
}

func TestValidateUniqueTrimmedStrings(t *testing.T) {
	if err := ValidateUniqueTrimmedStrings("values", []string{" a ", "b"}); err != nil {
		t.Fatalf("expected valid unique strings, got %v", err)
	}
	if err := ValidateUniqueTrimmedStrings("values", []string{"a", " a "}); err == nil {
		t.Fatalf("expected duplicate validation error")
	}
	if err := ValidateUniqueTrimmedStrings("values", []string{"a", " "}); err == nil {
		t.Fatalf("expected blank validation error")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		limit    int
		expected string
	}{
		{name: "short", input: "short", limit: 500, expected: "short"},
		{name: "tiny limit", input: "not long, but too long", limit: 2, expected: "..."},
		{name: "truncate", input: "something long", limit: 9, expected: "someth..."},
		{name: "equal length", input: "something", limit: 9, expected: "something"},
		{name: "rune safe", input: "äöüßabcd", limit: 6, expected: "äöü..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateString(tt.input, tt.limit); got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
