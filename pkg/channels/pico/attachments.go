package pico

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxPicoAttachmentTextBytes bounds the extracted text of a single file
	// attachment. The launcher API already truncates to this size; the channel
	// enforces it again for clients that talk to the gateway directly.
	maxPicoAttachmentTextBytes = 64 << 10
	// maxPicoAttachmentTotalTextBytes bounds all file attachments in one message.
	maxPicoAttachmentTotalTextBytes = 128 << 10
	maxPicoAttachments              = 5
	maxPicoAttachmentNameLen        = 120
	maxPicoAttachmentTypeLen        = 100
)

const (
	attachmentBeginMarker = "----- BEGIN ATTACHMENT -----"
	attachmentEndMarker   = "----- END ATTACHMENT -----"
)

// picoFileAttachment carries extracted text for a non-image attachment. Images
// keep using inline data URLs handled by parseInlineImageMedia.
type picoFileAttachment struct {
	Filename    string
	ContentType string
	Text        string
}

// parseFileAttachments extracts text attachments from the payload's
// "attachments" array. Entries with other types are ignored so image handling
// stays backwards compatible.
func parseFileAttachments(raw any) ([]picoFileAttachment, error) {
	if raw == nil {
		return nil, nil
	}

	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("attachments must be an array")
	}

	attachments := make([]picoFileAttachment, 0, len(values))
	totalTextBytes := 0
	for i, item := range values {
		attachment, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("attachments[%d]: attachment must be an object", i)
		}

		attachmentType, _ := attachment["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(attachmentType), "file") {
			continue
		}
		if len(attachments) >= maxPicoAttachments {
			return nil, fmt.Errorf("a message can include at most %d file attachments", maxPicoAttachments)
		}

		text, _ := attachment["text"].(string)
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("attachments[%d]: file attachment text is empty", i)
		}
		if len(text) > maxPicoAttachmentTextBytes {
			return nil, fmt.Errorf("attachments[%d]: file attachment text exceeds %d KB",
				i, maxPicoAttachmentTextBytes>>10)
		}
		totalTextBytes += len(text)
		if totalTextBytes > maxPicoAttachmentTotalTextBytes {
			return nil, fmt.Errorf("file attachments in a message exceed %d KB in total",
				maxPicoAttachmentTotalTextBytes>>10)
		}

		filename, _ := attachment["filename"].(string)
		contentType, _ := attachment["content_type"].(string)

		attachments = append(attachments, picoFileAttachment{
			Filename:    sanitizeAttachmentName(filename),
			ContentType: sanitizeAttachmentContentType(contentType),
			Text:        sanitizeAttachmentText(text),
		})
	}

	return attachments, nil
}

// appendAttachmentBlocks appends file attachment text to the user content in a
// delimited form so the agent can tell the user's message apart from the file.
func appendAttachmentBlocks(content string, attachments []picoFileAttachment) string {
	for _, attachment := range attachments {
		block := formatAttachmentBlock(attachment)
		if block == "" {
			continue
		}
		if strings.TrimSpace(content) == "" {
			content = block
			continue
		}
		content += "\n\n" + block
	}
	return content
}

func formatAttachmentBlock(attachment picoFileAttachment) string {
	if attachment.Text == "" {
		return ""
	}

	header := "[Attached file: " + attachment.Filename
	if attachment.ContentType != "" {
		header += " (" + attachment.ContentType + ")"
	}
	header += "]"

	return header + "\n" + attachmentBeginMarker + "\n" + attachment.Text + "\n" + attachmentEndMarker
}

func sanitizeAttachmentName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "attachment"
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '[' || r == ']':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, name)
	if utf8.RuneCountInString(name) > maxPicoAttachmentNameLen {
		name = string([]rune(name)[:maxPicoAttachmentNameLen])
	}
	return strings.TrimSpace(name)
}

func sanitizeAttachmentContentType(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if len(value) > maxPicoAttachmentTypeLen {
		value = value[:maxPicoAttachmentTypeLen]
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("/.+-", r) {
			return r
		}
		return -1
	}, value)
	return value
}

func sanitizeAttachmentText(raw string) string {
	text := strings.ToValidUTF8(raw, "")
	text = strings.ReplaceAll(text, "\x00", "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	// Prevent file content from closing the delimiter block early.
	text = strings.ReplaceAll(text, attachmentBeginMarker, "----- BEGIN ATTACHMENT ---")
	text = strings.ReplaceAll(text, attachmentEndMarker, "----- END ATTACHMENT ---")
	return strings.TrimSpace(text)
}
