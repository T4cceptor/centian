//go:build !windows

package agentrunner

import "syscall"

// detachedProcessAttrs returns process attributes that isolate the child process group on Unix.
func detachedProcessAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
