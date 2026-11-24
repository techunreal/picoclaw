package integrationtools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchSendsBrowserNavigationHeaders(t *testing.T) {
	withPrivateWebFetchHostsAllowed(t)

	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>ok</p></body></html>"))
	}))
	defer server.Close()

	tool, err := NewWebFetchTool(4000, format, testFetchLimit)
	if err != nil {
		t.Fatalf("NewWebFetchTool() error: %v", err)
	}
	if result := tool.Execute(context.Background(), map[string]any{"url": server.URL}); result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	want := map[string]string{
		"Accept-Language":           "en-US,en;q=0.9",
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
	}
	if !strings.Contains(got.Get("Accept"), "text/html") {
		t.Fatalf("Accept = %q", got.Get("Accept"))
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("%s = %q, want %q", key, got.Get(key), value)
		}
	}
	if strings.Contains(strings.ToLower(got.Get("Accept-Encoding")), "br") {
		t.Fatalf("Accept-Encoding must not request brotli manually: %q", got.Get("Accept-Encoding"))
	}

	transport, ok := tool.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", tool.client.Transport)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatalf("web_fetch transport must enable HTTP/2")
	}
}

func TestWebFetchRejectsProtectionPages(t *testing.T) {
	withPrivateWebFetchHostsAllowed(t)

	cases := []struct {
		name       string
		status     int
		cf         string
		body       string
		contentTyp string
		wantError  bool
	}{
		{
			name:       "cloudflare_403_challenge",
			status:     http.StatusForbidden,
			cf:         "challenge",
			body:       "<!DOCTYPE html><html><head><title>Just a moment...</title></head><body>Enable JavaScript and cookies to continue</body></html>",
			contentTyp: "text/html",
			wantError:  true,
		},
		{
			name:       "challenge_markers_without_header",
			status:     http.StatusForbidden,
			body:       "Just a moment...Enable JavaScript and cookies to continue",
			contentTyp: "text/plain",
			wantError:  true,
		},
		{
			name:       "localized_challenge_200",
			status:     http.StatusOK,
			body:       "<html><body>Bir dakika lütfen... Güvenlik doğrulaması yapılıyor</body></html>",
			contentTyp: "text/html",
			wantError:  true,
		},
		{
			name:       "rate_limited",
			status:     http.StatusTooManyRequests,
			body:       "slow down",
			contentTyp: "text/plain",
			wantError:  true,
		},
		{
			name:       "normal_page",
			status:     http.StatusOK,
			body:       "<html><body><h1>Article</h1><p>Real content</p></body></html>",
			contentTyp: "text/html",
			wantError:  false,
		},
	}

	for _, tc := range cases {
		for _, fetchFormat := range []string{"plaintext", "markdown"} {
			t.Run(tc.name+"/"+fetchFormat, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tc.cf != "" {
						w.Header().Set("Cf-Mitigated", tc.cf)
					}
					w.Header().Set("Content-Type", tc.contentTyp)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}))
				defer server.Close()

				tool, err := NewWebFetchTool(4000, fetchFormat, testFetchLimit)
				if err != nil {
					t.Fatalf("NewWebFetchTool() error: %v", err)
				}
				result := tool.Execute(context.Background(), map[string]any{"url": server.URL})
				if result.IsError != tc.wantError {
					t.Fatalf("IsError = %v, want %v: %s", result.IsError, tc.wantError, result.ForLLM)
				}
				if tc.wantError && !strings.Contains(strings.ToLower(result.ForLLM), "site protection blocked") {
					t.Fatalf("blocked message missing: %s", result.ForLLM)
				}
			})
		}
	}
}
