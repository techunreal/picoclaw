//go:build windows

package utils

import (
	"os/exec"
	"syscall"
)

// LauncherExecCommand creates an exec.Cmd with the HideWindow attribute set
// to prevent console window flashes on Windows.
func LauncherExecCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	ApplyLauncherProcAttrs(cmd)
	return cmd
}

// ApplyLauncherProcAttrs applies Windows-specific process attributes to hide
// the console window. It is safe to call with a nil cmd.
//
// HideWindow alone is not enough when Windows Terminal is the default terminal:
// the terminal can still surface a window. CREATE_NO_WINDOW creates the console
// without any window at all, which is what the gateway and helper commands need.
func ApplyLauncherProcAttrs(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	const createNoWindow = 0x08000000
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
