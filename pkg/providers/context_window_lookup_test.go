package providers

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestFindModelConfigForContextPrefersConfiguredEntry(t *testing.T) {
	cfg := &config.Config{
		ModelList: []*config.ModelConfig{
			{
				ModelName:     "Qwen3.6-35B",
				Provider:      "openai-compatible",
				Model:         "local",
				APIBase:       "http://127.0.0.1:8080/v1",
				ContextWindow: 65536,
			},
		},
	}

	got := FindModelConfigForContext(cfg, "local")
	if got == nil {
		t.Fatal("expected a model config")
	}
	if got.APIBase != "http://127.0.0.1:8080/v1" {
		t.Fatalf("APIBase = %q, want the configured endpoint", got.APIBase)
	}
	if got.ContextWindow != 65536 {
		t.Fatalf("ContextWindow = %d, want 65536", got.ContextWindow)
	}
}
