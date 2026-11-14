package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/browser"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/constants"
)

type browserRunner interface {
	Run(ctx context.Context, session string, argv ...string) (string, error)
	NavigationHost(raw string) (string, error)
	IsDomainAllowed(host string) bool
}

type approvalAsker interface {
	Request(ctx context.Context, domain string, timeout time.Duration) browser.ApprovalDecision
}

// BrowserTool exposes the agent-browser CLI as a single, compact tool.
type BrowserTool struct {
	client      browserRunner
	workspace   string
	allowRemote bool
	approvals   approvalAsker
	allowTemp   func(host string, ttl time.Duration)
	allowAlways func(host string)
}

// NewBrowserTool creates the browser tool. It fails when the allowlist is
// empty or the agent-browser executable cannot be found.
func NewBrowserTool(workspace string, cfg config.BrowserToolsConfig) (*BrowserTool, error) {
	client, err := browser.New(browser.Options{
		BinaryPath:      cfg.BinaryPath,
		BrowserPath:     cfg.BrowserPath,
		Headless:        cfg.IsHeadless(),
		AllowedDomains:  cfg.AllowedDomains,
		AllowAllDomains: cfg.AllowAllDomains,
		MaxOutput:       cfg.GetMaxOutput(),
		Timeout:         time.Duration(cfg.GetTimeoutSeconds()) * time.Second,
		StateDir:        filepath.Join(config.GetHome(), "browser"),
	})
	if err != nil {
		return nil, err
	}
	return &BrowserTool{
		client:      client,
		workspace:   workspace,
		allowRemote: cfg.AllowRemote,
		approvals:   browser.DefaultApprovals(),
		allowTemp:   browser.AllowTemporarily,
		allowAlways: browser.AllowHostAlways,
	}, nil
}

func (t *BrowserTool) Name() string { return "browser" }

func (t *BrowserTool) Description() string {
	return `Drive a real headless browser to open pages, read compact accessibility snapshots, and interact with elements.
Use action=open with a URL, then action=snapshot to get @eN refs, then action=click/fill/press using those refs.
Domains must be on the operator's allowlist. JavaScript eval, file uploads and downloads are blocked by policy.
Re-snapshot after navigation or DOM changes; refs become stale.`
}

func (t *BrowserTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"open", "snapshot", "click", "fill", "press", "scroll", "back", "wait", "screenshot", "close"},
				"description": "Action to perform. open (url), snapshot, click (ref), fill (ref,text), press (key), scroll (direction,amount), back, wait (value), screenshot (filename), close.",
			},
			"url": map[string]any{
				"type":        "string",
				"description": "Absolute http(s) URL for open. Must be on the allowlist.",
			},
			"ref": map[string]any{
				"type":        "string",
				"description": "Element reference from the latest snapshot, for example @e3.",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Text to fill into the referenced input (fill).",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "Key to press, for example Enter, Tab, Escape or ArrowDown (press).",
			},
			"direction": map[string]any{
				"type":        "string",
				"enum":        []string{"up", "down", "left", "right"},
				"description": "Scroll direction (scroll).",
			},
			"amount": map[string]any{
				"type":        "integer",
				"description": "Scroll distance in pixels (scroll), default 500.",
			},
			"interactive": map[string]any{
				"type":        "boolean",
				"description": "Only include interactive elements in snapshot (default true).",
			},
			"depth": map[string]any{
				"type":        "integer",
				"description": "Limit snapshot depth (0 = full).",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Wait target: milliseconds (e.g. 500) or a CSS selector / text (wait).",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "Optional screenshot file name under <workspace>/browser/.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *BrowserTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	channel := ToolChannel(ctx)
	if !t.allowRemote && channel != "" && !constants.IsLocalCommandChannel(channel) {
		return ErrorResult("browser is not allowed from remote channels; enable tools.browser.allow_remote explicitly")
	}

	action := strings.ToLower(strings.TrimSpace(stringArg(args, "action")))
	session := browser.SessionName(channel, ToolChatID(ctx))

	switch action {
	case "open":
		url := strings.TrimSpace(stringArg(args, "url"))
		if url == "" || len(url) > 2048 {
			return ErrorResult("url is required for open")
		}
		host, err := t.client.NavigationHost(url)
		if err != nil {
			return ErrorResult(err.Error())
		}
		if !t.client.IsDomainAllowed(host) {
			decision := t.requestApproval(ctx, host)
			switch decision {
			case browser.AllowOnce:
				t.allowDomainTemporarily(host)
			case browser.AllowAlways:
				t.allowDomainAlways(host)
			default:
				return ErrorResult("The user denied access to " + host + ".")
			}
		}
		return t.run(ctx, session, "Opening page", "open", url)

	case "snapshot":
		argv := []string{"snapshot", "--compact"}
		if value, ok := args["interactive"]; !ok || boolValue(value, true) {
			argv = append(argv, "-i")
		}
		depth := intValue(args, "depth", 0)
		if depth > 0 {
			if depth > 20 {
				depth = 20
			}
			argv = append(argv, "--depth", fmt.Sprintf("%d", depth))
		}
		return t.run(ctx, session, "Snapshot", argv...)

	case "click":
		ref := strings.TrimSpace(stringArg(args, "ref"))
		if !validBrowserRef(ref) {
			return ErrorResult("ref is required; use a @eN reference from the latest snapshot")
		}
		return t.run(ctx, session, "Click", "click", ref)

	case "fill":
		ref := strings.TrimSpace(stringArg(args, "ref"))
		if !validBrowserRef(ref) {
			return ErrorResult("ref is required; use a @eN reference from the latest snapshot")
		}
		text := stringArg(args, "text")
		if len(text) > 4000 {
			return ErrorResult("text is too long (max 4000 characters)")
		}
		return t.run(ctx, session, "Fill", "fill", ref, text)

	case "press":
		key := strings.TrimSpace(stringArg(args, "key"))
		if key == "" || len(key) > 40 {
			return ErrorResult("key is required for press")
		}
		return t.run(ctx, session, "Press", "press", key)

	case "scroll":
		direction := strings.ToLower(strings.TrimSpace(stringArg(args, "direction")))
		switch direction {
		case "up", "down", "left", "right":
		default:
			direction = "down"
		}
		amount := intValue(args, "amount", 500)
		if amount < 1 || amount > 5000 {
			amount = 500
		}
		return t.run(ctx, session, "Scroll", "scroll", direction, fmt.Sprintf("%d", amount))

	case "back":
		return t.run(ctx, session, "Back", "back")

	case "wait":
		value := strings.TrimSpace(stringArg(args, "value"))
		if value == "" || len(value) > 200 {
			return ErrorResult("value is required for wait (milliseconds or selector)")
		}
		return t.run(ctx, session, "Wait", "wait", value)

	case "screenshot":
		path, err := t.screenshotPath(stringArg(args, "filename"))
		if err != nil {
			return ErrorResult(err.Error())
		}
		result, err := t.client.Run(ctx, session, "screenshot", path)
		if err != nil {
			return browserErrorResult(err, result)
		}
		message := fmt.Sprintf("Screenshot saved to %s", path)
		if strings.TrimSpace(result) != "" {
			message += "\n" + result
		}
		return NewToolResult(message)

	case "close":
		return t.run(ctx, session, "Close", "close")

	default:
		return ErrorResult("unknown browser action")
	}
}

