package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LocateBrowser exposes system browser detection for status endpoints.
func LocateBrowser(explicit string) string {
	return resolveBrowser(explicit)
}

// IsExecutableFile reports whether path points at an existing file.
func IsExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Version runs the agent-browser CLI --version with a short timeout.
func Version(binaryPath string) (string, error) {
	binary, err := resolveBinary(binaryPath)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--version")
	applyHiddenProcAttrs(cmd)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if ctx.Err() != nil {
		return "", fmt.Errorf("agent-browser did not respond in time")
	}
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("agent-browser check failed: %s", firstLine(text))
		}
		return "", fmt.Errorf("agent-browser check failed: %w", err)
	}
	if text == "" {
		text = "agent-browser"
	}
	return text, nil
}
