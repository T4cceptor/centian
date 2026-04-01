package tasktemplates

import "embed"

// FS contains the built-in task verification templates bundled into the binary.
//
//go:embed integrated/*.yaml
var FS embed.FS
