package local

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

// PythonInfo describes the Python interpreter available for the local TTS
// sidecar.
type PythonInfo struct {
	Available  bool   `json:"available"`
	Executable string `json:"executable,omitempty"`
	Version    string `json:"version,omitempty"`
	Pip        bool   `json:"pip"`
	Venv       bool   `json:"venv"`
	Message    string `json:"message,omitempty"`
}

// VoiceOption is re-exported through the catalog for API consumers.
type Installed struct {
	RecipeID          string        `json:"recipe_id"`
	Name              string        `json:"name"`
	Language          string        `json:"language"`
	Adapter           string        `json:"adapter,omitempty"`
	Dir               string        `json:"dir"`
	Venv              string        `json:"venv,omitempty"`
	Port              int           `json:"port"`
	PID               int           `json:"pid,omitempty"`
	Token             string        `json:"token,omitempty"`
	Voices            []VoiceOption `json:"voices"`
	DefaultVoice      string        `json:"default_voice"`
	SampleRate        int           `json:"sample_rate"`
	ThirdParty        bool          `json:"third_party,omitempty"`
	LicenseAcceptedAt *time.Time    `json:"license_accepted_at,omitempty"`
	InstalledAt       time.Time     `json:"installed_at"`
}

// Status is the installation status shown in the UI.
type Status struct {
	Installing bool       `json:"installing"`
	Phase      string     `json:"phase"`
	Message    string     `json:"message"`
	Progress   int        `json:"progress"`
	Error      string     `json:"error,omitempty"`
	Done       bool       `json:"done"`
	Model      *Installed `json:"model,omitempty"`
}

// Manager installs and runs local TTS sidecars below ~/.picoclaw/tts.
type Manager struct {
	mu         sync.Mutex
	pythonMu   sync.Mutex
	depMu      sync.Mutex
	root       string
	status     Status
	installing bool
	processes  map[string]*exec.Cmd
	pythonInfo PythonInfo
	pythonAt   time.Time
	deps       map[string]Dependency
	depsAt     time.Time
}

var (
	defaultMu      sync.Mutex
	defaultManager *Manager
)

func Default() *Manager {
	root := filepath.Join(config.GetHome(), "tts")
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultManager == nil || defaultManager.root != root {
		defaultManager = NewManager(root)
	}
	return defaultManager
}

func NewManager(root string) *Manager {
	return &Manager{
		root:      root,
		processes: make(map[string]*exec.Cmd),
	}
}

// Dependency is the availability of an external tool a recipe needs.
type Dependency struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (m *Manager) manifestPath() string { return filepath.Join(m.root, "manifest.json") }

// Catalog returns the built-in catalog merged with the user manifest. A broken
// user manifest is reported without hiding the built-in models.
func (m *Manager) Catalog() ([]Recipe, error) {
	builtin, err := BuiltinManifest()
	if err != nil {
		return nil, err
	}
	recipes := append([]Recipe(nil), builtin.Models...)
	user, userErr := m.userRecipes()
	recipes = append(recipes, user...)
	return recipes, userErr
}

func (m *Manager) userRecipes() ([]Recipe, error) {
	data, err := os.ReadFile(m.manifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read the user manifest: %w", err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("user manifest: %w", err)
	}
	return manifest.Models, nil
}

