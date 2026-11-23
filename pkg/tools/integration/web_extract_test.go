package integrationtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExtractReadableContent_MainRoot(t *testing.T) {
	fixture := `<!doctype html><html><head><title>Article Title</title></head><body>
<header class="site-header"><nav>Outer Site Menu</nav></header>
<main>
  <header class="site-header"><nav>Inner Site Menu</nav></header>
  <article>
    <header><h1>Article Title</h1><p>By Jane Doe</p></header>
    <p>First article paragraph.</p>
    <div hidden>Hidden attribute text</div>
    <div aria-hidden="true">Aria hidden text</div>
    <div style="display:none">CSS hidden text</div>
    <div data-nosnippet>Author bio text</div>
    <aside>Related links</aside>
    <p>Second article paragraph.</p>
    <p>` + strings.Repeat("Filler article text. ", 40) + `</p>
  </article>
</main>
<footer>Footer text</footer>
<script>script text</script>
</body></html>`

	content := extractReadableContent(fixture)
	if !content.ok {
		t.Fatal("extractReadableContent() ok=false, want true")
	}
	if content.source != "main" {
		t.Fatalf("source = %q, want main", content.source)
	}
	if content.title != "Article Title" {
		t.Fatalf("title = %q, want Article Title", content.title)
	}
	for _, want := range []string{
		"First article paragraph.", "Second article paragraph.", "By Jane Doe",
	} {
		if !strings.Contains(content.text, want) {
			t.Fatalf("text missing %q: %s", want, content.text)
		}
	}
	for _, unwanted := range []string{
		"Outer Site Menu", "Inner Site Menu", "Hidden attribute text",
		"Aria hidden text", "CSS hidden text", "Author bio text",
		"Related links", "Footer text", "script text",
	} {
		if strings.Contains(content.text, unwanted) {
			t.Fatalf("text contains unwanted %q: %s", unwanted, content.text)
		}
	}
	first := strings.Index(content.text, "First article paragraph.")
	if first < 0 || utf8.RuneCountInString(content.text[:first]) > 500 {
		t.Fatalf("first paragraph at %d runes, want < 500", utf8.RuneCountInString(content.text[:first]))
	}
	if strings.Contains(content.html, "Inner Site Menu") || strings.Contains(content.html, "Author bio text") {
		t.Fatalf("cleaned HTML still contains chrome: %s", content.html)
	}
}

func TestExtractReadableContent_NoRootFallsBackToBody(t *testing.T) {
	fixture := `<html><head><title>Simple</title></head><body><p>` +
		strings.Repeat("Simple body text. ", 40) + `</p></body></html>`
	content := extractReadableContent(fixture)
	if !content.ok {
		t.Fatal("extractReadableContent() ok=false, want true")
	}
	if content.source != "text" {
		t.Fatalf("source = %q, want text", content.source)
	}
	if !strings.Contains(content.text, "Simple body text.") {
		t.Fatalf("text = %q, want the body text", content.text)
	}
}

func TestExtractReadableContent_SafetyNetForTinyRoot(t *testing.T) {
	fixture := `<html><body><main><p>Menu</p></main><div id="real"><p>` +
		strings.Repeat("Real content. ", 80) + `</p></div></body></html>`
	content := extractReadableContent(fixture)
	if content.ok {
		t.Fatalf("extractReadableContent() ok=true for a tiny root: %s", content.text)
	}
}

func TestWebFetchMainContent_SafetyNetUsesLegacyText(t *testing.T) {
	withPrivateWebFetchHostsAllowed(t)
	fixture := `<html><body><main><p>Menu</p></main><div id="real"><p>` +
		strings.Repeat("Real content. ", 80) + `</p></div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	tool, err := NewWebFetchTool(12000, "plaintext", testFetchLimit)
	if err != nil {
		t.Fatalf("NewWebFetchTool() error: %v", err)
	}
	result := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if result.IsError {
		t.Fatalf("Execute() error: %s", result.ForLLM)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.ForLLM), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["extractor"] != "text" {
		t.Fatalf("extractor = %v, want text (legacy fallback)", payload["extractor"])
	}
	if text, _ := payload["text"].(string); !strings.Contains(text, "Real content.") {
		t.Fatalf("text = %q, want the legacy fallback content", text)
	}
}

func TestWebFetchOffsetContinuesText(t *testing.T) {
	withPrivateWebFetchHostsAllowed(t)
	body := strings.Repeat("a", 1000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	tool, err := NewWebFetchTool(100, "plaintext", testFetchLimit)
	if err != nil {
		t.Fatalf("NewWebFetchTool() error: %v", err)
	}

	first := readFetchJSON(t, tool.Execute(context.Background(), map[string]any{"url": server.URL}))
	if first["total_chars"] != float64(1000) {
		t.Fatalf("total_chars = %v, want 1000", first["total_chars"])
	}
	if first["next_offset"] != float64(100) {
		t.Fatalf("next_offset = %v, want 100", first["next_offset"])
	}
	if first["truncated"] != true {
		t.Fatal("first response should be truncated")
	}

	second := readFetchJSON(t, tool.Execute(context.Background(), map[string]any{
		"url":    server.URL,
		"offset": float64(100),
	}))
	if second["next_offset"] != float64(200) {
		t.Fatalf("second next_offset = %v, want 200", second["next_offset"])
	}

	last := readFetchJSON(t, tool.Execute(context.Background(), map[string]any{
		"url":    server.URL,
		"offset": float64(950),
	}))
	if last["truncated"] != false {
		t.Fatal("final slice should not be truncated")
	}
	if _, ok := last["next_offset"]; ok {
		t.Fatalf("final slice should have no next_offset: %v", last["next_offset"])
	}
	if text, _ := last["text"].(string); len(text) != 50 {
		t.Fatalf("final text length = %d, want 50", len(text))
	}

	beyond := readFetchJSON(t, tool.Execute(context.Background(), map[string]any{
		"url":    server.URL,
		"offset": float64(1200),
	}))
	if beyond["truncated"] != false {
		t.Fatal("offset beyond the end should not be truncated")
	}
	if text, _ := beyond["text"].(string); text != "" {
		t.Fatalf("offset beyond the end text = %q, want empty", text)
	}
}

func TestWebFetchMarkdownUsesMainRoot(t *testing.T) {
	withPrivateWebFetchHostsAllowed(t)
	fixture := `<html><head><title>Markdown Article</title></head><body>
<header class="site-header"><nav>Site Menu</nav></header>
<main><article><h1>Markdown Article</h1><p>First markdown paragraph.</p><p>Second markdown paragraph.</p><p>` + strings.Repeat("Filler markdown text. ", 40) + `</p></article></main>
<footer>Footer</footer></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	tool, err := NewWebFetchTool(12000, "markdown", testFetchLimit)
	if err != nil {
		t.Fatalf("NewWebFetchTool() error: %v", err)
	}
	payload := readFetchJSON(t, tool.Execute(context.Background(), map[string]any{"url": server.URL}))
	if payload["extractor"] != "main" {
		t.Fatalf("extractor = %v, want main", payload["extractor"])
	}
	text, _ := payload["text"].(string)
	if !strings.Contains(text, "First markdown paragraph.") {
		t.Fatalf("markdown output missing article: %s", text)
	}
	if strings.Contains(text, "Site Menu") || strings.Contains(text, "Footer") {
		t.Fatalf("markdown output contains chrome: %s", text)
	}
}

func readFetchJSON(t *testing.T, result *ToolResult) map[string]any {
	t.Helper()
	if result.IsError {
		t.Fatalf("Execute() error: %s", result.ForLLM)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.ForLLM), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return payload
}
