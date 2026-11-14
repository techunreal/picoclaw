package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func startForwardProxy(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var seen atomic.Int32
	transport := &http.Transport{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if !r.URL.IsAbs() {
			http.Error(w, "proxy received a relative request", http.StatusBadRequest)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.Header = r.Header.Clone()
		resp, err := transport.RoundTrip(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(proxy.Close)
	return proxy, &seen
}

func startProxyModelsServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"proxy-model","owned_by":"test"}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleFetchModels_UsesProxy(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	target := startProxyModelsServer(t)
	proxy, seen := startForwardProxy(t)

	body := fmt.Sprintf(`{"provider":"openai-compatible","api_base":%q,"proxy":%q}`, target.URL+"/v1", proxy.URL)
	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/fetch", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if seen.Load() == 0 {
		t.Fatal("fetch did not go through the configured proxy")
	}

	var resp struct {
		Models []upstreamModel `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Models) != 1 || resp.Models[0].ID != "proxy-model" {
		t.Fatalf("models = %#v, want proxy-model", resp.Models)
	}
}

func TestHandleTestInlineModel_UsesProxy(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	target := startProxyModelsServer(t)
	proxy, seen := startForwardProxy(t)

	body := fmt.Sprintf(`{
		"provider":"openai",
		"model":"proxy-model",
		"api_base":%q,
		"proxy":%q
	}`, target.URL+"/v1", proxy.URL)
	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/test-inline", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if seen.Load() == 0 {
		t.Fatal("test-inline probe did not go through the configured proxy")
	}

	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false, want true (body=%s)", rec.Body.String())
	}
}

func TestHandleFetchModels_RejectsInvalidProxy(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/fetch", `{
		"provider":"openai",
		"api_base":"https://api.openai.com/v1",
		"proxy":"not-a-url"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid proxy") {
		t.Fatalf("body = %q, want invalid proxy error", rec.Body.String())
	}
}

func TestHandleFetchModels_ReusesStoredProxy(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	target := startProxyModelsServer(t)
	proxy, seen := startForwardProxy(t)

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		{
			ModelName: "proxy-model-entry",
			Provider:  "openai",
			Model:     "proxy-model",
			APIBase:   target.URL + "/v1",
			Proxy:     proxy.URL,
			APIKeys:   config.SimpleSecureStrings("sk-test"),
		},
	})

	body := fmt.Sprintf(`{
		"provider":"openai",
		"api_base":%q,
		"model_index":0
	}`, target.URL+"/v1")
	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/fetch", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if seen.Load() == 0 {
		t.Fatal("fetch did not reuse the stored model proxy")
	}
}

func TestHandleTestInlineModel_RejectsInvalidProxy(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/test-inline", `{
		"provider":"openai",
		"model":"gpt-4o-mini",
		"api_base":"https://api.openai.com/v1",
		"proxy":"ftp://proxy.example.com"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
