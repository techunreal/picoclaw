package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestSessionSecret(t *testing.T) (*LauncherDashboardSessionSecret, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "launcher-session.key")
	secret, err := LoadOrCreateLauncherDashboardSessionSecret(path)
	if err != nil {
		t.Fatalf("LoadOrCreateLauncherDashboardSessionSecret() error = %v", err)
	}
	return secret, path
}

func TestLauncherSessionSecretPersistsAcrossLoads(t *testing.T) {
	secret, path := newTestSessionSecret(t)
	first := secret.Token()
	if first == "" {
		t.Fatal("session secret is empty")
	}

	reloaded, err := LoadOrCreateLauncherDashboardSessionSecret(path)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if got := reloaded.Token(); got != first {
		t.Fatalf("reloaded token = %q, want %q", got, first)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secret file error = %v", err)
	}
	if strings.TrimSpace(string(data)) != first {
		t.Fatalf("file token = %q, want %q", strings.TrimSpace(string(data)), first)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat secret file error = %v", err)
		}
		if perm := info.Mode().Perm(); perm != LauncherSessionSecretFilePerm {
			t.Fatalf("secret file perm = %o, want %o", perm, LauncherSessionSecretFilePerm)
		}
	}
}

func TestLauncherSessionSecretRotateChangesTokenAndFile(t *testing.T) {
	secret, path := newTestSessionSecret(t)
	before := secret.Token()

	after, err := secret.Rotate()
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if after == before {
		t.Fatal("Rotate() returned the previous token")
	}
	if got := secret.Token(); got != after {
		t.Fatalf("Token() = %q, want %q", got, after)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secret file error = %v", err)
	}
	if strings.TrimSpace(string(data)) != after {
		t.Fatalf("file token = %q, want %q", strings.TrimSpace(string(data)), after)
	}

	reloaded, err := LoadOrCreateLauncherDashboardSessionSecret(path)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if got := reloaded.Token(); got != after {
		t.Fatalf("reloaded token = %q, want %q", got, after)
	}
}

func TestLauncherSessionSecretReplacesCorruptFile(t *testing.T) {
	for _, content := range []string{"", "not-a-token", strings.Repeat("a", 200)} {
		path := filepath.Join(t.TempDir(), "launcher-session.key")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		secret, err := LoadOrCreateLauncherDashboardSessionSecret(path)
		if err != nil {
			t.Fatalf("content %q: LoadOrCreateLauncherDashboardSessionSecret() error = %v", content, err)
		}
		token := secret.Token()
		if token == "" || token == content {
			t.Fatalf("content %q: token = %q, want a fresh token", content, token)
		}
		if err := validateLauncherSessionSecret(token); err != nil {
			t.Fatalf("content %q: fresh token invalid: %v", content, err)
		}
	}
}

func TestLauncherDashboardAuthUsesRotatableSecret(t *testing.T) {
	secret, _ := newTestSessionSecret(t)
	cfg := LauncherDashboardAuthConfig{SessionSecret: secret}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := LauncherDashboardAuth(cfg, next)

	oldToken := secret.Token()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: LauncherDashboardCookieName, Value: oldToken})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("before rotate: status = %d, want %d", rec.Code, http.StatusTeapot)
	}

	newToken, err := secret.Rotate()
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: LauncherDashboardCookieName, Value: oldToken})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after rotate with old cookie: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: LauncherDashboardCookieName, Value: newToken})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("after rotate with new cookie: status = %d, want %d", rec.Code, http.StatusTeapot)
	}
}

func TestLauncherSessionSecretEmptyTokenNeverAuthenticates(t *testing.T) {
	cfg := LauncherDashboardAuthConfig{SessionSecret: &LauncherDashboardSessionSecret{}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler must not run with an empty token")
	})
	handler := LauncherDashboardAuth(cfg, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: LauncherDashboardCookieName, Value: ""})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
