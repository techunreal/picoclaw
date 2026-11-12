package tools

import (
	"strings"
	"testing"
)

func TestBoundedBufferCapsOutput(t *testing.T) {
	buf := newBoundedBuffer(64)
	payload := strings.Repeat("a", 1024)

	n, err := buf.Write([]byte(payload))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write() n = %d, want %d", n, len(payload))
	}
	if buf.Len() > 64 {
		t.Fatalf("buffer length = %d, want <= 64", buf.Len())
	}
	got := buf.String()
	if !strings.Contains(got, outputTruncateMarker) {
		t.Fatal("expected truncation marker in capped output")
	}
	if len(got) > 64+len(outputTruncateMarker) {
		t.Fatalf("output length = %d, want bounded", len(got))
	}
}

func TestBoundedBufferKeepsSmallOutput(t *testing.T) {
	buf := newBoundedBuffer(64)
	if _, err := buf.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("String() = %q, want %q", got, "hello")
	}
}
