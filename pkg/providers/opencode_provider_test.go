package providers

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestOpenCodeProviderCatalog(t *testing.T) {
	goOption, ok := modelProviderOptionForName("opencode-go")
	if !ok {
		t.Fatal("opencode-go provider is missing")
	}
	if goOption.DefaultAPIBase != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("opencode-go base = %q", goOption.DefaultAPIBase)
	}
	if !goOption.CreateAllowed || !goOption.DefaultModelAllowed || !goOption.SupportsFetch {
		t.Fatalf("opencode-go flags = %#v", goOption)
	}
	if !IsHTTPAPIProtocol("opencode-go") {
		t.Fatal("opencode-go should use the HTTP API path")
	}
	if !IsSupportedModelProvider("opencode") {
		t.Fatal("the opencode alias should resolve to a supported provider")
	}
	aliased, _ := modelProviderOptionForName("opencode")
	if aliased.ID != "opencode-go" {
		t.Fatalf("opencode alias = %q, want opencode-go", aliased.ID)
	}

	zenOption, ok := modelProviderOptionForName("opencode-zen")
	if !ok {
		t.Fatal("opencode-zen provider is missing")
	}
	if zenOption.DefaultAPIBase != "https://opencode.ai/zen/v1" {
		t.Fatalf("opencode-zen base = %q", zenOption.DefaultAPIBase)
	}
	if !IsSupportedModelProvider("opencode-zen") || !IsHTTPAPIProtocol("opencode-zen") {
		t.Fatal("opencode-zen should be a supported HTTP API provider")
	}
}

func TestCreateProviderFromConfig_OpenCode(t *testing.T) {
	for _, item := range []struct {
		provider string
		model    string
	}{
		{provider: "opencode-go", model: "hy4-flash-free"},
		{provider: "opencode-zen", model: "deepseek-v4-pro"},
	} {
		cfg := &config.ModelConfig{
			ModelName: "opencode test",
			Provider:  item.provider,
			Model:     item.model,
		}
		cfg.SetAPIKey("sk-test")
		provider, modelID, err := CreateProviderFromConfig(cfg)
		if err != nil {
			t.Fatalf("%s: CreateProviderFromConfig() error = %v", item.provider, err)
		}
		if provider == nil {
			t.Fatalf("%s: CreateProviderFromConfig() returned nil provider", item.provider)
		}
		if modelID != item.model {
			t.Fatalf("%s: modelID = %q, want %q", item.provider, modelID, item.model)
		}
	}
}
