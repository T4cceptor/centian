package common

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
)

// AllocateFreePort reserves an ephemeral localhost port and returns its port number.
func AllocateFreePort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("allocate port: %w", err)
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", fmt.Errorf("split host port: %w", err)
	}
	return port, nil
}

// ShellQuote wraps a path in single quotes for copy-paste shell commands.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// TrimOutput limits surfaced command output to a readable tail section.
func TrimOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4000 {
		return value[:4000] + "\n...truncated..."
	}
	return value
}

// ProcessExists reports whether the current OS still sees the given PID.
func ProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.EPERM
}
