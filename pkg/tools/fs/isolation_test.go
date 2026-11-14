package fstools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolationWorkspaces(t *testing.T) (alpha, beta string) {
	t.Helper()
	root := t.TempDir()
	alpha = filepath.Join(root, "alpha")
	beta = filepath.Join(root, "beta")
	for _, dir := range []string{alpha, beta} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(beta, "AGENT.md"), []byte("beta personality"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return alpha, beta
}

func TestWriteFileRejectsSiblingAgentWorkspace(t *testing.T) {
	alpha, beta := isolationWorkspaces(t)
	tool := NewWriteFileTool(alpha, true)

	paths := []string{
		filepath.Join("..", "beta", "AGENT.md"),
		filepath.Join(beta, "AGENT.md"),
	}
	for _, target := range paths {
		result := tool.Execute(context.Background(), map[string]any{
			"path":    target,
			"content": "hacked",
		})
		if !result.IsError {
			t.Fatalf("write to %q succeeded, want an isolation error", target)
		}
		if !strings.Contains(strings.ToLower(result.ForLLM), "outside the workspace") &&
			!strings.Contains(strings.ToLower(result.ForLLM), "access denied") &&
			!strings.Contains(strings.ToLower(result.ForLLM), "escapes workspace") {
			t.Fatalf("unexpected error for %q: %s", target, result.ForLLM)
		}
	}

	data, err := os.ReadFile(filepath.Join(beta, "AGENT.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "beta personality" {
		t.Fatalf("beta AGENT.md was modified: %q", data)
	}
}

func TestReadFileRejectsSiblingAgentWorkspace(t *testing.T) {
	alpha, beta := isolationWorkspaces(t)
	tool := NewReadFileTool(alpha, true, 1<<20)

	paths := []string{
		filepath.Join("..", "beta", "AGENT.md"),
		filepath.Join(beta, "AGENT.md"),
	}
	for _, target := range paths {
		result := tool.Execute(context.Background(), map[string]any{"path": target})
		if !result.IsError {
			t.Fatalf("read of %q succeeded, want an isolation error", target)
		}
		if strings.Contains(result.ForLLM, "beta personality") {
			t.Fatalf("read of %q leaked the sibling workspace content", target)
		}
	}
}

func TestWriteFileRejectsSymlinkEscapeToSiblingWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	alpha, beta := isolationWorkspaces(t)
	if err := os.Symlink(beta, filepath.Join(alpha, "link")); err != nil {
		t.Skipf("symlink not available: %v", err)
	}

	tool := NewWriteFileTool(alpha, true)
	result := tool.Execute(context.Background(), map[string]any{
		"path":    filepath.Join("link", "AGENT.md"),
		"content": "hacked",
	})
	if !result.IsError {
		t.Fatal("write through a symlink to another workspace succeeded")
	}

	data, err := os.ReadFile(filepath.Join(beta, "AGENT.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "beta personality" {
		t.Fatalf("beta AGENT.md was modified through a symlink: %q", data)
	}
}
