package providers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestDetectModelContextWindowFromLlamaCppListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"/models/candidate.gguf","object":"model","context_length":65536}]}`))
	}))
	defer srv.Close()

	value := DetectModelContextWindow(&config.ModelConfig{
		Provider: "openai-compatible",
		Model:    "local",
		APIBase:  srv.URL + "/v1",
	})
	if value != 65536 {
		t.Fatalf("context window = %d, want 65536", value)
	}
}

func TestDetectModelContextWindowMatchesExactID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"other","context_length":4096},
			{"id":"chosen","context_window":131072}
		]}`))
	}))
	defer srv.Close()

	value := DetectModelContextWindow(&config.ModelConfig{
		Provider: "openai-compatible",
		Model:    "chosen",
		APIBase:  srv.URL,
	})
	if value != 131072 {
		t.Fatalf("context window = %d, want 131072", value)
	}
}

func TestDetectModelContextWindowUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}]}`))
	}))
	defer srv.Close()

	if value := DetectModelContextWindow(&config.ModelConfig{
		Provider: "openai-compatible",
		Model:    "missing",
		APIBase:  srv.URL,
	}); value != 0 {
		t.Fatalf("context window = %d, want 0", value)
	}
}
