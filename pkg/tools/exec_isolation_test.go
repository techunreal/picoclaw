package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExecRejectsSiblingAgentWorkspace(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	for _, dir := range []string{alpha, beta} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(beta, "AGENT.md"), []byte("beta personality"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tool, err := NewExecTool(alpha, true)
	if err != nil {
		t.Fatalf("NewExecTool() error = %v", err)
	}

	command := "cat ../beta/AGENT.md"
	if runtime.GOOS == "windows" {
		command = `type ..\beta\AGENT.md`
	}
	result := tool.Execute(context.Background(), map[string]any{"command": command})
	if !result.IsError {
		t.Fatalf("exec reached a sibling workspace, result: %s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "beta personality") {
		t.Fatalf("exec leaked sibling workspace content: %s", result.ForLLM)
	}
}

func TestExecRejectsSiblingAgentWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	for _, dir := range []string{alpha, beta} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}

	tool, err := NewExecTool(alpha, true)
	if err != nil {
		t.Fatalf("NewExecTool() error = %v", err)
	}

	result := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
		"cwd":     beta,
	})
	if !result.IsError {
		t.Fatalf("exec ran with a sibling workspace as cwd, result: %s", result.ForLLM)
	}
}
