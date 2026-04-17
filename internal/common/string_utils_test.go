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