// UserManifestJSON returns the saved user manifest as pretty JSON.
func (m *Manager) UserManifestJSON() (string, error) {
	data, err := os.ReadFile(m.manifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// SaveUserManifest validates and stores a user manifest.
func (m *Manager) SaveUserManifest(raw []byte) (Manifest, error) {
	manifest, err := ParseManifest(raw)
	if err != nil {
		return Manifest{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return Manifest{}, err
	}
	tmp := m.manifestPath() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return Manifest{}, err
	}
	if err := os.Rename(tmp, m.manifestPath()); err != nil {
		_ = os.Remove(tmp)
		return Manifest{}, err
	}
	return manifest, nil
}

// ClearUserManifest removes the user manifest.
func (m *Manager) ClearUserManifest() error {
	err := os.Remove(m.manifestPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ResolveRecipe resolves a model against the built-in and user catalogs.
func (m *Manager) ResolveRecipe(input string) (*Recipe, error) {
	recipes, err := m.Catalog()
	if err != nil && len(recipes) == 0 {
		return nil, err
	}
	return ResolveRecipeIn(recipes, input)
}

// Dependencies reports whether the external tools required by a recipe exist.
// The result is cached briefly so opening the TTS tab does not start a process
// on every query refresh.
func (m *Manager) Dependencies(names []string) []Dependency {
	out := make([]Dependency, 0, len(names))
	if len(names) == 0 {
		return out
	}
	m.depMu.Lock()
	defer m.depMu.Unlock()
	if m.deps == nil || time.Since(m.depsAt) > time.Minute {
		m.deps = make(map[string]Dependency)
		m.depsAt = time.Now()
	}
	for _, name := range names {
		cached, ok := m.deps[name]
		if !ok {
			cached = detectDependency(name)
			m.deps[name] = cached
		}
		out = append(out, cached)
	}
	return out
}

func detectDependency(name string) Dependency {
	result := Dependency{Name: name}
	if name != "espeak-ng" {
		result.Message = "unsupported dependency"
		return result
	}
	path, err := exec.LookPath("espeak-ng")
	if err != nil {
		result.Message = "espeak-ng is not installed. Install eSpeak NG 1.52.0 and try again."
		return result
	}
	result.Available = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	hideWindow(cmd)
	if out, err := cmd.Output(); err == nil {
		version := strings.TrimSpace(string(out))
		if line, _, found := strings.Cut(version, "\n"); found {
			version = line
		}
		result.Version = version
	}
	return result
}

func (m *Manager) missingDependencies(names []string) []string {
	var missing []string
	for _, dependency := range m.Dependencies(names) {
		if !dependency.Available {
			missing = append(missing, dependency.Name)
		}
	}
	return missing
}

func (m *Manager) venvDir() string    { return filepath.Join(m.root, "venv") }
func (m *Manager) serverPath() string { return filepath.Join(m.root, "server.py") }
func (m *Manager) logsDir() string    { return filepath.Join(m.root, "logs") }
func (m *Manager) modelDir(id string) string {
	return filepath.Join(m.root, "models", filepath.Base(strings.TrimSpace(id)))
}
func (m *Manager) registryPath() string { return filepath.Join(m.root, "registry.json") }
func (m *Manager) logPath(id string) string {
	return filepath.Join(m.logsDir(), filepath.Base(strings.TrimSpace(id))+".log")
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) Installed() []Installed {
	registry := m.loadRegistry()
	out := make([]Installed, 0, len(registry))
	for _, item := range registry {
		out = append(out, item)
	}
	return out
}

func (m *Manager) InstalledByID(id string) (Installed, bool) {
	registry := m.loadRegistry()
	item, ok := registry[id]
	return item, ok
}

// Healthy reports whether the sidecar for an installed model answers /health.
func (m *Manager) Healthy(installed Installed) bool {
	return m.healthy(installed)
}

// DetectPython finds a system Python with pip and venv support. It never
// installs Python; the UI asks the user before any environment is created.
// The result is cached briefly so opening the TTS tab does not spawn console
// processes on every query refresh.
func (m *Manager) DetectPython() PythonInfo {
	m.pythonMu.Lock()
	defer m.pythonMu.Unlock()
	if !m.pythonAt.IsZero() && time.Since(m.pythonAt) < time.Minute {
		return m.pythonInfo
	}
	m.pythonInfo = detectPython()
	m.pythonAt = time.Now()
	return m.pythonInfo
}

func detectPython() PythonInfo {
	candidates := [][]string{
		{"py", "-3"},
		{"python3"},
		{"python"},
	}
	var lastMessage string
	for _, candidate := range candidates {
		exe, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		args := append([]string{}, candidate[1:]...)
		args = append(args, "-c",
			"import sys; print(sys.executable); print('%d.%d' % sys.version_info[:2])")
		out, err := hiddenCommand(exe, args...).Output()
		if err != nil {
			lastMessage = err.Error()
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) < 2 {
			continue
		}
		pythonExe := strings.TrimSpace(lines[0])
		version := strings.TrimSpace(lines[1])
		parts := strings.Split(version, ".")
		if len(parts) < 2 {
			continue
		}
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		if major < 3 || (major == 3 && minor < 10) {
			lastMessage = fmt.Sprintf("Python %s is too old", version)
			continue
		}
		pip := hiddenCommand(pythonExe, "-m", "pip", "--version").Run() == nil
		venv := hiddenCommand(pythonExe, "-c", "import venv, ensurepip").Run() == nil
		return PythonInfo{
			Available:  pip && venv,
			Executable: pythonExe,
			Version:    version,
			Pip:        pip,
			Venv:       venv,
			Message:    pythonAvailabilityMessage(pip, venv),
		}
	}
	if lastMessage == "" {
		lastMessage = "Python 3.10+ was not found."
	}
	return PythonInfo{Message: lastMessage}
}

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	return cmd
}

func pythonAvailabilityMessage(pip, venv bool) string {
	if pip && venv {
		return ""
	}
	var missing []string
	if !pip {
		missing = append(missing, "pip")
	}
	if !venv {
		missing = append(missing, "venv")
	}
	return "Python is missing: " + strings.Join(missing, ", ")
}

// StartInstall runs the installation in the background and reports progress
// through Status. licenseAt is recorded for third-party models after the user
// accepted the license.
func (m *Manager) StartInstall(
	ctx context.Context,
	recipe Recipe,
	licenseAt time.Time,
	onDone func(*Installed, error),
) error {
	if missing := m.missingDependencies(recipe.Requires); len(missing) > 0 {
		return fmt.Errorf("missing dependency: %s", strings.Join(missing, ", "))
	}
	m.mu.Lock()
	if m.installing {
		m.mu.Unlock()
		return errors.New("a local TTS installation is already running")
	}
	m.installing = true
	m.status = Status{Installing: true, Phase: "starting", Message: "Preparing installation", Progress: 0}
	m.mu.Unlock()

	go func() {
		installCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Minute)
		defer cancel()
		installed, err := m.install(installCtx, recipe, licenseAt)
		if onDone != nil {
			onDone(installed, err)
		}
		m.mu.Lock()
		m.installing = false
		m.status.Installing = false
		m.status.Done = err == nil
		if err != nil {
			m.status.Phase = "error"
			m.status.Message = "Installation failed"
			m.status.Error = err.Error()
		} else {
			m.status.Phase = "ready"
			m.status.Message = "Model installed"
			m.status.Progress = 100
			m.status.Model = installed
		}
		m.mu.Unlock()
	}()
	return nil
}

func (m *Manager) setStatus(phase, message string, progress int) {
	m.mu.Lock()
	m.status.Phase = phase
	m.status.Message = message
	m.status.Progress = progress
	m.mu.Unlock()
}

func (m *Manager) install(ctx context.Context, recipe Recipe, licenseAt time.Time) (*Installed, error) {
	python := m.DetectPython()
	if !python.Available {
		return nil, fmt.Errorf("Python 3.10+ with pip and venv is required: %s", python.Message)
	}
	if err := os.MkdirAll(m.modelDir(recipe.ID), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.logsDir(), 0o755); err != nil {
		return nil, err
	}

	modelPath := filepath.Join(m.modelDir(recipe.ID), filepath.FromSlash(recipe.ModelFile))
	configPath := filepath.Join(m.modelDir(recipe.ID), filepath.FromSlash(recipe.ConfigFile))
	m.setStatus("download", "Downloading model files", 5)
	if err := downloadFileChecked(ctx, hfFileURL(recipe.HFRepo, recipe.ModelFile), modelPath, recipe.Checksums[recipe.ModelFile]); err != nil {
		return nil, err
	}
	if err := downloadFileChecked(ctx, hfFileURL(recipe.HFRepo, recipe.ConfigFile), configPath, recipe.Checksums[recipe.ConfigFile]); err != nil {
		return nil, err
	}
	if err := m.downloadExtras(ctx, recipe); err != nil {
		return nil, err
	}

	m.setStatus("python", "Preparing Python environment", 35)
	if err := m.ensurePythonEnv(ctx, python, recipe); err != nil {
		return nil, err
	}
	m.setStatus("server", "Writing local TTS server", 75)
	if err := writeServerScript(m.serverPath()); err != nil {
		return nil, err
	}
	if recipe.Frontend != nil {
		if err := writeLocalProfile(m.modelDir(recipe.ID), recipe); err != nil {
			return nil, err
		}
	}

	port, err := freePort()
	if err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	installed := &Installed{
		RecipeID:     recipe.ID,
		Name:         recipe.Name,
		Language:     recipe.Language,
		Adapter:      recipe.Adapter,
		Dir:          m.modelDir(recipe.ID),
		Venv:         m.venvDir(),
		Port:         port,
		Token:        token,
		Voices:       append([]VoiceOption(nil), recipe.Voices...),
		DefaultVoice: recipe.DefaultVoice,
		SampleRate:   recipe.SampleRate,
		ThirdParty:   recipe.ThirdParty,
		InstalledAt:  time.Now().UTC(),
	}
	if recipe.ThirdParty && !licenseAt.IsZero() {
		accepted := licenseAt.UTC()
		installed.LicenseAcceptedAt = &accepted
	}
	m.setStatus("start", "Starting local TTS model", 85)
	if err := m.startLocked(installed); err != nil {
		return nil, err
	}
	if err := m.saveRegistry(*installed); err != nil {
		return nil, err
	}
	m.setStatus("ready", "Model installed", 100)
	return installed, nil
}

func writeLocalProfile(dir string, recipe Recipe) error {
	profile := map[string]any{
		"recipe_id":     recipe.ID,
		"adapter":       recipe.Adapter,
		"model_file":    recipe.ModelFile,
		"config_file":   recipe.ConfigFile,
		"sample_rate":   recipe.SampleRate,
		"max_tokens":    recipe.MaxTokens,
		"voices":        recipe.Voices,
		"default_voice": recipe.DefaultVoice,
		"language":      recipe.Language,
		"frontend":      recipe.Frontend,
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "picoclaw.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type localProfile struct {
	ModelFile string `json:"model_file"`
}

func localModelFile(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "picoclaw.json"))
	if err != nil {
		return ""
	}
	var profile localProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return ""
	}
	return strings.TrimSpace(profile.ModelFile)
}

