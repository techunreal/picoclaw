package local

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	espeakToolVersion = "1.52.0"
	espeakMSIURL      = "https://github.com/espeak-ng/espeak-ng/releases/download/1.52.0/espeak-ng.msi"
	espeakMSISHA256   = "7F673C709EA5DD579D3B5EBB98688CC575328A6AB7438D2BC405B88CEDAEAFB9"
	espeakMSISize     = int64(12765862)
	espeakLicense     = "GPL-3.0"
)

func (m *Manager) toolsDir() string  { return filepath.Join(m.root, "tools") }
func (m *Manager) espeakDir() string { return filepath.Join(m.toolsDir(), "espeak-ng") }

// findEspeakExe looks for the espeak-ng executable below root. The MSI
// administrative install creates an "eSpeak NG" folder, so the exact depth is
// not fixed.
func findEspeakExe(root string) string {
	found := ""
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "espeak-ng.exe") {
			found = path
		}
		return nil
	})
	return found
}

func espeakDataDir(exe string) string {
	return filepath.Join(filepath.Dir(exe), "espeak-ng-data")
}

func espeakEnv(exe string) []string {
	env := os.Environ()
	if data := espeakDataDir(exe); data != "" {
		env = append(env, "ESPEAK_DATA_PATH="+data)
	}
	return env
}

// espeakAvailable returns the executable to use and whether it belongs to the
// managed download under ~/.picoclaw/tts/tools.
func (m *Manager) espeakAvailable() (string, bool) {
	if exe := findEspeakExe(m.espeakDir()); exe != "" {
		return exe, true
	}
	if exe, err := exec.LookPath("espeak-ng"); err == nil {
		return exe, false
	}
	return "", false
}

func espeakVersion(ctx context.Context, exe string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, "--version")
	cmd.Env = espeakEnv(exe)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(out))
	if line, _, found := strings.Cut(version, "\n"); found {
		version = line
	}
	return version, nil
}

// EnsureEspeak downloads eSpeak NG 1.52.0 into the local tools directory after
// the user accepted the GPL-3.0 license. It never installs system-wide.
func (m *Manager) EnsureEspeak(ctx context.Context, accepted bool) (string, error) {
	if exe, _ := m.espeakAvailable(); exe != "" {
		return exe, nil
	}
	if !accepted {
		return "", fmt.Errorf("%s %s must be accepted before downloading eSpeak NG", espeakLicense, espeakToolVersion)
	}
	if !isWindows() {
		return "", fmt.Errorf("automatic eSpeak NG download is only available on Windows; install espeak-ng %s manually", espeakToolVersion)
	}

	m.toolMu.Lock()
	defer m.toolMu.Unlock()
	if exe := findEspeakExe(m.espeakDir()); exe != "" {
		return exe, nil
	}
	if err := os.MkdirAll(m.toolsDir(), 0o755); err != nil {
		return "", err
	}
	msi := filepath.Join(m.toolsDir(), "espeak-ng-"+espeakToolVersion+".msi")
	if err := downloadFileChecked(ctx, espeakMSIURL, msi, espeakMSISHA256); err != nil {
		return "", fmt.Errorf("could not download eSpeak NG: %w", err)
	}
	defer os.Remove(msi)

	target := m.espeakDir()
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	extractCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	shortMSI := shortPath(msi)
	shortTarget := shortPath(target)
	if strings.Contains(shortMSI, " ") || strings.Contains(shortTarget, " ") {
		return "", fmt.Errorf("could not create a space-free path for eSpeak NG; move PicoClaw to a folder without spaces")
	}
	cmd := exec.CommandContext(extractCtx, "msiexec.exe", "/a", shortMSI, "/qn", "TARGETDIR="+shortTarget)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("could not extract eSpeak NG: %w: %s", err, tail(string(out)))
	}
	_ = os.Remove(filepath.Join(target, "espeak-ng.msi"))

	exe := findEspeakExe(target)
	if exe == "" {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("eSpeak NG archive did not contain espeak-ng.exe")
	}
	version, err := espeakVersion(ctx, exe)
	if err != nil {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("eSpeak NG did not start: %w", err)
	}
	if !strings.Contains(version, espeakToolVersion) {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("eSpeak NG version mismatch: got %q, want %s", version, espeakToolVersion)
	}
	m.depMu.Lock()
	m.deps = nil
	m.depsAt = time.Time{}
	m.depMu.Unlock()
	return exe, nil
}

func (m *Manager) missingDependencyNames(names []string) []string {
	var missing []string
	for _, dependency := range m.Dependencies(names) {
		if !dependency.Available {
			missing = append(missing, dependency.Name)
		}
	}
	return missing
}
