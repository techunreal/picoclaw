package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/session"
)

func autoMemoryTestOptions(message string) processOptions {
	return processOptions{
		Dispatch: DispatchRequest{
			SessionKey:  "session-1",
			UserMessage: message,
			InboundContext: &bus.InboundContext{
				Channel: "pico",
				ChatID:  "pico:session-1",
			},
		},
	}
}

func resetAutoMemorySwitches(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SetMemoryEnabled(true)
		SetMemoryAutoWrite(false)
	})
	SetMemoryEnabled(true)
	SetMemoryAutoWrite(true)
}

func TestAutoMemoryWritesTodaysNote(t *testing.T) {
	resetAutoMemorySwitches(t)
	workspace := t.TempDir()
	agent := &AgentInstance{
		ID:        "main",
		Workspace: workspace,
		Sessions:  session.NewSessionManager(t.TempDir()),
	}

	(&AgentLoop{}).maybeAutoWriteMemory(
		agent,
		autoMemoryTestOptions("plan the weekend"),
		"Sure, here is a short plan.",
	)

	note := NewMemoryStore(workspace).ReadToday()
	if !strings.Contains(note, "User: plan the weekend") {
		t.Fatalf("daily note missing user message:\n%s", note)
	}
	if !strings.Contains(note, "Assistant: Sure, here is a short plan.") {
		t.Fatalf("daily note missing assistant message:\n%s", note)
	}
	if !strings.HasPrefix(note, "# ") {
		t.Fatalf("daily note header missing:\n%s", note)
	}
}

func TestAutoMemoryRespectsSwitches(t *testing.T) {
	resetAutoMemorySwitches(t)
	workspace := t.TempDir()
	manager := session.NewSessionManager(t.TempDir())
	agent := &AgentInstance{ID: "main", Workspace: workspace, Sessions: manager}

	cases := []struct {
		name      string
		mutate    func()
		user      string
		assistant string
	}{
		{
			name: "auto write off",
			mutate: func() {
				SetMemoryAutoWrite(false)
			},
			user: "hello", assistant: "hi",
		},
		{
			name: "memory master off",
			mutate: func() {
				SetMemoryEnabled(false)
			},
			user: "hello", assistant: "hi",
		},
		{
			name:   "internal channel",
			mutate: func() {},
			user:   "hello", assistant: "hi",
		},
		{
			name:   "empty assistant",
			mutate: func() {},
			user:   "hello", assistant: "   ",
		},
		{
			name:   "slash command",
			mutate: func() {},
			user:   "/status", assistant: "ok",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetAutoMemorySwitches(t)
			tc.mutate()
			opts := autoMemoryTestOptions(tc.user)
			if tc.name == "internal channel" {
				opts.Dispatch.InboundContext.Channel = "system"
			}
			(&AgentLoop{}).maybeAutoWriteMemory(agent, opts, tc.assistant)
			if note := NewMemoryStore(workspace).ReadToday(); note != "" {
				t.Fatalf("unexpected daily note write:\n%s", note)
			}
		})
	}
}

func TestAutoMemoryRespectsSessionOptOut(t *testing.T) {
	resetAutoMemorySwitches(t)
	workspace := t.TempDir()
	store, err := memory.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	backend := session.NewJSONLBackend(store)
	disabled := false
	if err := backend.SetSessionMemoryEnabled("session-1", &disabled); err != nil {
		t.Fatalf("SetSessionMemoryEnabled() error = %v", err)
	}
	agent := &AgentInstance{ID: "main", Workspace: workspace, Sessions: backend}

	(&AgentLoop{}).maybeAutoWriteMemory(agent, autoMemoryTestOptions("secret"), "noted")
	if note := NewMemoryStore(workspace).ReadToday(); note != "" {
		t.Fatalf("session opt-out must not write memory:\n%s", note)
	}
}

func TestAutoMemorySkipsIncognito(t *testing.T) {
	resetAutoMemorySwitches(t)
	workspace := t.TempDir()
	store, err := memory.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}
	backend := session.NewJSONLBackend(store)
	backend.SetSessionEphemeral("session-1", true)
	agent := &AgentInstance{ID: "main", Workspace: workspace, Sessions: backend}

	(&AgentLoop{}).maybeAutoWriteMemory(agent, autoMemoryTestOptions("ghost"), "boo")
	if note := NewMemoryStore(workspace).ReadToday(); note != "" {
		t.Fatalf("incognito session must not write memory:\n%s", note)
	}
}

func TestFormatAutoMemoryEntryCollapsesAndTruncates(t *testing.T) {
	now := time.Date(2026, 11, 14, 9, 5, 0, 0, time.Local)
	entry := formatAutoMemoryEntry(now, "line one\nline two", "reply\twith   spaces")
	if strings.Contains(entry, "\n") {
		t.Fatalf("entry must be a single line: %q", entry)
	}
	if !strings.Contains(entry, "line one line two") {
		t.Fatalf("user message not collapsed: %q", entry)
	}
	if !strings.Contains(entry, "reply with spaces") {
		t.Fatalf("assistant message not collapsed: %q", entry)
	}
	if !strings.Contains(entry, "09:05") {
		t.Fatalf("entry missing timestamp: %q", entry)
	}

	long := strings.Repeat("a", autoMemorySnippetLimit*2)
	truncated := formatAutoMemoryEntry(now, long, long)
	if len(truncated) > autoMemoryEntryLimit {
		t.Fatalf("entry length = %d, want <= %d", len(truncated), autoMemoryEntryLimit)
	}
	if !strings.HasSuffix(truncated, "...") {
		t.Fatalf("truncated entry must end with ellipsis: %q", truncated)
	}
}

func TestAutoMemorySkipsFullDailyNote(t *testing.T) {
	resetAutoMemorySwitches(t)
	workspace := t.TempDir()
	full := strings.Repeat("x", maxAutoDailyNoteBytes+1)
	now := time.Now()
	dateStr := now.Format("20060102")
	notePath := filepath.Join(workspace, "memory", dateStr[:6], dateStr+".md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte(full), 0o600); err != nil {
		t.Fatal(err)
	}

	agent := &AgentInstance{
		ID:        "main",
		Workspace: workspace,
		Sessions:  session.NewSessionManager(t.TempDir()),
	}
	(&AgentLoop{}).maybeAutoWriteMemory(agent, autoMemoryTestOptions("hello"), "hi")

	after := NewMemoryStore(workspace).ReadToday()
	if len(after) != len(full) {
		t.Fatalf("full daily note was modified: got %d bytes, want %d", len(after), len(full))
	}
}
