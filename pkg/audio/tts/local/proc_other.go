//go:build !windows

package local

import (
	"os"
	"os/exec"
)

func isWindows() bool { return false }

func hideWindow(_ *exec.Cmd) {}

func shortPath(path string) string { return path }

func killProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}
