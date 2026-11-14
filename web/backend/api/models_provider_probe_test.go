package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func postTestInline(t *testing.T, configPath, body string) (int, map[string]any) {
	t.Helper()

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/test-inline", body)
	var resp map[string]any
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v, body=%s", err, rec.Body.String())
		}
	}
	return rec.Code, resp
}

func TestHandleTestInlineModel_GeminiProbe(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-key" {
			t.Errorf("x-goog-api-key = %q, want gemini-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[{"name":"models/gemini-2.0-flash"}]}`)
	}))
	defer srv.Close()

	body := fmt.Sprintf(`{
		"provider":"gemini",
		"model":"gemini-2.0-flash",
		"api_base":%q,
		"api_key":"gemini-key"
	}`, srv.URL+"/v1beta")
	code, resp := postTestInline(t, configPath, body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Fatalf("success = %v, want true (resp=%v)", resp["success"], resp)
	}
}

func TestHandleTestInlineModel_GeminiUnknownModel(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[{"name":"models/other-model"}]}`)
	}))
	defer srv.Close()

	body := fmt.Sprintf(`{
		"provider":"gemini",
		"model":"gemini-2.0-flash",
		"api_base":%q,
		"api_key":"gemini-key"
	}`, srv.URL+"/v1beta")
	code, resp := postTestInline(t, configPath, body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if success, _ := resp["success"].(bool); success {
		t.Fatalf("success = true, want false for an unavailable model")
	}
}

func TestHandleTestInlineModel_AnthropicProbe(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-api-key"); got != "anthropic-key" {
			t.Errorf("x-api-key = %q, want anthropic-key", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want 2023-06-01", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"claude-sonnet-4-6"}]}`)
	}))
	defer srv.Close()

	body := fmt.Sprintf(`{
		"provider":"anthropic-messages",
		"model":"claude-sonnet-4-6",
		"api_base":%q,
		"api_key":"anthropic-key"
	}`, srv.URL+"/v1")
	code, resp := postTestInline(t, configPath, body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Fatalf("success = %v, want true (resp=%v)", resp["success"], resp)
	}
}

func TestHandleTestInlineModel_AnthropicUnknownModel(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"claude-other"}]}`)
	}))
	defer srv.Close()

	body := fmt.Sprintf(`{
		"provider":"anthropic",
		"model":"claude-sonnet-4-6",
		"api_base":%q,
		"api_key":"anthropic-key"
	}`, srv.URL+"/v1")
	code, resp := postTestInline(t, configPath, body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if success, _ := resp["success"].(bool); success {
		t.Fatalf("success = true, want false for an unavailable model")
	}
}

func TestHandleTestModel_PerformsRealProbe(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"probe-model"}]}`)
	}))
	defer srv.Close()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		{
			ModelName: "probe-entry",
			Provider:  "openai-compatible",
			Model:     "probe-model",
			APIBase:   srv.URL + "/v1",
			APIKeys:   config.SimpleSecureStrings("sk-test"),
		},
	})

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/0/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if hits.Load() == 0 {
		t.Fatal("saved model test did not perform a network probe")
	}

	var resp struct {
		Success bool   `json:"success"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success || resp.Status != modelStatusAvailable {
		t.Fatalf("response = %#v, want success with available status", resp)
	}
}

func TestHandleTestModel_UnconfiguredDoesNotProbe(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		{
			ModelName: "unconfigured",
			Provider:  "openai",
			Model:     "gpt-4o-mini",
			APIBase:   "https://api.openai.com/v1",
		},
	})

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models/0/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool   `json:"success"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Success || resp.Status != modelStatusUnconfigured {
		t.Fatalf("response = %#v, want unconfigured failure", resp)
	}
}
