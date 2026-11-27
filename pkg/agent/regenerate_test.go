package agent

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
)

func TestRegenerateRequested(t *testing.T) {
	if regenerateRequested(processOptions{}) {
		t.Fatal("empty options must not regenerate")
	}
	opts := processOptions{Dispatch: DispatchRequest{InboundContext: &bus.InboundContext{
		Raw: map[string]string{"regenerate": "true"},
	}}}
	if !regenerateRequested(opts) {
		t.Fatal("regenerate flag was not detected")
	}
}

func TestPrepareRegenerateRewindsToLastUser(t *testing.T) {
	store := session.NewSessionManager(t.TempDir())
	key := "regenerate-test"
	store.AddMessage(key, "user", "first")
	store.AddMessage(key, "assistant", "first answer")
	store.AddFullMessage(key, providers.Message{Role: "user", Content: "second"})
	store.AddFullMessage(key, providers.Message{Role: "assistant", Content: "bad tool text"})

	ts := &turnState{
		session:    store,
		sessionKey: key,
		opts: processOptions{Dispatch: DispatchRequest{InboundContext: &bus.InboundContext{
			Raw: map[string]string{"regenerate": "true"},
		}}},
	}
	if !ts.prepareRegenerate() {
		t.Fatal("prepareRegenerate returned false")
	}
	if ts.userMessage != "second" {
		t.Fatalf("userMessage = %q, want second", ts.userMessage)
	}
	saved := store.GetHistory(key)
	if len(saved) != 2 || saved[0].Content != "first" || saved[1].Content != "first answer" {
		t.Fatalf("saved history = %#v", saved)
	}
}
