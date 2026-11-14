package browser

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestProxy(t *testing.T, domains []string) (*allowlistProxy, *url.URL) {
	t.Helper()
	proxy, err := startAllowlistProxy()
	if err != nil {
		t.Fatalf("startAllowlistProxy() error = %v", err)
	}
	proxy.setAllowlist(domains, false)
	parsed, err := url.Parse(proxy.url)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return proxy, parsed
}

func TestAllowlistProxy_HTTPAllowedAndBlocked(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello from target"))
	}))
	defer target.Close()

	_, proxyURL := newTestProxy(t, []string{"127.0.0.1"})
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("allowed request error = %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "hello from target") {
		t.Fatalf("status = %d body = %q", resp.StatusCode, body)
	}

	blockedProxy, blockedURL := newTestProxy(t, []string{"example.com"})
	blockedClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(blockedURL)}}
	resp, err = blockedClient.Get(target.URL)
	if err != nil {
		t.Fatalf("blocked request error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("blocked status = %d, want 403", resp.StatusCode)
	}
	_ = blockedProxy
}

func TestAllowlistProxy_HTTPSAllowedAndBlocked(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secure hello"))
	}))
	defer target.Close()

	_, proxyURL := newTestProxy(t, []string{"127.0.0.1"})
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("allowed CONNECT error = %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "secure hello") {
		t.Fatalf("body = %q", body)
	}

	_, blockedURL := newTestProxy(t, []string{"example.com"})
	blockedClient := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(blockedURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if _, err := blockedClient.Get(target.URL); err == nil {
		t.Fatal("blocked CONNECT should fail")
	}
}

func TestAllowlistProxy_RequiresProxyAuth(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	proxy, proxyURL := newTestProxy(t, []string{"127.0.0.1"})
	noAuth := *proxyURL
	noAuth.User = nil
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(&noAuth)}}

	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("request error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want 407", resp.StatusCode)
	}
	_ = proxy
}