func (t *BrowserTool) run(ctx context.Context, session, label string, argv ...string) *ToolResult {
	result, err := t.client.Run(ctx, session, argv...)
	if err != nil {
		return browserErrorResult(err, result)
	}
	if strings.TrimSpace(result) == "" {
		return NewToolResult(label + " completed.")
	}
	return NewToolResult(result)
}

func browserErrorResult(err error, output string) *ToolResult {
	message := err.Error()
	if strings.TrimSpace(output) != "" {
		message += "\n" + strings.TrimSpace(output)
	}
	return ErrorResult(message)
}

func (t *BrowserTool) screenshotPath(filename string) (string, error) {
	dir := filepath.Join(t.workspace, "browser")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to prepare screenshot directory")
	}
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "page-" + time.Now().Format("20060102-150405") + ".png"
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = sanitizeBrowserFilename(name)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid screenshot file name")
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		name += ".png"
	}
	return filepath.Join(dir, name), nil
}

func sanitizeBrowserFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return -1
		}
		return r
	}, name)
	if len(name) > 120 {
		name = name[:120]
	}
	return strings.TrimSpace(name)
}

func (t *BrowserTool) requestApproval(ctx context.Context, host string) browser.ApprovalDecision {
	asker := t.approvals
	if asker == nil {
		asker = browser.DefaultApprovals()
	}
	return asker.Request(ctx, host, 90*time.Second)
}

func (t *BrowserTool) allowDomainTemporarily(host string) {
	if t.allowTemp != nil {
		t.allowTemp(host, 2*time.Minute)
		return
	}
	browser.AllowTemporarily(host, 2*time.Minute)
}

func (t *BrowserTool) allowDomainAlways(host string) {
	if t.allowAlways != nil {
		t.allowAlways(host)
		return
	}
	browser.AllowHostAlways(host)
}

func validBrowserRef(ref string) bool {
	if ref == "" || len(ref) > 200 {
		return false
	}
	if strings.HasPrefix(ref, "@e") {
		for _, r := range ref[2:] {
			if r < '0' || r > '9' {
				return false
			}
		}
		return len(ref) > 2
	}
	// Allow a bounded CSS selector as a fallback.
	return strings.ContainsAny(ref, "#.[]")
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func boolValue(value any, fallback bool) bool {
	parsed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return parsed
}

func intValue(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return fallback
	}
}
