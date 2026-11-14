package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/h2non/filetype"
	"github.com/ledongthuc/pdf"

	"github.com/sipeed/picoclaw/pkg/audio/asr"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
)

const (
	// maxChatAttachmentBytes bounds a single uploaded attachment. Images are
	// still sent inline by the chat composer, so this only covers PDF, text and
	// audio files.
	maxChatAttachmentBytes = 10 << 20
	// maxChatTextFileBytes bounds plain-text reads before they are extracted.
	maxChatTextFileBytes = 2 << 20
	// maxExtractedAttachmentText bounds the text that is injected into a chat
	// message. Anything beyond this is truncated and marked in the response.
	maxExtractedAttachmentText = 64 << 10
	// maxChatAttachmentNameLen bounds the filename echoed back to the client.
	maxChatAttachmentNameLen = 120
	// chatAttachmentTranscribeTimeout bounds the ASR request triggered from an
	// upload so a slow provider cannot hold the HTTP request forever.
	chatAttachmentTranscribeTimeout = 120 * time.Second
)

// detectTranscriber is a seam for tests; production resolves the configured
// speech model exactly like the gateway does.
var detectTranscriber = asr.DetectTranscriber

var errNoExtractableText = errors.New("no extractable text")

// chatTextExtensions lists the plain-text formats accepted as chat attachments.
// The file must also be valid UTF-8 and free of NUL bytes.
var chatTextExtensions = map[string]string{
	".txt":      "text/plain",
	".text":     "text/plain",
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".csv":      "text/csv",
	".tsv":      "text/tab-separated-values",
	".json":     "application/json",
	".jsonl":    "application/json",
	".ndjson":   "application/json",
	".log":      "text/plain",
	".yaml":     "application/yaml",
	".yml":      "application/yaml",
	".xml":      "application/xml",
	".html":     "text/html",
	".htm":      "text/html",
	".css":      "text/css",
	".js":       "text/javascript",
	".mjs":      "text/javascript",
	".cjs":      "text/javascript",
	".ts":       "text/typescript",
	".tsx":      "text/typescript",
	".jsx":      "text/javascript",
	".go":       "text/x-go",
	".py":       "text/x-python",
	".rb":       "text/x-ruby",
	".rs":       "text/x-rust",
	".java":     "text/x-java",
	".kt":       "text/x-kotlin",
	".c":        "text/x-c",
	".h":        "text/x-c",
	".cpp":      "text/x-c++",
	".hpp":      "text/x-c++",
	".cs":       "text/x-csharp",
	".sh":       "text/x-shellscript",
	".ps1":      "text/x-powershell",
	".sql":      "text/x-sql",
	".ini":      "text/plain",
	".toml":     "text/plain",
	".cfg":      "text/plain",
	".conf":     "text/plain",
	".env":      "text/plain",
	".srt":      "text/plain",
	".vtt":      "text/vtt",
}

// chatAudioExtensions lists accepted audio container extensions.
var chatAudioExtensions = map[string]string{
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".opus": "audio/opus",
	".flac": "audio/flac",
	".aac":  "audio/aac",
	".wma":  "audio/x-ms-wma",
	".amr":  "audio/amr",
}

