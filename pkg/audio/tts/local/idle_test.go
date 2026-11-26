package local

import (
	"testing"
	"time"
)

func TestIdleCandidates(t *testing.T) {
	manager := NewManager(t.TempDir())
	manager.idleAfter = 10 * time.Minute
	now := time.Now()

	manager.mu.Lock()
	manager.lastUsed["old"] = now.Add(-11 * time.Minute)
	manager.lastUsed["recent"] = now.Add(-5 * time.Minute)
	manager.lastUsed["busy"] = now.Add(-30 * time.Minute)
	manager.active["busy"] = 1
	manager.mu.Unlock()

	candidates := manager.idleCandidates(now)
	if len(candidates) != 1 || candidates[0] != "old" {
		t.Fatalf("candidates = %#v, want [old]", candidates)
	}

	end := manager.BeginRequest("fresh")
	if end == nil {
		t.Fatal("BeginRequest returned nil")
	}
	manager.mu.Lock()
	if manager.active["fresh"] != 1 {
		t.Fatalf("active = %d, want 1", manager.active["fresh"])
	}
	manager.mu.Unlock()
	end()
	manager.mu.Lock()
	if manager.active["fresh"] != 0 {
		t.Fatalf("active after end = %d, want 0", manager.active["fresh"])
	}
	manager.mu.Unlock()
}

func TestDefaultIdleTimeout(t *testing.T) {
	manager := NewManager(t.TempDir())
	if manager.IdleTimeout() != 10*time.Minute {
		t.Fatalf("idle timeout = %v, want 10m", manager.IdleTimeout())
	}
}
