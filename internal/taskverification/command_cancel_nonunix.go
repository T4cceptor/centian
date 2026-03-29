//go:build !unix

package taskverification

import "os/exec"

func configureCommandCancellation(_ *exec.Cmd) {}
