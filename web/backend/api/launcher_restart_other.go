//go:build !windows

package api

import "os/exec"

// launchReplacement starts a new launcher instance detached from this process.
func launchReplacement(exe string) error {
	cmd := exec.Command(exe)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
