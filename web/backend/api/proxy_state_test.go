package api

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

// resetGatewayProxyState clears process-wide gateway state so proxy handlers see
// a stopped gateway even when a previous test left a fake process behind.
func resetGatewayProxyState(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvHome, t.TempDir())

	gateway.mu.Lock()
	gateway.cmd = nil
	gateway.pidData = nil
	gateway.owned = false
	setGatewayRuntimeStatusLocked("stopped")
	gateway.mu.Unlock()
}
