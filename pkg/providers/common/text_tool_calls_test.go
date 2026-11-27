package common

import (
	"strings"
	"testing"
)

const textToolSample = `Before. [tool_use: edit_file<arg_key:abc123>path</arg_key:abc123><arg_value:abc123>C:\work\MEMORY.md</arg_value:abc123><arg_key:abc123>old_text</arg_key:abc123><arg_value:abc123>old line</arg_value:abc123><arg_key:abc123>count</arg_key:abc123><arg_value:abc123>3</arg_value:abc123></tool_call:abc123></tool_calls:abc123> After.`

func TestParseTextToolCalls(t *testing.T) {
	clean, calls := ParseTextToolCalls(textToolSample)
	if clean != "Before.  After." {
		t.Fatalf("clean = %q", clean)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %#v", calls)
	}
	call := calls[0]
	if call.ID != "abc123" || call.Name != "edit_file" {
		t.Fatalf("call = %#v", call)
	}
	if call.Arguments["path"] != `C:\work\MEMORY.md` {
		t.Fatalf("path = %#v", call.Arguments["path"])
	}
	if call.Arguments["old_text"] != "old line" {
		t.Fatalf("old_text = %#v", call.Arguments["old_text"])
	}
	if call.Arguments["count"] != float64(3) {
		t.Fatalf("count = %#v", call.Arguments["count"])
	}
}

func TestParseTextToolCallsMultiple(t *testing.T) {
	content := `[tool_use: read_file<arg_key:a>path</arg_key:a><arg_value:a>a.txt</arg_value:a></tool_call:a></tool_calls:a>
[tool_use: read_file<arg_key:b>path</arg_key:b><arg_value:b>b.txt</arg_value:b></tool_call:b></tool_calls:b>`
	clean, calls := ParseTextToolCalls(content)
	if clean != "" {
		t.Fatalf("clean = %q", clean)
	}
	if len(calls) != 2 || calls[0].Arguments["path"] != "a.txt" || calls[1].Arguments["path"] != "b.txt" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestParseTextToolCallsDropsPartialBlock(t *testing.T) {
	content := "Visible [tool_use: edit_file<arg_key:x>path</arg_key:x><arg_value:x>secret"
	clean, calls := ParseTextToolCalls(content)
	if clean != "Visible" {
		t.Fatalf("clean = %q", clean)
	}
	if len(calls) != 0 {
		t.Fatalf("partial tool call must not execute: %#v", calls)
	}
}

func TestNormalizeTextToolCalls(t *testing.T) {
	response := &LLMResponse{Content: textToolSample, FinishReason: "stop"}
	NormalizeTextToolCalls(response)
	if strings.Contains(response.Content, "tool_use") {
		t.Fatalf("content still contains markup: %q", response.Content)
	}
	if len(response.ToolCalls) != 1 || response.FinishReason != "tool_calls" {
		t.Fatalf("response = %#v", response)
	}

	existing := &LLMResponse{
		Content:   textToolSample,
		ToolCalls: []ToolCall{{ID: "structured", Name: "read_file"}},
	}
	NormalizeTextToolCalls(existing)
	if len(existing.ToolCalls) != 1 || existing.ToolCalls[0].ID != "structured" {
		t.Fatalf("structured call must win: %#v", existing.ToolCalls)
	}
}

func TestVisibleStreamContent(t *testing.T) {
	if got := VisibleStreamContent("Hello [tool_use: edit_file"); got != "Hello" {
		t.Fatalf("visible = %q", got)
	}
	if got := VisibleStreamContent("Hello world"); got != "Hello world" {
		t.Fatalf("visible = %q", got)
	}
}