func (m *Manager) ensurePythonEnv(ctx context.Context, python PythonInfo, recipe Recipe) error {
	venvPython := m.venvPython()
	if _, err := os.Stat(venvPython); err != nil {
		cmd := exec.CommandContext(ctx, python.Executable, "-m", "venv", m.venvDir())
		hideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("could not create Python venv: %w: %s", err, tail(string(out)))
		}
	}
	pip := func(args ...string) error {
		cmd := exec.CommandContext(ctx, venvPython, append([]string{"-m", "pip"}, args...)...)
		hideWindow(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("pip %s failed: %w: %s", strings.Join(args, " "), err, tail(string(out)))
		}
		return nil
	}
	m.setStatus("python", "Installing Python packages", 45)
	if err := pip("install", "--disable-pip-version-check", "onnxruntime", "numpy"); err != nil {
		return err
	}
	if strings.TrimSpace(recipe.PhonemizerSource) == "" {
		return nil
	}
	m.setStatus("python", "Installing phonemizer", 60)
	phonemizerDir, phonemizerRoot, err := m.downloadPhonemizerPackage(ctx, recipe.PhonemizerSource)
	if err != nil {
		return err
	}
	defer os.RemoveAll(phonemizerRoot)
	return pip("install", "--disable-pip-version-check", phonemizerDir)
}

