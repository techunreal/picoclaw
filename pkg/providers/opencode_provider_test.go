package providers

import (
	"strings"
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

func TestOpenCodeSessionHeader(t *testing.T) {
	for _, protocol := range []string{"opencode-go", "opencode-zen"} {
		headers := customHeadersForProtocol(protocol, nil)
		if strings.TrimSpace(headers["x-opencode-session"]) == "" {
			t.Fatalf("%s: x-opencode-session header missing", protocol)
		}
	}
	custom := map[string]string{"X-OpenCode-Session": "mine"}
	headers := customHeadersForProtocol("opencode-go", custom)
	if headers["X-OpenCode-Session"] != "mine" {
		t.Fatalf("existing header was overwritten: %#v", headers)
	}
	if _, exists := headers["x-opencode-session"]; exists {
		t.Fatalf("duplicate header added: %#v", headers)
	}
	if customHeadersForProtocol("openai", nil) != nil {
		t.Fatal("non-OpenCode providers should not receive extra headers")
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
