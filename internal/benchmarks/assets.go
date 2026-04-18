package benchmarks

import (
	"embed"
	"fmt"
)

//go:embed assets/*
var embeddedAssets embed.FS

// asset returns the embedded benchmark asset contents as a string.
func asset(name string) (string, error) {
	data, err := embeddedAssets.ReadFile("assets/" + name)
	if err != nil {
		return "", fmt.Errorf("read asset %q: %w", name, err)
	}
	return string(data), nil
}