func (m *Manager) downloadExtras(ctx context.Context, recipe Recipe) error {
	if len(recipe.ExtraPrefixes) == 0 {
		return nil
	}
	return downloadRepoFiltered(ctx, recipe.HFRepo, m.modelDir(recipe.ID), recipe.ExtraPrefixes)
}

func (m *Manager) downloadPhonemizerPackage(ctx context.Context, source string) (string, string, error) {
	kind, ref, ok := strings.Cut(strings.TrimSpace(source), ":")
	if !ok || strings.TrimSpace(ref) == "" {
		return "", "", fmt.Errorf("invalid phonemizer source %q", source)
	}
	tmpDir, err := os.MkdirTemp(m.root, "phonemizer-*")
	if err != nil {
		return "", "", err
	}
	extractDir := filepath.Join(tmpDir, "src")
	switch kind {
	case "hf":
		if err := downloadRepo(ctx, ref, extractDir); err != nil {
			os.RemoveAll(tmpDir)
			return "", "", err
		}
	case "github":
		zipPath := filepath.Join(tmpDir, "package.zip")
		url := "https://github.com/" + ref + "/archive/refs/heads/main.zip"
		if err := downloadFile(ctx, url, zipPath); err != nil {
			os.RemoveAll(tmpDir)
			return "", "", err
		}
		if err := unzip(zipPath, extractDir); err != nil {
			os.RemoveAll(tmpDir)
			return "", "", err
		}
	default:
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("unsupported phonemizer source %q", source)
	}
	if dir, ok := findPackageDir(extractDir); ok {
		return dir, tmpDir, nil
	}
	os.RemoveAll(tmpDir)
	return "", "", errors.New("phonemizer package layout was not recognized")
}

