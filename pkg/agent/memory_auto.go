// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/session"
)

const (
	// autoMemorySnippetLimit caps each side of an automatic daily-note entry.
	autoMemorySnippetLimit = 280
	// autoMemoryEntryLimit caps the whole entry after formatting.
	autoMemoryEntryLimit = 640
	// maxAutoDailyNoteBytes stops appending when today's note grows too large.
	maxAutoDailyNoteBytes = 64 * 1024
)

// autoMemoryWriteMu serializes read-modify-write cycles on today's note so
// concurrent sessions cannot lose entries.
var autoMemoryWriteMu sync.Mutex

// maybeAutoWriteMemory appends a compact summary of a completed turn to
// today's daily note when all memory switches allow it.
func (al *AgentLoop) maybeAutoWriteMemory(
	agent *AgentInstance,
	opts processOptions,
	assistantMessage string,
) {
	if agent == nil {
		return
	}
	if !MemoryEnabled() || !MemoryAutoWriteEnabled() {
		return
	}
	if strings.TrimSpace(agent.Workspace) == "" {
		return
	}
	if constants.IsInternalChannel(opts.Dispatch.Channel()) {
		return
	}
	if !sessionAllowsMemory(agent.Sessions, opts.Dispatch.SessionKey) {
		return
	}

	userMessage := strings.TrimSpace(opts.Dispatch.UserMessage)
	assistantMessage = strings.TrimSpace(assistantMessage)
	if userMessage == "" || assistantMessage == "" {
		return
	}
	// Slash commands never reach the model; guard anyway against future
	// command handling that still reports a reply.
	if strings.HasPrefix(userMessage, "/") {
		return
	}

	entry := formatAutoMemoryEntry(time.Now(), userMessage, assistantMessage)
	if err := appendDailyMemoryEntry(agent.Workspace, entry); err != nil {
		logger.WarnCF("agent", "Failed to append automatic daily memory",
			map[string]any{
				"agent_id": agent.ID,
				"error":    err.Error(),
			})
	}
}

// sessionAllowsMemory reports whether the session did not opt out of memory.
// Incognito sessions store an explicit false preference and are excluded.
func sessionAllowsMemory(sessions session.SessionStore, sessionKey string) bool {
	if sessions == nil || strings.TrimSpace(sessionKey) == "" {
		return false
	}
	store, ok := sessions.(session.SessionMemoryPreferenceStore)
	if !ok {
		return true
	}
	enabled, found := store.GetSessionMemoryEnabled(sessionKey)
	if found {
		return enabled
	}
	return true
}

// formatAutoMemoryEntry renders one line for the daily note. Newlines are
// collapsed so a single turn always occupies one bullet.
func formatAutoMemoryEntry(now time.Time, userMessage, assistantMessage string) string {
	entry := "- " + now.Format("15:04") +
		" — User: " + autoMemorySnippet(userMessage) +
		" → Assistant: " + autoMemorySnippet(assistantMessage)
	if len(entry) > autoMemoryEntryLimit {
		entry = strings.ToValidUTF8(entry[:autoMemoryEntryLimit], "") + "..."
	}
	return entry
}

func autoMemorySnippet(text string) string {
	collapsed := strings.Join(strings.Fields(text), " ")
	runes := []rune(collapsed)
	if len(runes) <= autoMemorySnippetLimit {
		return collapsed
	}
	return string(runes[:autoMemorySnippetLimit]) + "..."
}

// appendDailyMemoryEntry writes an entry to today's note, respecting the size
// cap. A full note is skipped rather than truncated, so existing notes are
// never lost.
func appendDailyMemoryEntry(workspace, entry string) error {
	store := NewMemoryStore(workspace)

	autoMemoryWriteMu.Lock()
	defer autoMemoryWriteMu.Unlock()

	if len(store.ReadToday()) >= maxAutoDailyNoteBytes {
		logger.DebugCF("agent", "Skipping automatic daily memory: note is full",
			map[string]any{"limit_bytes": maxAutoDailyNoteBytes})
		return nil
	}
	return store.AppendToday(entry)
}
