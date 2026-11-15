package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/web/backend/middleware"
)

const testSessionPassword = "dashboard-test-password"

func newTestSessionSecret(t *testing.T) *middleware.LauncherDashboardSessionSecret {
	t.Helper()

	secret, err := middleware.LoadOrCreateLauncherDashboardSessionSecret(
		filepath.Join(t.TempDir(), "launcher-session.key"),
	)
	if err != nil {
		t.Fatalf("LoadOrCreateLauncherDashboardSessionSecret() error = %v", err)
	}
	return secret
}

func loginCookie(t *testing.T, mux *http.ServeMux) string {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/login",
		strings.NewReader(`{"password":"`+testSessionPassword+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login code = %d body=%s", rec.Code, rec.Body.String())
	}

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == middleware.LauncherDashboardCookieName {
			return cookie.Value
		}
	}
	t.Fatal("login did not set the dashboard session cookie")
	return ""
}

func statusAuthenticated(t *testing.T, mux *http.ServeMux, token string) bool {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{
			Name:  middleware.LauncherDashboardCookieName,
			Value: token,
		})
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return body.Authenticated
}

func TestLauncherAuthSessionSurvivesRestart(t *testing.T) {
	store := &fakePasswordStore{initialized: true, password: testSessionPassword}
	path := filepath.Join(t.TempDir(), "launcher-session.key")

	secret, err := middleware.LoadOrCreateLauncherDashboardSessionSecret(path)
	if err != nil {
		t.Fatalf("first load error = %v", err)
	}
	mux := http.NewServeMux()
	RegisterLauncherAuthRoutes(mux, LauncherAuthRouteOpts{
		SessionSecret: secret,
		PasswordStore: store,
	})
	token := loginCookie(t, mux)
	if token != secret.Token() {
		t.Fatalf("cookie token = %q, want current secret", token)
	}

	// Simulate a launcher restart: a new process loads the same key file.
	restarted, err := middleware.LoadOrCreateLauncherDashboardSessionSecret(path)
	if err != nil {
		t.Fatalf("restart load error = %v", err)
	}
	restartedMux := http.NewServeMux()
	RegisterLauncherAuthRoutes(restartedMux, LauncherAuthRouteOpts{
		SessionSecret: restarted,
		PasswordStore: store,
	})
	if !statusAuthenticated(t, restartedMux, token) {
		t.Fatal("session cookie issued before restart should still authenticate")
	}
}

func TestLauncherAuthSetupRotatesSession(t *testing.T) {
	store := &fakePasswordStore{initialized: true, password: testSessionPassword}
	secret := newTestSessionSecret(t)
	mux := http.NewServeMux()
	RegisterLauncherAuthRoutes(mux, LauncherAuthRouteOpts{
		SessionSecret: secret,
		PasswordStore: store,
	})

	oldToken := secret.Token()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/setup",
		strings.NewReader(`{"password":"new-password","confirm":"new-password"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{
		Name:  middleware.LauncherDashboardCookieName,
		Value: oldToken,
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup code = %d body=%s", rec.Code, rec.Body.String())
	}
	if store.password != "new-password" {
		t.Fatalf("stored password = %q, want new-password", store.password)
	}

	newToken := secret.Token()
	if newToken == oldToken {
		t.Fatal("setup did not rotate the session secret")
	}
	if statusAuthenticated(t, mux, oldToken) {
		t.Fatal("old session cookie must be invalid after a password change")
	}
	if !statusAuthenticated(t, mux, newToken) {
		t.Fatal("new session cookie should be valid after a password change")
	}

	var issued string
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == middleware.LauncherDashboardCookieName {
			issued = cookie.Value
		}
	}
	if issued != newToken {
		t.Fatalf("setup response cookie = %q, want rotated token", issued)
	}
}

func TestLauncherAuthAnonymousLogoutDoesNotRotateSession(t *testing.T) {
	store := &fakePasswordStore{initialized: true, password: testSessionPassword}
	secret := newTestSessionSecret(t)
	mux := http.NewServeMux()
	RegisterLauncherAuthRoutes(mux, LauncherAuthRouteOpts{
		SessionSecret: secret,
		PasswordStore: store,
	})

	before := secret.Token()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout code = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := secret.Token(); got != before {
		t.Fatal("anonymous logout must not rotate the session secret")
	}
}

func TestLauncherAuthLogoutRotatesSession(t *testing.T) {
	store := &fakePasswordStore{initialized: true, password: testSessionPassword}
	secret := newTestSessionSecret(t)
	mux := http.NewServeMux()
	RegisterLauncherAuthRoutes(mux, LauncherAuthRouteOpts{
		SessionSecret: secret,
		PasswordStore: store,
	})

	oldToken := secret.Token()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{
		Name:  middleware.LauncherDashboardCookieName,
		Value: oldToken,
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout code = %d body=%s", rec.Code, rec.Body.String())
	}

	if got := secret.Token(); got == oldToken {
		t.Fatal("logout did not rotate the session secret")
	}
	if statusAuthenticated(t, mux, oldToken) {
		t.Fatal("session cookie must be invalid after sign out")
	}

	var cleared bool
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == middleware.LauncherDashboardCookieName && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout did not clear the session cookie")
	}
}
