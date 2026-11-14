package browser

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// allowlistProxy is a local HTTP CONNECT proxy that only forwards traffic to
// hosts on the configured allowlist. Running the allowlist here, instead of
// relying on Chromium request interception, keeps the browser stable on Edge and
// still blocks redirects and sub-resource requests to other domains.
type allowlistProxy struct {
	listener net.Listener
	server   *http.Server
	url      string
	user     string
	pass     string
	domains  atomic.Pointer[[]string]
	allowAll atomic.Bool
}

var (
	sharedProxy     *allowlistProxy
	sharedProxyOnce sync.Once
	sharedProxyErr  error
)

func sharedAllowlistProxy() (*allowlistProxy, error) {
	sharedProxyOnce.Do(func() {
		sharedProxy, sharedProxyErr = startAllowlistProxy()
	})
	return sharedProxy, sharedProxyErr
}

func startAllowlistProxy() (*allowlistProxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start browser allowlist proxy: %w", err)
	}

	user, err := randomToken(16)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	pass, err := randomToken(24)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	proxy := &allowlistProxy{
		listener: listener,
		user:     user,
		pass:     pass,
	}
	proxy.url = fmt.Sprintf("http://%s:%s@%s/", user, pass, listener.Addr().String())

	empty := []string{}
	proxy.domains.Store(&empty)
	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = proxy.server.Serve(listener)
	}()
	return proxy, nil
}

func (p *allowlistProxy) proxyURL() string { return p.url }

func (p *allowlistProxy) setAllowlist(domains []string, allowAll bool) {
	normalized := normalizeDomainPatterns(domains)
	p.domains.Store(&normalized)
	p.allowAll.Store(allowAll)
}

func (p *allowlistProxy) hostAllowed(host string) bool {
	if p.allowAll.Load() {
		return true
	}
	patterns := p.domains.Load()
	if patterns == nil {
		return false
	}
	return HostAllowed(host, *patterns)
}

func (p *allowlistProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.authorized(r) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="picoclaw-browser"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}

	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

func (p *allowlistProxy) authorized(r *http.Request) bool {
	header := r.Header.Get("Proxy-Authorization")
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	expected := p.user + ":" + p.pass
	return subtle.ConstantTimeCompare(decoded, []byte(expected)) == 1
}

func (p *allowlistProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
		port = "443"
	}
	if !p.hostAllowed(host) {
		http.Error(w, "domain is not in the browser allowlist: "+host, http.StatusForbidden)
		return
	}

	target, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 15*time.Second)
	if err != nil {
		http.Error(w, "failed to connect to "+host, http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = target.Close()
		http.Error(w, "proxy hijacking is not supported", http.StatusInternalServerError)
		return
	}
	client, _, err := hijacker.Hijack()
	if err != nil {
		_ = target.Close()
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	go func() {
		_, _ = io.Copy(target, client)
		_ = target.Close()
	}()
	go func() {
		_, _ = io.Copy(client, target)
		_ = client.Close()
	}()
}

func (p *allowlistProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL == nil || r.URL.Hostname() == "" {
		http.Error(w, "invalid proxy request", http.StatusBadRequest)
		return
	}
	host := r.URL.Hostname()
	if !p.hostAllowed(host) {
		http.Error(w, "domain is not in the browser allowlist: "+host, http.StatusForbidden)
		return
	}

	outbound := r.Clone(r.Context())
	outbound.RequestURI = ""
	outbound.URL.Scheme = "http"
	outbound.URL.Host = r.URL.Host
	outbound.Header.Del("Proxy-Authorization")
	outbound.Header.Del("Proxy-Connection")
	outbound.Host = r.URL.Host

	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !p.hostAllowed(req.URL.Hostname()) {
				return errors.New("redirect blocked by browser allowlist")
			}
			return nil
		},
	}
	resp, err := client.Do(outbound)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
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
}

func randomToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate proxy credentials: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
