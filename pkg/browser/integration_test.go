package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestClient_IntegrationLocalPage drives a real headless browser. It is opt-in
// because CI images may not ship Chrome/Edge:
//
//	PICOCLAW_BROWSER_INTEGRATION=1 go test ./pkg/browser -run Integration
func TestClient_IntegrationLocalPage(t *testing.T) {
	if os.Getenv("PICOCLAW_BROWSER_INTEGRATION") == "" {
		t.Skip("set PICOCLAW_BROWSER_INTEGRATION=1 to run the real browser test")
	}

	page := `<!doctype html><html><head><title>Browser test</title></head><body>
<h1>Browser test page</h1>
<button id="change" onclick="document.getElementById('result').textContent='Clicked!'">Change</button>
<input id="name" aria-label="Name" />
<p id="result">Waiting</p>
</body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://example.org/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("AGENT_BROWSER_SOCKET_DIR", filepath.Join(home, "sockets"))
	t.Setenv("AGENT_BROWSER_NAMESPACE", "picoclaw-test")

	client, err := New(Options{
		AllowedDomains: []string{"127.0.0.1"},
		Headless:       true,
		MaxOutput:      8000,
		Timeout:        60 * time.Second,
		StateDir:       filepath.Join(home, "browser-state"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if _, err := client.Run(ctx, "integration", "open", server.URL); err != nil {
		t.Fatalf("open error = %v", err)
	}
	snapshot, err := client.Run(ctx, "integration", "snapshot", "--compact", "-i")
	if err != nil {
		t.Fatalf("snapshot error = %v", err)
	}
	ref := firstRef(snapshot, "Change")
	if ref == "" {
		t.Fatalf("snapshot has no Change button ref:\n%s", snapshot)
	}
	if _, err := client.Run(ctx, "integration", "click", ref); err != nil {
		t.Fatalf("click error = %v", err)
	}
	after, err := client.Run(ctx, "integration", "snapshot")
	if err != nil {
		t.Fatalf("second snapshot error = %v", err)
	}
	if !strings.Contains(after, "Clicked!") {
		t.Fatalf("click did not update page:\n%s", after)
	}

	if err := client.ValidateURL("https://example.org/"); err == nil {
		t.Fatal("ValidateURL allowed a domain outside the allowlist")
	}
	// The proxy blocks the cross-domain redirect; agent-browser may still report
	// the open as successful while keeping the original URL.
	_, _ = client.Run(ctx, "integration", "open", server.URL+"/redirect")
	if urlOut, err := client.Run(ctx, "integration", "get", "url"); err == nil &&
		strings.Contains(urlOut, "example.org") {
		t.Fatalf("browser navigated to a non-allowed redirect target: %s", urlOut)
	}
	// Recover the allowed page after the blocked redirect.
	if _, err := client.Run(ctx, "integration", "open", server.URL); err != nil {
		t.Fatalf("reopen allowed page error = %v", err)
	}

	if _, err := client.Close(ctx, "integration"); err != nil {
		t.Fatalf("close error = %v", err)
	}
}

var refPattern = regexp.MustCompile(`\[ref=(e\d+)\]`)

func firstRef(snapshot, name string) string {
	for _, line := range strings.Split(snapshot, "\n") {
		if strings.Contains(line, name) {
			if match := refPattern.FindStringSubmatch(line); len(match) == 2 {
				return "@" + match[1]
			}
		}
	}
	return ""
}
