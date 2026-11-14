// Package browser wraps the agent-browser CLI so the agent can drive a real
// browser without handing it an unrestricted shell.
package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrBinaryNotFound is returned when agent-browser cannot be located.
	ErrBinaryNotFound = errors.New("agent-browser executable not found")
	// ErrNoAllowedDomains is returned when the allowlist is empty.
	ErrNoAllowedDomains = errors.New("browser allowlist is empty")
	// ErrDomainNotAllowed is returned for URLs outside the allowlist.
	ErrDomainNotAllowed = errors.New("domain is not in the browser allowlist")
)

// Options configures a Client.
type Options struct {
	BinaryPath      string
	BrowserPath     string
	Headless        bool
	AllowedDomains  []string
	AllowAllDomains bool
	MaxOutput       int
	Timeout         time.Duration
	// StateDir stores the generated action policy file. Defaults to a temp
	// directory under the OS temp dir when empty.
	StateDir string
}

// Client runs agent-browser commands for one process. Each call connects to the
// per-session agent-browser daemon that keeps the browser open between calls.
type Client struct {
	binary      string
	browserPath string
	headless    bool
	domains     []string
	allowAll    bool
	maxOutput   int
	timeout     time.Duration
	policyPath  string
	proxyURL    string
}

// New resolves the executable and prepares the action policy.
func New(opts Options) (*Client, error) {
	if !opts.AllowAllDomains && len(opts.AllowedDomains) == 0 {
		return nil, ErrNoAllowedDomains
	}

	binary, err := resolveBinary(opts.BinaryPath)
	if err != nil {
		return nil, err
	}

	maxOutput := opts.MaxOutput
	if maxOutput <= 0 {
		maxOutput = 20000
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	stateDir := opts.StateDir
	if stateDir == "" {
		stateDir = filepath.Join(os.TempDir(), "picoclaw_browser")
	}
	policyPath, err := ensureActionPolicy(stateDir)
	if err != nil {
		return nil, err
	}

	proxy, err := sharedAllowlistProxy()
	if err != nil {
		return nil, err
	}
	proxy.setAllowlist(opts.AllowedDomains, opts.AllowAllDomains)

	return &Client{
		binary:      binary,
		browserPath: resolveBrowser(opts.BrowserPath),
		headless:    opts.Headless,
		domains:     normalizeDomainPatterns(opts.AllowedDomains),
		allowAll:    opts.AllowAllDomains,
		maxOutput:   maxOutput,
		timeout:     timeout,
		policyPath:  policyPath,
		proxyURL:    proxy.proxyURL(),
	}, nil
}

// BinaryPath returns the resolved agent-browser executable.
func (c *Client) BinaryPath() string { return c.binary }

// BrowserPath returns the detected Chrome/Edge executable, or empty when
// agent-browser should use its own browser.
func (c *Client) BrowserPath() string { return c.browserPath }

// SessionName derives a stable agent-browser session name from the chat.
func SessionName(channel, chatID string) string {
	raw := strings.TrimSpace(channel) + ":" + strings.TrimSpace(chatID)
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, raw)
	sanitized = strings.Trim(sanitized, "-_")
	if utf8.RuneCountInString(sanitized) > 40 {
		sanitized = string([]rune(sanitized)[:40])
	}
	if sanitized == "" {
		sanitized = "local"
	}
	return "pc-" + sanitized + "-" + shortHash(raw)
}

// Run executes one agent-browser command inside a session.
func (c *Client) Run(ctx context.Context, session string, argv ...string) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("browser command is empty")
	}

	args := []string{
		"--session", session,
		"--max-output", strconv.Itoa(c.maxOutput),
		"--content-boundaries",
		"--action-policy", c.policyPath,
		"--args", "--proxy-bypass-list=<-loopback>,--disable-quic,--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
	}
	if c.browserPath != "" {
		args = append(args, "--executable-path", c.browserPath)
	}
	if !c.headless {
		args = append(args, "--headed")
	}
	args = append(args, argv...)

	runCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, c.binary, args...)
	cmd.Env = append(os.Environ(),
		"AGENT_BROWSER_PROXY="+c.proxyURL,
		"AGENT_BROWSER_PROXY_USERNAME=",
		"AGENT_BROWSER_PROXY_PASSWORD=",
	)
	output, err := cmd.CombinedOutput()
	text := truncateOutput(string(output), c.maxOutput)
	if runCtx.Err() != nil {
		return text, fmt.Errorf("browser command timed out after %s", c.timeout)
	}
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("browser command failed: %s", firstLine(text))
		}
		return text, fmt.Errorf("browser command failed: %w", err)
	}
	return text, nil
}

// ValidateURL checks the scheme and the domain allowlist before navigation.
func (c *Client) ValidateURL(raw string) error {
	return validateURL(raw, c.domains, c.allowAll)
}

// Close releases the browser session used by this chat.
func (c *Client) Close(ctx context.Context, session string) (string, error) {
	return c.Run(ctx, session, "close")
}

func ensureActionPolicy(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "action-policy.json")
	policy := `{"default":"deny","allow":["navigate","click","fill","interact","snapshot","scroll","wait","get","read","close"]}` + "\n"
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == policy {
		return path, nil
	}
	if err := os.WriteFile(path, []byte(policy), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

var (
	windowsBinaryPaths = []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	unixBrowserPaths = []string{
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	}
)

func resolveBrowser(explicit string) string {
	if explicit != "" {
		if info, err := os.Stat(explicit); err == nil && !info.IsDir() {
			return explicit
		}
	}
	candidates := unixBrowserPaths
	if runtime.GOOS == "windows" {
		candidates = windowsBinaryPaths
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// LocateBinary resolves the agent-browser executable without creating a client.
func LocateBinary(explicit string) (string, error) {
	return resolveBinary(explicit)
}

func resolveBinary(explicit string) (string, error) {
	candidates := make([]string, 0, 6)
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "agent-browser.exe"),
			filepath.Join(dir, "agent-browser"),
		)
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates,
				filepath.Join(appData, "npm", "node_modules", "agent-browser", "bin", "agent-browser-win32-x64.exe"),
			)
		}
	}
	if path, err := exec.LookPath("agent-browser.exe"); err == nil {
		candidates = append(candidates, path)
	}
	if path, err := exec.LookPath("agent-browser"); err == nil {
		candidates = append(candidates, path)
	}

	for _, candidate := range candidates {
		resolved, ok := usableBinary(candidate)
		if ok {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("%w: install agent-browser or set tools.browser.binary_path", ErrBinaryNotFound)
}

func usableBinary(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case "", ".exe":
		return path, true
	case ".cmd", ".bat":
		if target := resolveWindowsShim(path); target != "" {
			return target, true
		}
		return path, true
	}
	return "", false
}

var shimExePattern = regexp.MustCompile(`"([^"]+\.exe)"`)

// resolveWindowsShim reads an npm .cmd shim and returns the real executable it
// launches. Avoids cmd.exe quoting issues for agent text containing spaces.
func resolveWindowsShim(shimPath string) string {
	data, err := os.ReadFile(shimPath)
	if err != nil {
		return ""
	}
	match := shimExePattern.FindStringSubmatch(string(data))
	if len(match) < 2 {
		return ""
	}
	target := match[1]
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(shimPath), target)
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return ""
	}
	return target
}

func truncateOutput(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit > 0 && len(text) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		return strings.TrimSpace(text[:cut]) + "\n...[truncated]"
	}
	return text
}

func firstLine(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return text[:idx]
	}
	return text
}
