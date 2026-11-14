package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestBrowserTool_IntegrationLocalPage(t *testing.T) {
	if os.Getenv("PICOCLAW_BROWSER_INTEGRATION") == "" {
		t.Skip("set PICOCLAW_BROWSER_INTEGRATION=1 to run the real browser test")
	}

	page := `<!doctype html><html><head><title>Tool test</title></head><body>
<button onclick="document.getElementById('r').textContent='Clicked!'">Change</button>
<p id="r">Waiting</p></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("PICOCLAW_HOME", filepath.Join(home, ".picoclaw"))
	t.Setenv("AGENT_BROWSER_SOCKET_DIR", filepath.Join(home, "sockets"))
	t.Setenv("AGENT_BROWSER_NAMESPACE", "picoclaw-tool-test")

	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	cfg := config.DefaultConfig().Tools.Browser
	cfg.Enabled = true
	cfg.AllowedDomains = []string{"127.0.0.1"}
	cfg.Headless = configBoolPointer(true)
	cfg.MaxOutput = 8000
	cfg.TimeoutSeconds = 60

	tool, err := NewBrowserTool(workspace, cfg)
	if err != nil {
		t.Fatalf("NewBrowserTool() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if result := tool.Execute(ctx, map[string]any{"action": "open", "url": server.URL}); result.IsError {
		t.Fatalf("open error = %v", result.ForLLM)
	}
	snapshot := tool.Execute(ctx, map[string]any{"action": "snapshot"})
	if snapshot.IsError {
		t.Fatalf("snapshot error = %v", snapshot.ForLLM)
	}
	ref := firstToolRef(snapshot.ForLLM, "Change")
	if ref == "" {
		t.Fatalf("snapshot has no Change ref:\n%s", snapshot.ForLLM)
	}
	if result := tool.Execute(ctx, map[string]any{"action": "click", "ref": ref}); result.IsError {
		t.Fatalf("click error = %v", result.ForLLM)
	}
	after := tool.Execute(ctx, map[string]any{"action": "snapshot", "interactive": false})
	if after.IsError || !strings.Contains(after.ForLLM, "Clicked!") {
		t.Fatalf("click did not update the page:\n%s", after.ForLLM)
	}

	shot := tool.Execute(ctx, map[string]any{"action": "screenshot", "filename": "shot.png"})
	if shot.IsError {
		t.Fatalf("screenshot error = %v", shot.ForLLM)
	}
	if _, err := os.Stat(filepath.Join(workspace, "browser", "shot.png")); err != nil {
		t.Fatalf("screenshot file missing: %v", err)
	}

	if result := tool.Execute(ctx, map[string]any{"action": "close"}); result.IsError {
		t.Fatalf("close error = %v", result.ForLLM)
	}
}

func configBoolPointer(value bool) *bool { return &value }

func firstToolRef(output, name string) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, name) {
			continue
		}
		start := strings.Index(line, "[ref=")
		if start < 0 {
			continue
		}
		rest := line[start+len("[ref="):]
		end := strings.IndexByte(rest, ']')
		if end <= 0 {
			continue
		}
		return "@" + rest[:end]
	}
	return ""
}
