package agent

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/session"
)

func TestSessionMemoryPreferenceFlowsToPrompt(t *testing.T) {
	store, err := memory.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	backend := session.NewJSONLBackend(store)

	disabled := false
	if err := backend.SetSessionMemoryEnabled("session-off", &disabled); err != nil {
		t.Fatalf("SetSessionMemoryEnabled() error = %v", err)
	}

	var req PromptBuildRequest
	applySessionMemoryPreference(&req, backend, "session-off")
	if req.MemoryEnabled == nil || *req.MemoryEnabled {
		t.Fatalf("expected session memory to be disabled, got %#v", req.MemoryEnabled)
	}

	// Inbound Pico metadata updates the preference before the turn runs.
	applyInboundMemoryPreference(backend, "session-on", map[string]string{
		"memory_enabled": "false",
	})
	enabled, found := backend.GetSessionMemoryEnabled("session-on")
	if !found || enabled {
		t.Fatalf("inbound preference = (%v,%v), want (false,true)", enabled, found)
	}

	applyInboundMemoryPreference(backend, "session-on", map[string]string{
		"memory_enabled": "true",
	})
	enabled, found = backend.GetSessionMemoryEnabled("session-on")
	if !found || !enabled {
		t.Fatalf("updated preference = (%v,%v), want (true,true)", enabled, found)
	}
}
