package providers

import (
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestResolveModelConfig_DisabledModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.ModelName = "disabled-model"
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "disabled-model",
			Provider:  "openai",
			Model:     "gpt-4o-mini",
			APIKeys:   config.SimpleSecureStrings("sk-test"),
			Disabled:  true,
		},
	}

	if _, err := ResolveModelConfig(cfg, "disabled-model"); err == nil {
		t.Fatal("ResolveModelConfig() expected an error for a disabled model")
	} else if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("ResolveModelConfig() error = %v, want a disabled model error", err)
	}
}

func TestResolveModelConfig_SkipsDisabledTemplate(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "disabled-template",
			Provider:  "openai",
			Model:     "gpt-4o-mini",
			APIKeys:   config.SimpleSecureStrings("sk-test"),
			Disabled:  true,
		},
	}

	if _, err := ResolveModelConfig(cfg, "openai/gpt-4.1-mini"); err == nil {
		t.Fatal("ResolveModelConfig() should not use a disabled model as a provider template")
	}
}
