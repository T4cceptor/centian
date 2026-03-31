//go:build windows

package agentrunner

import "syscall"

func detachedProcessAttrs() *syscall.SysProcAttr {
	return nil
}
