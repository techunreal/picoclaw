package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func officePythonName() string {
	if runtime.GOOS == "windows" {
		return "python.exe"
	}
	return "python3"
}

func TestOfficePythonPathFindsBundledRuntime(t *testing.T) {
	appDir := t.TempDir()
	if got := OfficePythonPath(appDir); got != "" {
		t.Fatalf("OfficePythonPath(empty runtime) = %q, want empty", got)
	}

	runtimeDir := filepath.Join(appDir, "tools", "python")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	python := filepath.Join(runtimeDir, officePythonName())
	if err := os.WriteFile(python, []byte("binary"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if got := OfficePythonPath(appDir); got != python {
		t.Fatalf("OfficePythonPath() = %q, want %q", got, python)
	}
}

func TestEnsureOfficePythonEnvKeepsExplicitValue(t *testing.T) {
	const explicit = `C:\custom\python.exe`
	t.Setenv(EnvOfficePython, explicit)
	if got := EnsureOfficePythonEnv(); got != explicit {
		t.Fatalf("EnsureOfficePythonEnv() = %q, want %q", got, explicit)
	}
}
