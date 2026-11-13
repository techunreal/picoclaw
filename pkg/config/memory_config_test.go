package config

import (
	"encoding/json"
	"testing"
)

func TestMemoryConfigDefaultsToEnabled(t *testing.T) {
	var cfg MemoryConfig
	if !cfg.IsEnabled() {
		t.Fatal("zero-value MemoryConfig must be enabled")
	}

	cfg.SetEnabled(false)
	if cfg.IsEnabled() {
		t.Fatal("explicitly disabled MemoryConfig must be disabled")
	}
	cfg.SetEnabled(true)
	if !cfg.IsEnabled() {
		t.Fatal("explicitly enabled MemoryConfig must be enabled")
	}
}

func TestMemoryConfigAutoWriteDefaultsToDisabled(t *testing.T) {
	var cfg MemoryConfig
	if cfg.IsAutoWriteEnabled() {
		t.Fatal("zero-value MemoryConfig must not auto-write")
	}

	cfg.SetAutoWrite(true)
	if !cfg.IsAutoWriteEnabled() {
		t.Fatal("explicitly enabled auto-write must be enabled")
	}
	cfg.SetAutoWrite(false)
	if cfg.IsAutoWriteEnabled() {
		t.Fatal("explicitly disabled auto-write must be disabled")
	}
}

func TestMemoryConfigAutoWriteJSONRoundTrip(t *testing.T) {
	raw := []byte(`{"workspace":"/tmp/ws","memory":{"enabled":true,"auto_write":true}}`)
	var defaults AgentDefaults
	if err := json.Unmarshal(raw, &defaults); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !defaults.Memory.IsAutoWriteEnabled() {
		t.Fatal("memory.auto_write=true should enable auto-write")
	}

	encoded, err := json.Marshal(defaults)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal(encoded) error = %v", err)
	}
	memory, ok := decoded["memory"].(map[string]any)
	if !ok {
		t.Fatalf("encoded memory field missing: %s", string(encoded))
	}
	if autoWrite, ok := memory["auto_write"].(bool); !ok || !autoWrite {
		t.Fatalf("encoded memory.auto_write = %v, want true", memory["auto_write"])
	}
}

func TestMemoryConfigJSONRoundTrip(t *testing.T) {
	raw := []byte(`{"workspace":"/tmp/ws","memory":{"enabled":false}}`)
	var defaults AgentDefaults
	if err := json.Unmarshal(raw, &defaults); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if defaults.Memory.IsEnabled() {
		t.Fatal("memory.enabled=false should disable memory")
	}

	encoded, err := json.Marshal(defaults)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal(encoded) error = %v", err)
	}
	memory, ok := decoded["memory"].(map[string]any)
	if !ok {
		t.Fatalf("encoded memory field missing: %s", string(encoded))
	}
	if enabled, ok := memory["enabled"].(bool); !ok || enabled {
		t.Fatalf("encoded memory.enabled = %v, want false", memory["enabled"])
	}
}
