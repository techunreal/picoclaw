package session

import (
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// maxEphemeralSessions bounds memory use when incognito chats are abandoned
// before the gateway restarts. Oldest sessions are evicted first.
const maxEphemeralSessions = 64

type ephemeralSession struct {
	messages      []providers.Message
	summary       string
	scope         *SessionScope
	aliases       []string
	memoryEnabled *bool
	createdAt     time.Time
}

func (b *JSONLBackend) isEphemeralKey(sessionKey string) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	_, ok := b.ephemeral[sessionKey]
	return ok
}

// SetSessionEphemeral puts a session into memory-only mode (or restores the
// normal persisted mode). Incognito sessions default to memory disabled.
func (b *JSONLBackend) SetSessionEphemeral(sessionKey string, ephemeral bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	if !ephemeral {
		delete(b.ephemeral, sessionKey)
		return
	}
	if b.ephemeral == nil {
		b.ephemeral = make(map[string]*ephemeralSession)
	}
	if _, exists := b.ephemeral[sessionKey]; exists {
		return
	}
	if len(b.ephemeral) >= maxEphemeralSessions {
		b.evictOldestEphemeralLocked()
	}
	disabled := false
	b.ephemeral[sessionKey] = &ephemeralSession{
		memoryEnabled: &disabled,
		createdAt:     time.Now(),
	}
}

func (b *JSONLBackend) evictOldestEphemeralLocked() {
	var oldestKey string
	var oldest time.Time
	for key, state := range b.ephemeral {
		if oldestKey == "" || state.createdAt.Before(oldest) {
			oldestKey = key
			oldest = state.createdAt
		}
	}
	if oldestKey != "" {
		delete(b.ephemeral, oldestKey)
	}
}

func (b *JSONLBackend) ephemeralAddMessage(sessionKey, role, content string) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return false
	}
	state.messages = append(state.messages, providers.Message{Role: role, Content: content})
	return true
}

func (b *JSONLBackend) ephemeralAddFullMessage(sessionKey string, msg providers.Message) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return false
	}
	state.messages = append(state.messages, msg)
	return true
}

func (b *JSONLBackend) ephemeralHistory(sessionKey string) ([]providers.Message, bool) {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return nil, false
	}
	return append([]providers.Message(nil), state.messages...), true
}

func (b *JSONLBackend) ephemeralEnsureMetadata(sessionKey string, scope *SessionScope, aliases []string) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return false
	}
	state.scope = CloneScope(scope)
	state.aliases = append([]string(nil), aliases...)
	return true
}

func (b *JSONLBackend) ephemeralScope(sessionKey string) (*SessionScope, bool) {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return nil, false
	}
	return CloneScope(state.scope), true
}

func (b *JSONLBackend) ephemeralMemoryEnabled(sessionKey string) (bool, bool, bool) {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return true, false, false
	}
	if state.memoryEnabled == nil {
		return true, false, true
	}
	return *state.memoryEnabled, true, true
}

func (b *JSONLBackend) ephemeralSetMemoryEnabled(sessionKey string, enabled *bool) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return false
	}
	state.memoryEnabled = enabled
	return true
}

func (b *JSONLBackend) ephemeralSummary(sessionKey string) (string, bool) {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return "", false
	}
	return state.summary, true
}

func (b *JSONLBackend) ephemeralSetSummary(sessionKey, summary string) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return false
	}
	state.summary = summary
	return true
}

func (b *JSONLBackend) ephemeralSetHistory(sessionKey string, history []providers.Message) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return false
	}
	state.messages = append([]providers.Message(nil), history...)
	return true
}

func (b *JSONLBackend) ephemeralTruncate(sessionKey string, keepLast int) bool {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	state, ok := b.ephemeral[sessionKey]
	if !ok {
		return false
	}
	if keepLast < 0 {
		keepLast = 0
	}
	if keepLast < len(state.messages) {
		state.messages = append([]providers.Message(nil), state.messages[len(state.messages)-keepLast:]...)
	}
	return true
}

func (b *JSONLBackend) ephemeralClear() {
	b.ephemeralMu.Lock()
	defer b.ephemeralMu.Unlock()
	clear(b.ephemeral)
}