type chatAttachmentResponse struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Kind        string `json:"kind"`
	Size        int64  `json:"size"`
	Text        string `json:"text,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Transcript  bool   `json:"transcript,omitempty"`
}

func (h *Handler) registerChatAttachmentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/chat/attachments", h.handleChatAttachmentUpload)
}

// handleChatAttachmentUpload validates a chat attachment and prepares the text
// that will be injected into the conversation. Images are intentionally not
// accepted here: the composer sends them inline as data URLs.
func (h *Handler) handleChatAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxChatAttachmentBytes+(1<<20))

	reader, err := r.MultipartReader()
	if err != nil {
		writeChatAttachmentError(w, http.StatusBadRequest, "invalid_request",
			"Expected a multipart/form-data upload.")
		return
	}

	var (
		name        string
		contentType string
		data        []byte
	)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeChatAttachmentError(w, http.StatusRequestEntityTooLarge, "too_large",
					"This file is too large. The limit is 10 MB.")
				return
			}
			writeChatAttachmentError(w, http.StatusBadRequest, "invalid_request",
				"Failed to read the upload.")
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}

		name = sanitizeChatAttachmentFilename(part.FileName())
		contentType = strings.TrimSpace(part.Header.Get("Content-Type"))
		data, err = io.ReadAll(io.LimitReader(part, maxChatAttachmentBytes+1))
		_ = part.Close()
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeChatAttachmentError(w, http.StatusRequestEntityTooLarge, "too_large",
					"This file is too large. The limit is 10 MB.")
				return
			}
			writeChatAttachmentError(w, http.StatusBadRequest, "read_failed",
				"Failed to read the uploaded file.")
			return
		}
		break
	}

	if name == "" {
		writeChatAttachmentError(w, http.StatusBadRequest, "missing_file",
			"No file was uploaded.")
		return
	}
	if len(data) > maxChatAttachmentBytes {
		writeChatAttachmentError(w, http.StatusRequestEntityTooLarge, "too_large",
			"This file is too large. The limit is 10 MB.")
		return
	}
	if len(data) == 0 {
		writeChatAttachmentError(w, http.StatusBadRequest, "empty_file",
			"The uploaded file is empty.")
		return
	}

	ext := strings.ToLower(filepath.Ext(name))

	switch {
	case isPDFPayload(data):
		h.respondWithPDFText(w, name, data)
	case isChatAudioFile(ext, data):
		h.respondWithTranscript(w, r.Context(), name, ext, contentType, data)
	case isChatTextFile(ext):
		respondWithTextFile(w, name, ext, data)
	default:
		writeChatAttachmentError(w, http.StatusUnsupportedMediaType, "unsupported_type",
			fmt.Sprintf("%q is not a supported attachment. Attach an image, PDF, text/code file or audio file.", name))
	}
}

func (h *Handler) respondWithPDFText(w http.ResponseWriter, name string, data []byte) {
	text, truncated, err := extractPDFText(data)
	if errors.Is(err, errNoExtractableText) {
		writeChatAttachmentError(w, http.StatusUnprocessableEntity, "no_extractable_text",
			"This PDF has no extractable text. Scanned PDFs are not supported yet.")
		return
	}
	if err != nil {
		logger.WarnCF("chat", "Failed to extract PDF text", map[string]any{
			"filename": name,
			"error":    err.Error(),
		})
		writeChatAttachmentError(w, http.StatusUnprocessableEntity, "pdf_unreadable",
			"Could not read this PDF. It may be encrypted or damaged.")
		return
	}

	writeChatAttachmentJSON(w, http.StatusOK, chatAttachmentResponse{
		Filename:    name,
		ContentType: "application/pdf",
		Kind:        "pdf",
		Size:        int64(len(data)),
		Text:        text,
		Truncated:   truncated,
	})
}

func (h *Handler) respondWithTranscript(
	w http.ResponseWriter,
	ctx context.Context,
	name, ext, contentType string,
	data []byte,
) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeChatAttachmentError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to read the configuration.")
		return
	}

	transcriber := detectTranscriber(cfg)
	if transcriber == nil {
		writeChatAttachmentError(w, http.StatusUnprocessableEntity, "asr_not_configured",
			"Audio transcription is not configured. Add a speech model in Models, or attach a text/PDF file instead.")
		return
	}

	audioPath, cleanup, err := writeChatAttachmentTempFile(name, data)
	if err != nil {
		writeChatAttachmentError(w, http.StatusInternalServerError, "temp_write_failed",
			"Failed to prepare the audio file for transcription.")
		return
	}
	defer cleanup()

	transcribeCtx, cancel := context.WithTimeout(ctx, chatAttachmentTranscribeTimeout)
	defer cancel()

	result, err := transcriber.Transcribe(transcribeCtx, audioPath)
	if err != nil {
		logger.WarnCF("chat", "Audio transcription failed", map[string]any{
			"filename": name,
			"error":    err.Error(),
		})
		writeChatAttachmentError(w, http.StatusUnprocessableEntity, "transcription_failed",
			"Could not transcribe this audio file.")
		return
	}
	if result == nil || strings.TrimSpace(result.Text) == "" {
		writeChatAttachmentError(w, http.StatusUnprocessableEntity, "empty_transcript",
			"No speech was detected in this audio file.")
		return
	}

	text, truncated := trimChatAttachmentText(normalizeChatAttachmentText(result.Text))
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "audio/") {
		contentType = chatAudioExtensions[ext]
	}

	writeChatAttachmentJSON(w, http.StatusOK, chatAttachmentResponse{
		Filename:    name,
		ContentType: contentType,
		Kind:        "audio",
		Size:        int64(len(data)),
		Text:        text,
		Truncated:   truncated,
		Transcript:  true,
	})
}

func respondWithTextFile(w http.ResponseWriter, name, ext string, data []byte) {
	if len(data) > maxChatTextFileBytes {
		writeChatAttachmentError(w, http.StatusRequestEntityTooLarge, "too_large",
			fmt.Sprintf("Text files are limited to %d KB.", maxChatTextFileBytes>>10))
		return
	}
	if bytes.IndexByte(data, 0) >= 0 {
		writeChatAttachmentError(w, http.StatusUnsupportedMediaType, "not_text",
			"This file is not plain text.")
		return
	}

	decoded := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(decoded) {
		writeChatAttachmentError(w, http.StatusUnsupportedMediaType, "not_text",
			"This file is not valid UTF-8 text.")
		return
	}

	text, truncated := trimChatAttachmentText(normalizeChatAttachmentText(string(decoded)))
	if strings.TrimSpace(text) == "" {
		writeChatAttachmentError(w, http.StatusUnprocessableEntity, "empty_file",
			"This text file is empty.")
		return
	}

	writeChatAttachmentJSON(w, http.StatusOK, chatAttachmentResponse{
		Filename:    name,
		ContentType: chatTextExtensions[ext],
		Kind:        "text",
		Size:        int64(len(data)),
		Text:        text,
		Truncated:   truncated,
	})
}

// extractPDFText returns up to maxExtractedAttachmentText bytes of PDF text.
// The parser is third-party code, so panics on malformed input are converted
// into errors instead of taking down the launcher.
func extractPDFText(data []byte) (text string, truncated bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			text = ""
			truncated = false
			err = fmt.Errorf("pdf parser panic: %v", recovered)
		}
	}()

	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", false, err
	}

	plain, err := reader.GetPlainText()
	if err != nil {
		return "", false, err
	}
	if plain == nil {
		return "", false, errNoExtractableText
	}

	raw, err := io.ReadAll(io.LimitReader(plain, int64(maxExtractedAttachmentText)+1))
	if err != nil {
		return "", false, err
	}

	text = normalizeChatAttachmentText(string(raw))
	if len(raw) > maxExtractedAttachmentText {
		truncated = true
		text = truncateChatAttachmentText(text, maxExtractedAttachmentText)
	}
	if strings.TrimSpace(text) == "" {
		return "", false, errNoExtractableText
	}
	return text, truncated, nil
}

func isPDFPayload(data []byte) bool {
	return bytes.HasPrefix(data, []byte("%PDF-"))
}

func isChatTextFile(ext string) bool {
	_, ok := chatTextExtensions[ext]
	return ok
}

func isChatAudioFile(ext string, data []byte) bool {
	mimeType, ok := chatAudioExtensions[ext]
	if !ok || mimeType == "" {
		return false
	}

	kind, err := filetype.Match(data)
	if err != nil || kind == filetype.Unknown {
		// Extensions are still useful for formats without reliable magic bytes.
		return true
	}

	detected := strings.ToLower(kind.MIME.Value)
	if strings.HasPrefix(detected, "audio/") || detected == "application/ogg" {
		return true
	}
	// M4A files are commonly detected as video/mp4 containers.
	return ext == ".m4a" && strings.HasPrefix(detected, "video/")
}

func writeChatAttachmentTempFile(name string, data []byte) (path string, cleanup func(), err error) {
	dir := filepath.Join(media.TempDir(), "chat-attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", func() {}, err
	}

	ext := filepath.Ext(name)
	if ext == "" {
		ext = ".bin"
	}
	file, err := os.CreateTemp(dir, "upload-*"+ext)
	if err != nil {
		return "", func() {}, err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", func() {}, err
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", func() {}, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func sanitizeChatAttachmentFilename(raw string) string {
	name := strings.TrimSpace(raw)
	// Browsers may include a client path; keep only the base name.
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if utf8.RuneCountInString(name) > maxChatAttachmentNameLen {
		runes := []rune(name)
		name = string(runes[:maxChatAttachmentNameLen])
	}
	return name
}

func normalizeChatAttachmentText(raw string) string {
	text := strings.ToValidUTF8(raw, "")
	text = strings.ReplaceAll(text, "\x00", "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\f", "\n\n")
	// Keep the injected text from spoofing the attachment delimiters used by
	// the pico channel.
	text = strings.ReplaceAll(text, "----- END ATTACHMENT -----", "----- END ATTACHMENT ---")
	text = strings.ReplaceAll(text, "----- BEGIN ATTACHMENT -----", "----- BEGIN ATTACHMENT ---")
	return strings.TrimSpace(text)
}

func trimChatAttachmentText(text string) (string, bool) {
	if len(text) <= maxExtractedAttachmentText {
		return text, false
	}
	return truncateChatAttachmentText(text, maxExtractedAttachmentText), true
}

func truncateChatAttachmentText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return strings.TrimSpace(text[:cut])
}

func writeChatAttachmentJSON(w http.ResponseWriter, status int, payload chatAttachmentResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeChatAttachmentError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":  code,
		"error": message,
	})
}
