//go:build windows

package api

import (
	"os/exec"
)

// launchReplacement starts a new launcher instance through Explorer. Explorer
// creates the process outside this launcher's job object, so it survives the
// current process exiting.
func launchReplacement(exe string) error {
	return exec.Command("explorer.exe", exe).Start()
}
