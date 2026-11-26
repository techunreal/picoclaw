package local

import (
	"context"
	"crypto/rand"
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
	RecipeID     string        `json:"recipe_id"`
	Name         string        `json:"name"`
	Language     string        `json:"language"`
	Dir          string        `json:"dir"`
	Venv         string        `json:"venv,omitempty"`
	Port         int           `json:"port"`
	PID          int           `json:"pid,omitempty"`
	Token        string        `json:"token,omitempty"`
	Voices       []VoiceOption `json:"voices"`
	DefaultVoice string        `json:"default_voice"`
	SampleRate   int           `json:"sample_rate"`
	InstalledAt  time.Time     `json:"installed_at"`
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
	root       string
	status     Status
	installing bool
	processes  map[string]*exec.Cmd
}

var (
	defaultOnce    sync.Once
	defaultManager *Manager
)

func Default() *Manager {
	defaultOnce.Do(func() {
		defaultManager = NewManager(filepath.Join(config.GetHome(), "tts"))
	})
	return defaultManager
}

func NewManager(root string) *Manager {
	return &Manager{
		root:      root,
		processes: make(map[string]*exec.Cmd),
	}
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
func (m *Manager) DetectPython() PythonInfo {
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
		out, err := exec.Command(exe, args...).Output()
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
		pip := exec.Command(pythonExe, "-m", "pip", "--version").Run() == nil
		venv := exec.Command(pythonExe, "-c", "import venv, ensurepip").Run() == nil
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
// through Status.
func (m *Manager) StartInstall(
	ctx context.Context,
	recipe Recipe,
	onDone func(*Installed, error),
) error {
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
		installed, err := m.install(installCtx, recipe)
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

func (m *Manager) install(ctx context.Context, recipe Recipe) (*Installed, error) {
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
	if err := downloadFile(ctx, hfFileURL(recipe.HFRepo, recipe.ModelFile), modelPath); err != nil {
		return nil, err
	}
	if err := downloadFile(ctx, hfFileURL(recipe.HFRepo, recipe.ConfigFile), configPath); err != nil {
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
		Dir:          m.modelDir(recipe.ID),
		Venv:         m.venvDir(),
		Port:         port,
		Token:        token,
		Voices:       append([]VoiceOption(nil), recipe.Voices...),
		DefaultVoice: recipe.DefaultVoice,
		SampleRate:   recipe.SampleRate,
		InstalledAt:  time.Now().UTC(),
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

func (m *Manager) ensurePythonEnv(ctx context.Context, python PythonInfo, recipe Recipe) error {
	venvPython := m.venvPython()
	if _, err := os.Stat(venvPython); err != nil {
		cmd := exec.CommandContext(ctx, python.Executable, "-m", "venv", m.venvDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("could not create Python venv: %w: %s", err, tail(string(out)))
		}
	}
	pip := func(args ...string) error {
		cmd := exec.CommandContext(ctx, venvPython, append([]string{"-m", "pip"}, args...)...)
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
	m.setStatus("python", "Installing Turkish phonemizer", 60)
	phonemizerDir, phonemizerRoot, err := m.downloadPhonemizer(ctx, recipe)
	if err != nil {
		return err
	}
	defer os.RemoveAll(phonemizerRoot)
	return pip("install", "--disable-pip-version-check", phonemizerDir)
}

func (m *Manager) downloadPhonemizer(ctx context.Context, recipe Recipe) (string, string, error) {
	tmpDir, err := os.MkdirTemp(m.root, "phonemizer-*")
	if err != nil {
		return "", "", err
	}
	extractDir := filepath.Join(tmpDir, "src")
	if err := downloadRepo(ctx, recipe.PhonemizerRepo, extractDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", "", err
	}
	if _, err := os.Stat(filepath.Join(extractDir, "pyproject.toml")); err == nil {
		return extractDir, tmpDir, nil
	}
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(extractDir, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
			return dir, tmpDir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "setup.py")); err == nil {
			return dir, tmpDir, nil
		}
	}
	os.RemoveAll(tmpDir)
	return "", "", errors.New("phonemizer package layout was not recognized")
}

type hfTreeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// downloadRepo copies a small Hugging Face repository through the tree API.
// It is used for Python packages that do not publish a downloadable archive.
func downloadRepo(ctx context.Context, repo, dst string) error {
	apiURL := "https://huggingface.co/api/models/" + repo + "/tree/main?recursive=true"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "picoclaw-local-tts/"+config.FormatVersion())
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("repository listing failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("repository listing failed: HTTP %d", resp.StatusCode)
	}
	var entries []hfTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return fmt.Errorf("repository listing failed: %w", err)
	}
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
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
	if cmd, ok := m.processes[id]; ok && cmd.Process != nil {
		_ = cmd.Process.Kill()
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
	if _, err := os.Stat(filepath.Join(installed.Dir, "onnx", "model.onnx")); err != nil {
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
	cmd := exec.Command(
		venvPython,
		m.serverPath(),
		"--model-dir", installed.Dir,
		"--port", strconv.Itoa(installed.Port),
		"--token", installed.Token,
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
	return resp.StatusCode == http.StatusOK
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
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("download failed: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
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
