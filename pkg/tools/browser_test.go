package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeBrowserRunner struct {
	validateErr error
	calls       [][]string
	output      string
	runErr      error
}

func (f *fakeBrowserRunner) ValidateURL(raw string) error {
	if f.validateErr != nil {
		return f.validateErr
	}
	if !strings.HasPrefix(raw, "https://") {
		return errors.New("invalid url")
	}
	return nil
}

func (f *fakeBrowserRunner) Run(ctx context.Context, session string, argv ...string) (string, error) {
	f.calls = append(f.calls, append([]string{session}, argv...))
	return f.output, f.runErr
}

func newTestBrowserTool(t *testing.T, runner *fakeBrowserRunner) *BrowserTool {
	t.Helper()
	return &BrowserTool{client: runner, workspace: t.TempDir()}
}

func TestBrowserTool_OpenAndSnapshot(t *testing.T) {
	runner := &fakeBrowserRunner{output: "- link \"Docs\" [ref=e1]"}
	tool := newTestBrowserTool(t, runner)

	result := tool.Execute(context.Background(), map[string]any{
		"action": "open",
		"url":    "https://example.com/",
	})
	if result.IsError {
		t.Fatalf("open error = %v", result.ForLLM)
	}

	result = tool.Execute(context.Background(), map[string]any{
		"action": "snapshot",
		"depth":  float64(4),
	})
	if result.IsError {
		t.Fatalf("snapshot error = %v", result.ForLLM)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(runner.calls))
	}
	snapshotArgs := strings.Join(runner.calls[1], " ")
	for _, want := range []string{"snapshot", "--compact", "-i", "--depth 4"} {
		if !strings.Contains(snapshotArgs, want) {
			t.Fatalf("snapshot args = %q, want %q", snapshotArgs, want)
		}
	}
}

func TestBrowserTool_RejectsDisallowedURL(t *testing.T) {
	runner := &fakeBrowserRunner{}
	tool := newTestBrowserTool(t, runner)

	result := tool.Execute(context.Background(), map[string]any{
		"action": "open",
		"url":    "http://evil.test/",
	})
	if !result.IsError {
		t.Fatal("expected allowlist error")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("browser ran despite validation error: %v", runner.calls)
	}
}

func TestBrowserTool_Actions(t *testing.T) {
	runner := &fakeBrowserRunner{}
	tool := newTestBrowserTool(t, runner)

	cases := []map[string]any{
		{"action": "click", "ref": "@e2"},
		{"action": "fill", "ref": "@e3", "text": "hello world"},
		{"action": "press", "key": "Enter"},
		{"action": "scroll", "direction": "down", "amount": float64(700)},
		{"action": "back"},
		{"action": "wait", "value": "500"},
		{"action": "close"},
	}
	for _, args := range cases {
		result := tool.Execute(context.Background(), args)
		if result.IsError {
			t.Fatalf("%v error = %v", args["action"], result.ForLLM)
		}
	}
	if len(runner.calls) != len(cases) {
		t.Fatalf("calls = %d, want %d", len(runner.calls), len(cases))
	}
	if got := strings.Join(runner.calls[3], " "); !strings.Contains(got, "scroll down 700") {
		t.Fatalf("scroll args = %q", got)
	}
}

func TestBrowserTool_RemoteChannels(t *testing.T) {
	runner := &fakeBrowserRunner{}
	tool := newTestBrowserTool(t, runner)
	remoteCtx := WithToolContext(context.Background(), "telegram", "chat-1")

	result := tool.Execute(remoteCtx, map[string]any{"action": "back"})
	if !result.IsError {
		t.Fatal("browser should be blocked for remote channels by default")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("browser ran for remote channel: %v", runner.calls)
	}

	tool.allowRemote = true
	if result := tool.Execute(remoteCtx, map[string]any{"action": "back"}); result.IsError {
		t.Fatalf("allowed remote action error = %v", result.ForLLM)
	}

	localCtx := WithToolContext(context.Background(), "pico", "pico-session")
	tool.allowRemote = false
	if result := tool.Execute(localCtx, map[string]any{"action": "back"}); result.IsError {
		t.Fatalf("local WebUI action error = %v", result.ForLLM)
	}
}

func TestBrowserTool_ValidatesRefs(t *testing.T) {
	runner := &fakeBrowserRunner{}
	tool := newTestBrowserTool(t, runner)

	for _, args := range []map[string]any{
		{"action": "click"},
		{"action": "click", "ref": "javascript:alert(1)"},
		{"action": "fill", "ref": "@e", "text": "x"},
	} {
		if result := tool.Execute(context.Background(), args); !result.IsError {
			t.Fatalf("args %v should fail", args)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unexpected browser calls: %v", runner.calls)
	}
}

func TestBrowserTool_ScreenshotPathStaysInWorkspace(t *testing.T) {
	runner := &fakeBrowserRunner{}
	tool := newTestBrowserTool(t, runner)

	result := tool.Execute(context.Background(), map[string]any{
		"action":   "screenshot",
		"filename": `..\..\evil.png`,
	})
	if result.IsError {
		t.Fatalf("screenshot error = %v", result.ForLLM)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	path := runner.calls[0][len(runner.calls[0])-1]
	if !strings.HasPrefix(path, tool.workspace) {
		t.Fatalf("screenshot path = %q, want inside workspace %q", path, tool.workspace)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("screenshot dir missing: %v", err)
	}
	if strings.Contains(path, "..") {
		t.Fatalf("screenshot path still contains traversal: %q", path)
	}
}

func TestBrowserTool_UnknownAction(t *testing.T) {
	tool := newTestBrowserTool(t, &fakeBrowserRunner{})
	result := tool.Execute(context.Background(), map[string]any{"action": "eval"})
	if !result.IsError {
		t.Fatal("eval should not be exposed")
	}
}
