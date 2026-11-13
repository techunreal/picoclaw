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
