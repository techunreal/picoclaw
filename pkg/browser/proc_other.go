//go:build !windows

package browser

import "os/exec"

func applyHiddenProcAttrs(cmd *exec.Cmd) {}
