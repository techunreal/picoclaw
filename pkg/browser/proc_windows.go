//go:build windows

package browser

import (
	"os/exec"
	"syscall"
)

// applyHiddenProcAttrs keeps helper processes (agent-browser) from opening a
// console window, including when Windows Terminal is the default terminal.
func applyHiddenProcAttrs(cmd *exec.Cmd) {
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
