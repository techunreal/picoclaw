package tools

import (
	"strings"
	"testing"
)

func TestRedactCommandRemovesCommandFromShellOutput(t *testing.T) {
	command := `agent-browser open https://example.com && echo done`
	stderr := "At line:1 char:1\n+ agent-browser open https://example.com && echo done\n" +
		"+ ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~\n" +
		"The token '&&' is not a valid statement separator in this version."

	got := redactCommand(stderr, command)
	if strings.Contains(got, "agent-browser open") {
		t.Fatalf("redacted output still contains the command: %q", got)
	}
	if strings.Contains(got, "https://example.com") {
		t.Fatalf("redacted output still contains command arguments: %q", got)
	}
	if !strings.Contains(got, "[command]") {
		t.Fatalf("redacted output should mark the command: %q", got)
	}
	if !strings.Contains(got, "not a valid statement separator") {
		t.Fatalf("redaction removed the underlying error: %q", got)
	}
}

func TestRedactCommandKeepsOutputWithoutCommand(t *testing.T) {
	got := redactCommand("plain stderr line", "")
	if got != "plain stderr line" {
		t.Fatalf("unexpected output: %q", got)
	}
}
