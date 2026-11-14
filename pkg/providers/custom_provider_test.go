package providers

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestModelProviderOptions_IncludesOpenAICompatible(t *testing.T) {
	options := ModelProviderOptions()
	var found *ModelProviderOption
	for i := range options {
		if options[i].ID == "openai-compatible" {
			found = &options[i]
			break
		}
	}
	if found == nil {
		t.Fatal("openai-compatible provider option is missing")
	}
	if !found.CreateAllowed || !found.DefaultModelAllowed || !found.SupportsFetch {
		t.Fatalf(
			"openai-compatible flags = create:%t default:%t fetch:%t, want all true",
			found.CreateAllowed,
			found.DefaultModelAllowed,
			found.SupportsFetch,
		)
	}
	if found.DefaultAPIBase != "" {
		t.Fatalf("default_api_base = %q, want empty (user supplied)", found.DefaultAPIBase)
	}
	if !IsHTTPAPIProtocol("openai-compatible") {
		t.Fatal("openai-compatible should be an HTTP API protocol")
	}
}

func TestIsCustomOpenAICompatibleProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		apiBase  string
		want     bool
	}{
		{name: "unknown with base", provider: "my-llm", apiBase: "https://llm.internal/v1", want: true},
		{name: "unknown with dotted slug", provider: "my.llm_2", apiBase: "http://127.0.0.1:8000/v1", want: true},
		{name: "unknown without base", provider: "my-llm", apiBase: "", want: false},
		{name: "supported provider", provider: "openai", apiBase: "https://api.openai.com/v1", want: false},
		{name: "catalog custom provider", provider: "openai-compatible", apiBase: "https://llm.internal/v1", want: false},
		{name: "slug starting with dash", provider: "-bad", apiBase: "https://llm.internal/v1", want: false},
		{name: "slug with slash", provider: "my/llm", apiBase: "https://llm.internal/v1", want: false},
		{name: "slug with space", provider: "my llm", apiBase: "https://llm.internal/v1", want: false},
		{name: "empty provider", provider: "", apiBase: "https://llm.internal/v1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCustomOpenAICompatibleProvider(tt.provider, tt.apiBase); got != tt.want {
				t.Fatalf("IsCustomOpenAICompatibleProvider(%q, %q) = %t, want %t", tt.provider, tt.apiBase, got, tt.want)
			}
		})
	}
}

func TestCreateProviderFromConfig_OpenAICompatible(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "custom",
		Provider:  "openai-compatible",
		Model:     "my-model",
		APIBase:   "https://llm.internal/v1",
	}
	cfg.SetAPIKey("sk-custom")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "my-model" {
		t.Fatalf("modelID = %q, want my-model", modelID)
	}
}

func TestCreateProviderFromConfig_OpenAICompatibleRequiresAPIBase(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "custom",
		Provider:  "openai-compatible",
		Model:     "my-model",
	}
	cfg.SetAPIKey("sk-custom")

	if _, _, err := CreateProviderFromConfig(cfg); err == nil {
		t.Fatal("CreateProviderFromConfig() expected an error without api_base")
	}
}

func TestCreateProviderFromConfig_CustomUnknownProviderWithBase(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "custom-unknown",
		Provider:  "my-llm",
		Model:     "my-model",
		APIBase:   "https://llm.internal/v1",
	}
	cfg.SetAPIKey("sk-custom")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "my-model" {
		t.Fatalf("modelID = %q, want my-model", modelID)
	}
}
