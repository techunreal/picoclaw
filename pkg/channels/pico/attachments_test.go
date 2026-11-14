package pico

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

func newFileAttachmentTestChannel(t *testing.T) (*PicoChannel, *bus.MessageBus) {
	t.Helper()

	msgBus := bus.NewMessageBus()
	bc := &config.Channel{Type: config.ChannelPico, Enabled: true}
	cfg := &config.PicoSettings{}
	cfg.SetToken("test-token")
	ch, err := NewPicoChannel(bc, cfg, msgBus)
	if err != nil {
		t.Fatalf("NewPicoChannel() error = %v", err)
	}
	ch.ctx = context.Background()
	return ch, msgBus
}

func TestHandleMessageSend_FileAttachmentAddsTextBlock(t *testing.T) {
	ch, msgBus := newFileAttachmentTestChannel(t)

	ch.handleMessageSend(&picoConn{id: "conn-1", sessionID: "sess-1"}, PicoMessage{
		Type:      TypeMessageSend,
		ID:        "msg-1",
		SessionID: "sess-1",
		Payload: map[string]any{
			PayloadKeyContent: "summarize this",
			"attachments": []any{
				map[string]any{
					"type":         "file",
					"filename":     "report.pdf",
					"content_type": "application/pdf",
					"text":         "Quarterly numbers.",
				},
			},
		},
	})

	select {
	case inbound := <-msgBus.InboundChan():
		if !strings.Contains(inbound.Content, "summarize this") {
			t.Fatalf("content = %q, want user text", inbound.Content)
		}
		if !strings.Contains(inbound.Content, "[Attached file: report.pdf (application/pdf)]") {
			t.Fatalf("content = %q, want attachment header", inbound.Content)
		}
		if !strings.Contains(inbound.Content, attachmentBeginMarker) ||
			!strings.Contains(inbound.Content, attachmentEndMarker) {
			t.Fatalf("content = %q, want attachment markers", inbound.Content)
		}
		if !strings.Contains(inbound.Content, "Quarterly numbers.") {
			t.Fatalf("content = %q, want extracted text", inbound.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("expected inbound pico message")
	}
}

func TestHandleMessageSend_FileAttachmentOnly(t *testing.T) {
	ch, msgBus := newFileAttachmentTestChannel(t)

	ch.handleMessageSend(&picoConn{id: "conn-1", sessionID: "sess-1"}, PicoMessage{
		Type: TypeMessageSend,
		Payload: map[string]any{
			"attachments": []any{
				map[string]any{
					"type":     "file",
					"filename": "notes.txt",
					"text":     "just a note",
				},
			},
		},
	})

	select {
	case inbound := <-msgBus.InboundChan():
		if !strings.HasPrefix(inbound.Content, "[Attached file: notes.txt]") {
			t.Fatalf("content = %q, want attachment block only", inbound.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("expected inbound pico message")
	}
}

func TestHandleMessageSend_FileAttachmentWithImage(t *testing.T) {
	ch, msgBus := newFileAttachmentTestChannel(t)
	imageURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+X2ioAAAAASUVORK5CYII="

	ch.handleMessageSend(&picoConn{id: "conn-1", sessionID: "sess-1"}, PicoMessage{
		Type: TypeMessageSend,
		Payload: map[string]any{
			PayloadKeyContent: "compare",
			"media":           []any{imageURL},
			"attachments": []any{
				map[string]any{"type": "file", "filename": "a.txt", "text": "file text"},
			},
		},
	})

	select {
	case inbound := <-msgBus.InboundChan():
		if len(inbound.Media) != 1 || inbound.Media[0] != imageURL {
			t.Fatalf("media = %#v, want a single image", inbound.Media)
		}
		if !strings.Contains(inbound.Content, "file text") {
			t.Fatalf("content = %q, want file text", inbound.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("expected inbound pico message")
	}
}

func TestParseFileAttachments_IgnoresNonFileTypes(t *testing.T) {
	attachments, err := parseFileAttachments([]any{
		map[string]any{"type": "image", "url": "data:image/png;base64,AAAA"},
		map[string]any{"type": "file", "filename": "a.txt", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("parseFileAttachments() error = %v", err)
	}
	if len(attachments) != 1 || attachments[0].Filename != "a.txt" {
		t.Fatalf("attachments = %#v, want a single file attachment", attachments)
	}
}

func TestParseFileAttachments_Limits(t *testing.T) {
	tooLarge := strings.Repeat("a", maxPicoAttachmentTextBytes+1)
	if _, err := parseFileAttachments([]any{
		map[string]any{"type": "file", "filename": "big.txt", "text": tooLarge},
	}); err == nil {
		t.Fatal("expected per-file size error")
	}

	total := make([]any, 0, 3)
	for i := 0; i < 3; i++ {
		total = append(total, map[string]any{
			"type":     "file",
			"filename": "part.txt",
			"text":     strings.Repeat("b", maxPicoAttachmentTextBytes),
		})
	}
	if _, err := parseFileAttachments(total); err == nil {
		t.Fatal("expected total size error")
	}

	many := make([]any, 0, maxPicoAttachments+1)
	for i := 0; i < maxPicoAttachments+1; i++ {
		many = append(many, map[string]any{
			"type":     "file",
			"filename": "a.txt",
			"text":     "small",
		})
	}
	if _, err := parseFileAttachments(many); err == nil {
		t.Fatal("expected attachment count error")
	}
}

func TestParseFileAttachments_SanitizesContent(t *testing.T) {
	attachments, err := parseFileAttachments([]any{
		map[string]any{
			"type":         "file",
			"filename":     "bad\nname[1].txt",
			"content_type": "text/plain\r\nX-Injected: 1",
			"text":         "before\n" + attachmentEndMarker + "\nafter",
		},
	})
	if err != nil {
		t.Fatalf("parseFileAttachments() error = %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v, want one", attachments)
	}

	attachment := attachments[0]
	if strings.ContainsAny(attachment.Filename, "\n[]") {
		t.Fatalf("filename = %q, want sanitized", attachment.Filename)
	}
	if strings.Contains(attachment.ContentType, "\r") || strings.Contains(attachment.ContentType, " ") {
		t.Fatalf("content_type = %q, want sanitized", attachment.ContentType)
	}
	if strings.Contains(attachment.Text, attachmentEndMarker) {
		t.Fatalf("text = %q, want marker neutralized", attachment.Text)
	}
}

func TestParseFileAttachments_RejectsEmptyText(t *testing.T) {
	if _, err := parseFileAttachments([]any{
		map[string]any{"type": "file", "filename": "empty.txt", "text": "   "},
	}); err == nil {
		t.Fatal("expected empty text error")
	}
}