func findPackageDir(root string) (string, bool) {
	if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); err == nil {
		return root, true
	}
	if _, err := os.Stat(filepath.Join(root, "setup.py")); err == nil {
		return root, true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
			return dir, true
		}
		if _, err := os.Stat(filepath.Join(dir, "setup.py")); err == nil {
			return dir, true
		}
	}
	return "", false
}

type hfTreeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func listHFRepo(ctx context.Context, repo string) ([]hfTreeEntry, error) {
	apiURL := "https://huggingface.co/api/models/" + repo + "/tree/main?recursive=true"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "picoclaw-local-tts/"+config.FormatVersion())
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("repository listing failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repository listing failed: HTTP %d", resp.StatusCode)
	}
	var entries []hfTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("repository listing failed: %w", err)
	}
	return entries, nil
}

// downloadRepo copies a small Hugging Face repository through the tree API.
// It is used for Python packages that do not publish a downloadable archive.
func downloadRepo(ctx context.Context, repo, dst string) error {
	return downloadRepoFiltered(ctx, repo, dst, nil)
}

func downloadRepoFiltered(ctx context.Context, repo, dst string, prefixes []string) error {
	entries, err := listHFRepo(ctx, repo)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		if len(prefixes) > 0 {
			match := false
			for _, prefix := range prefixes {
				if strings.HasPrefix(entry.Path, prefix) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		rel := filepath.FromSlash(entry.Path)
		if strings.HasPrefix(rel, ".") || strings.Contains(rel, "..") {
			continue
		}
		target := filepath.Join(dst, rel)
		if err := downloadFile(ctx, hfFileURL(repo, entry.Path), target); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) venvPython() string {
	if isWindows() {
		return filepath.Join(m.venvDir(), "Scripts", "python.exe")
	}
	return filepath.Join(m.venvDir(), "bin", "python")
}

const sidecarVersion = "3"

// EnsureRunning starts the sidecar for an installed model when needed.
func (m *Manager) EnsureRunning(id string) error {
	installed, ok := m.InstalledByID(id)
	if !ok {
		return fmt.Errorf("local TTS model %q is not installed", id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := writeServerScript(m.serverPath()); err != nil {
		return err
	}
	if m.healthy(installed) {
		return nil
	}
	if installed.PID > 0 {
		killProcessTree(installed.PID)
		installed.PID = 0
		_ = m.saveRegistry(installed)
		time.Sleep(300 * time.Millisecond)
	}
	if cmd, ok := m.processes[id]; ok && cmd.Process != nil {
		killProcessTree(cmd.Process.Pid)
		delete(m.processes, id)
	}
	if err := m.startLocked(&installed); err != nil {
		return err
	}
	return nil
}

func (m *Manager) startLocked(installed *Installed) error {
	if m.healthy(*installed) {
		return nil
	}
	if _, err := os.Stat(m.serverPath()); err != nil {
		return fmt.Errorf("local TTS server script is missing: %w", err)
	}
	modelRel := localModelFile(installed.Dir)
	if modelRel == "" {
		modelRel = "onnx/model.onnx"
	}
	if _, err := os.Stat(filepath.Join(installed.Dir, filepath.FromSlash(modelRel))); err != nil {
		return fmt.Errorf("model file is missing: %w", err)
	}
	venvPython := m.venvPython()
	if _, err := os.Stat(venvPython); err != nil {
		return fmt.Errorf("Python environment is missing: %w", err)
	}
	logFile, err := os.OpenFile(m.logPath(installed.RecipeID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	adapter := strings.TrimSpace(installed.Adapter)
	if adapter == "" {
		adapter = "clarvia"
	}
	cmd := exec.Command(
		venvPython,
		m.serverPath(),
		"--model-dir", installed.Dir,
		"--port", strconv.Itoa(installed.Port),
		"--token", installed.Token,
		"--adapter", adapter,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
		m.mu.Lock()
		if m.processes[installed.RecipeID] == cmd {
			delete(m.processes, installed.RecipeID)
		}
		m.mu.Unlock()
	}()
	m.processes[installed.RecipeID] = cmd
	installed.PID = cmd.Process.Pid
	if err := m.saveRegistry(*installed); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if m.healthy(*installed) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("local TTS server did not become ready; see %s", m.logPath(installed.RecipeID))
}

func (m *Manager) healthy(installed Installed) bool {
	if installed.Port <= 0 {
		return false
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/health", installed.Port), nil)
	if err != nil {
		return false
	}
	if installed.Token != "" {
		req.Header.Set("Authorization", "Bearer "+installed.Token)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var payload struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false
	}
	return payload.Status == "ok" && payload.Version == sidecarVersion
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cmd, ok := m.processes[id]; ok && cmd.Process != nil {
		killProcessTree(cmd.Process.Pid)
		delete(m.processes, id)
	}
	registry := m.loadRegistry()
	if installed, ok := registry[id]; ok {
		killProcessTree(installed.PID)
		installed.PID = 0
		registry[id] = installed
		_ = m.writeRegistry(registry)
	}
}

func (m *Manager) Uninstall(id string) error {
	m.Stop(id)
	registry := m.loadRegistry()
	if _, ok := registry[id]; !ok {
		return fmt.Errorf("local TTS model %q is not installed", id)
	}
	if err := os.RemoveAll(m.modelDir(id)); err != nil {
		return err
	}
	delete(registry, id)
	return m.writeRegistry(registry)
}

func (m *Manager) loadRegistry() map[string]Installed {
	data, err := os.ReadFile(m.registryPath())
	if err != nil {
		return map[string]Installed{}
	}
	var registry map[string]Installed
	if err := json.Unmarshal(data, &registry); err != nil || registry == nil {
		return map[string]Installed{}
	}
	return registry
}

func (m *Manager) saveRegistry(installed Installed) error {
	registry := m.loadRegistry()
	registry[installed.RecipeID] = installed
	return m.writeRegistry(registry)
}

func (m *Manager) writeRegistry(registry map[string]Installed) error {
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.registryPath(), data, 0o644)
}

func hfFileURL(repo, file string) string {
	return "https://huggingface.co/" + repo + "/resolve/main/" + file + "?download=true"
}

func downloadFile(ctx context.Context, url, dst string) error {
	return downloadFileChecked(ctx, url, dst, "")
}

func downloadFileChecked(ctx context.Context, url, dst, wantSHA256 string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "picoclaw-local-tts/"+config.FormatVersion())
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d from %s", resp.StatusCode, url)
	}
	tmp := dst + ".part"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	writer := io.Writer(file)
	if strings.TrimSpace(wantSHA256) != "" {
		writer = io.MultiWriter(file, hasher)
	}
	if _, err := io.Copy(writer, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("download failed: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if strings.TrimSpace(wantSHA256) != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(got, strings.TrimSpace(wantSHA256)) {
			_ = os.Remove(tmp)
			return fmt.Errorf("checksum mismatch for %s: got %s", filepath.Base(dst), got)
		}
	}
	return os.Rename(tmp, dst)
}

func unzip(src, dst string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()
	base := filepath.Clean(dst)
	for _, file := range reader.File {
		target := filepath.Join(base, filepath.FromSlash(file.Name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), base+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe ZIP path %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func tail(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 2000 {
		return text
	}
	return text[len(text)-2000:]
}

func writeServerScript(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(serverScript), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
