package benchmarks

import (
	"embed"
	"fmt"
)

//go:embed assets/*
var embeddedAssets embed.FS

func asset(name string) (string, error) {
	data, err := embeddedAssets.ReadFile("assets/" + name)
	if err != nil {
		return "", fmt.Errorf("read asset %q: %w", name, err)
	}
	return string(data), nil
}
