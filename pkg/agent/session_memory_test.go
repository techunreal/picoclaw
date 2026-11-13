package agent

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/session"
)

func TestInboundIncognitoSessionStaysInMemory(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	backend := session.NewJSONLBackend(store)

	applyInboundIncognito(backend, "ghost", map[string]string{"incognito": "true"})
	backend.EnsureSessionMetadata("ghost", nil, nil)
	backend.AddMessage("ghost", "user", "secret")

	history := backend.GetHistory("ghost")
	if len(history) != 1 || history[0].Content != "secret" {
		t.Fatalf("incognito history = %+v, want one message", history)
	}
	if enabled, found := backend.GetSessionMemoryEnabled("ghost"); enabled || !found {
		t.Fatalf("incognito memory preference = (%v,%v), want (false,true)", enabled, found)
	}

	store2, err := memory.NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	defer store2.Close()
	fresh := session.NewJSONLBackend(store2)
	if history := fresh.GetHistory("ghost"); len(history) != 0 {
		t.Fatalf("incognito history leaked to disk: %+v", history)
	}
	if scope := fresh.GetSessionScope("ghost"); scope != nil {
		t.Fatalf("incognito scope leaked to disk: %+v", scope)
	}
}

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
