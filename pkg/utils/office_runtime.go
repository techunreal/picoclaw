package utils

import (
	"os"
	"path/filepath"
	"runtime"
)

// EnvOfficePython points office skills at the bundled Python runtime.
const EnvOfficePython = "PICOCLAW_OFFICE_PYTHON"

// OfficePythonPath returns the bundled office Python path for an application
// directory, or an empty string when the runtime is not present.
func OfficePythonPath(appDir string) string {
	if appDir == "" {
		return ""
	}
	name := "python.exe"
	if runtime.GOOS != "windows" {
		name = "python3"
	}
	candidate := filepath.Join(appDir, "tools", "python", name)
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return ""
	}
	return candidate
}

// EnsureOfficePythonEnv sets EnvOfficePython when a bundled runtime sits next
// to the current executable. An existing environment value always wins.
func EnsureOfficePythonEnv() string {
	if existing := os.Getenv(EnvOfficePython); existing != "" {
		return existing
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	path := OfficePythonPath(filepath.Dir(exe))
	if path == "" {
		return ""
	}
	_ = os.Setenv(EnvOfficePython, path)
	return path
}
