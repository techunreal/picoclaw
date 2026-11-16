package providers

import (
	"net/http"
	"net/http/httptest"
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

func TestFindModelConfigForContextDerivesProviderBase(t *testing.T) {
	cfg := &config.Config{
		ModelList: []*config.ModelConfig{
			{
				ModelName: "hy4 opencode",
				Provider:  "opencode-go",
				Model:     "hy4-flash-free",
			},
		},
	}

	for _, name := range []string{"hy4 opencode", "hy4-flash-free"} {
		got := FindModelConfigForContext(cfg, name)
		if got == nil {
			t.Fatalf("%s: expected a model config", name)
		}
		if got.APIBase != "https://opencode.ai/zen/go/v1" {
			t.Fatalf("%s: APIBase = %q, want the provider default", name, got.APIBase)
		}
	}
}

func TestFindModelConfigForContextDoesNotBorrowOpenAITemplate(t *testing.T) {
	cfg := &config.Config{
		ModelList: []*config.ModelConfig{
			{
				ModelName: "gpt-5.4",
				Provider:  "openai",
				Model:     "gpt-5.4",
				APIBase:   "https://api.openai.com/v1",
			},
		},
	}

	if got := FindModelConfigForContext(cfg, "unknown-model"); got != nil {
		t.Fatalf("expected nil, got provider %q base %q", got.Provider, got.APIBase)
	}
}

func TestSupportsContextWindowDetection_OpenCode(t *testing.T) {
	for _, provider := range []string{"opencode-go", "opencode-zen"} {
		if !SupportsContextWindowDetection(&config.ModelConfig{Provider: provider}) {
			t.Fatalf("%s should support context window detection", provider)
		}
	}
	if SupportsContextWindowDetection(&config.ModelConfig{Provider: "openai"}) {
		t.Fatal("openai should not be probed at startup")
	}
}

func TestProbeContextWindow_ReadsLimitContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"hy4-flash-free","limit":{"context":1048576,"input":983040,"output":65536}}]}`))
	}))
	defer server.Close()

	if got := probeContextWindow(server.URL, "hy4-flash-free", ""); got != 1048576 {
		t.Fatalf("probeContextWindow() = %d, want 1048576", got)
	}
}
