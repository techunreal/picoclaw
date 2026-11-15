package middleware

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sipeed/picoclaw/pkg/fileutil"
)

// LauncherSessionSecretFilePerm is the permission used for the persistent
// dashboard session secret file (owner read/write only on Unix).
const LauncherSessionSecretFilePerm os.FileMode = 0o600

// LauncherDashboardSessionSecret holds the persistent dashboard session token.
//
// The token is the value stored in the HttpOnly session cookie. Persisting it
// keeps existing sessions valid across launcher restarts, while Rotate
// invalidates every previously issued cookie.
type LauncherDashboardSessionSecret struct {
	mu    sync.RWMutex
	path  string
	token string
}

// LoadOrCreateLauncherDashboardSessionSecret loads the session token from path,
// creating a new one when the file is missing or unreadable/corrupt. A corrupt
// file is replaced so the launcher can still start; existing sessions are then
// invalidated, which is the safe failure mode.
func LoadOrCreateLauncherDashboardSessionSecret(
	path string,
) (*LauncherDashboardSessionSecret, error) {
	token, err := readLauncherSessionSecret(path)
	if err == nil {
		return &LauncherDashboardSessionSecret{path: path, token: token}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		// Keep the reason for operators but continue with a fresh token.
		token, writeErr := createLauncherSessionSecret(path)
		if writeErr != nil {
			return nil, fmt.Errorf(
				"launcher session secret is unusable (%v) and could not be recreated: %w",
				err,
				writeErr,
			)
		}
		return &LauncherDashboardSessionSecret{path: path, token: token}, nil
	}

	token, err = createLauncherSessionSecret(path)
	if err != nil {
		return nil, err
	}
	return &LauncherDashboardSessionSecret{path: path, token: token}, nil
}

// Token returns the current session cookie value.
func (s *LauncherDashboardSessionSecret) Token() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

// Rotate generates, persists and activates a new session token. Every cookie
// issued before the call stops authenticating.
func (s *LauncherDashboardSessionSecret) Rotate() (string, error) {
	if s == nil {
		return "", errors.New("launcher session secret is not configured")
	}

	token, err := randomURLToken(launcherSessionCookieBytes)
	if err != nil {
		return "", err
	}
	if err := writeLauncherSessionSecret(s.path, token); err != nil {
		return "", err
	}

	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
	return token, nil
}

func createLauncherSessionSecret(path string) (string, error) {
	token, err := randomURLToken(launcherSessionCookieBytes)
	if err != nil {
		return "", err
	}
	if err := writeLauncherSessionSecret(path, token); err != nil {
		return "", err
	}
	return token, nil
}

func readLauncherSessionSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if err := validateLauncherSessionSecret(token); err != nil {
		return "", err
	}
	return token, nil
}

func writeLauncherSessionSecret(path, token string) error {
	if err := validateLauncherSessionSecret(token); err != nil {
		return err
	}
	if err := fileutil.WriteFileAtomic(path, []byte(token+"\n"), LauncherSessionSecretFilePerm); err != nil {
		return fmt.Errorf("failed to write launcher session secret: %w", err)
	}
	return nil
}

func validateLauncherSessionSecret(token string) error {
	if token == "" {
		return errors.New("empty session secret")
	}
	// The token must be a raw URL-safe base64 string of the expected length.
	if len(token) != base64.RawURLEncoding.EncodedLen(launcherSessionCookieBytes) {
		return errors.New("session secret has an unexpected length")
	}
	if _, err := base64.RawURLEncoding.DecodeString(token); err != nil {
		return fmt.Errorf("session secret is not valid base64: %w", err)
	}
	return nil
}
