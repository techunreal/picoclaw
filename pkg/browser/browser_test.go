package browser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeDomainPattern(t *testing.T) {
	tests := map[string]string{
		"example.com":               "example.com",
		" Example.COM ":             "example.com",
		"https://example.com/path":  "example.com",
		"http://example.com:8080/x": "example.com",
		"*.example.com":             "*.example.com",
		"example.com.":              "example.com",
	}
	for input, want := range tests {
		if got := NormalizeDomainPattern(input); got != want {
			t.Fatalf("NormalizeDomainPattern(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHostAllowed(t *testing.T) {
	patterns := []string{"example.com", "*.github.com", "localhost"}

	allowed := []string{"example.com", "EXAMPLE.com", "api.github.com", "github.com", "localhost"}
	for _, host := range allowed {
		if !HostAllowed(host, patterns) {
			t.Fatalf("HostAllowed(%q) = false, want true", host)
		}
	}

	blocked := []string{"evil.com", "notexample.com", "github.com.evil.com", ""}
	for _, host := range blocked {
		if HostAllowed(host, patterns) {
			t.Fatalf("HostAllowed(%q) = true, want false", host)
		}
	}
}

func TestValidateURL(t *testing.T) {
	patterns := []string{"example.com", "*.example.com"}

	valid := []string{
		"https://example.com/",
		"http://www.example.com/path?q=1",
		"https://api.example.com:8443/x",
	}
	for _, raw := range valid {
		if err := validateURL(raw, patterns, false); err != nil {
			t.Fatalf("validateURL(%q) error = %v", raw, err)
		}
	}

	invalid := []string{
		"",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"data:text/html,hi",
		"https://user:pass@example.com/",
		"https://evil.com/",
		"http://notexample.com/",
	}
	for _, raw := range invalid {
		if err := validateURL(raw, patterns, false); err == nil {
			t.Fatalf("validateURL(%q) = nil, want error", raw)
		}
	}

	if err := validateURL("https://evil.com/", nil, true); err != nil {
		t.Fatalf("allow-all validateURL error = %v", err)
	}
}

func TestSessionName(t *testing.T) {
	name := SessionName("pico", "pico:session with spaces/and stuff")
	if !strings.HasPrefix(name, "pc-pico-pico-session-with-spaces-and-stu") {
		t.Fatalf("SessionName() = %q, want sanitized prefix", name)
	}
	if strings.ContainsAny(name, " :/") {
		t.Fatalf("SessionName() = %q, want no spaces or separators", name)
	}
	if SessionName("", "") != SessionName("", "") {
		t.Fatal("SessionName() should be stable")
	}
	if SessionName("a", "b") == SessionName("a", "c") {
		t.Fatal("different chats should produce different session names")
	}
}

func TestNew_RequiresAllowlist(t *testing.T) {
	_, err := New(Options{BinaryPath: "missing"})
	if !errors.Is(err, ErrNoAllowedDomains) {
		t.Fatalf("New() error = %v, want ErrNoAllowedDomains", err)
	}
}

func TestNew_ResolvesBinaryAndPolicy(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent-browser.exe")
	if err := os.WriteFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client, err := New(Options{
		BinaryPath:     binary,
		AllowedDomains: []string{"example.com"},
		StateDir:       filepath.Join(dir, "state"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if client.BinaryPath() != binary {
		t.Fatalf("BinaryPath() = %q, want %q", client.BinaryPath(), binary)
	}

	policy := filepath.Join(dir, "state", "action-policy.json")
	data, err := os.ReadFile(policy)
	if err != nil {
		t.Fatalf("policy file missing: %v", err)
	}
	if !strings.Contains(string(data), `"eval"`) && !strings.Contains(string(data), `"default":"deny"`) {
		t.Fatalf("policy = %s, want a restrictive default", data)
	}
}

func TestActionPolicyAllowsEveryToolAction(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent-browser.exe")
	if err := os.WriteFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := New(Options{
		BinaryPath:     binary,
		AllowedDomains: []string{"example.com"},
		StateDir:       filepath.Join(dir, "state"),
	}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "state", "action-policy.json"))
	if err != nil {
		t.Fatalf("policy file missing: %v", err)
	}
	policy := string(data)
	// The browser tool exposes open, snapshot, click, fill, press, scroll, back,
	// wait, screenshot and close. agent-browser checks the raw action names, so
	// press and back must be present even though earlier policies omitted them.
	for _, action := range []string{
		"navigate", "snapshot", "click", "fill", "press", "scroll", "back",
		"wait", "screenshot", "close",
	} {
		if !strings.Contains(policy, `"`+action+`"`) {
			t.Fatalf("policy = %s, missing action %q", policy, action)
		}
	}
	for _, blocked := range []string{"eval", "download", "upload"} {
		if strings.Contains(policy, `"`+blocked+`"`) {
			t.Fatalf("policy = %s, must not allow %q", policy, blocked)
		}
	}
}

func TestCleanChildEnvDropsWindowsCompatLayer(t *testing.T) {
	env := []string{
		"PATH=C:\\Windows\\system32",
		"__COMPAT_LAYER=DetectorsAppHealth",
		"__compat_layer=AnotherShim",
		"TEMP=C:\\Temp",
	}
	cleaned := cleanChildEnv(env)
	if len(cleaned) != 2 {
		t.Fatalf("cleanChildEnv() kept %d entries, want 2: %v", len(cleaned), cleaned)
	}
	for _, entry := range cleaned {
		if strings.HasPrefix(strings.ToLower(entry), "__compat_layer=") {
			t.Fatalf("cleanChildEnv() kept a compatibility layer: %q", entry)
		}
	}
}

func TestCleanChildEnvKeepsUnrelatedVariables(t *testing.T) {
	env := []string{
		"AGENT_BROWSER_ARGS=--headless=new",
		"__COMPAT_LAYER=DetectorsAppHealth",
		"SYSTEMROOT=C:\\Windows",
	}
	cleaned := cleanChildEnv(env)
	if len(cleaned) != 2 {
		t.Fatalf("cleanChildEnv() = %v, want the two unrelated variables", cleaned)
	}
	if cleaned[0] != "AGENT_BROWSER_ARGS=--headless=new" || cleaned[1] != "SYSTEMROOT=C:\\Windows" {
		t.Fatalf("cleanChildEnv() = %v, order or contents changed", cleaned)
	}
}
