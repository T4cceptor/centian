//go:build windows

package agentrunner

import "syscall"

// detachedProcessAttrs returns nil on Windows because the current runner does not need extra attrs there.
func detachedProcessAttrs() *syscall.SysProcAttr {
	return nil
}
