package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/audio/asr"
	"github.com/sipeed/picoclaw/pkg/config"
)

func setupChatAttachmentTestEnv(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("PICOCLAW_HOME", filepath.Join(tmp, ".picoclaw"))
	t.Setenv("TMP", filepath.Join(tmp, "tmp"))
	t.Setenv("TEMP", filepath.Join(tmp, "tmp"))
	t.Setenv("TMPDIR", filepath.Join(tmp, "tmp"))

	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, config.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	return configPath
}

func newChatAttachmentMux(t *testing.T, configPath string) *http.ServeMux {
	t.Helper()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.registerChatAttachmentRoutes(mux)
	return mux
}

func doChatAttachmentUpload(
	t *testing.T,
	mux *http.ServeMux,
	filename string,
	data []byte,
) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("part.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeChatAttachmentResponse(t *testing.T, rec *httptest.ResponseRecorder) chatAttachmentResponse {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload chatAttachmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return payload
}

func assertChatAttachmentError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, wantStatus, rec.Body.String())
	}
	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, body = %s", err, rec.Body.String())
	}
	if payload.Code != wantCode {
		t.Fatalf("error code = %q, want %q", payload.Code, wantCode)
	}
	if strings.TrimSpace(payload.Error) == "" {
		t.Fatal("error message is empty")
	}
}

// buildTestPDF returns a minimal single-page PDF with a proper xref table.
func buildTestPDF(pageText string) []byte {
	content := "BT /F1 24 Tf 72 720 Td (" + pageText + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects))
	for i, object := range objects {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objects)+1)
	buf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return buf.Bytes()
}

func TestChatAttachment_TextFile(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	rec := doChatAttachmentUpload(t, mux, "notes.md", []byte("# Title\n\nHello from a text file."))
	payload := decodeChatAttachmentResponse(t, rec)

	if payload.Kind != "text" {
		t.Fatalf("kind = %q, want text", payload.Kind)
	}
	if payload.Filename != "notes.md" {
		t.Fatalf("filename = %q, want notes.md", payload.Filename)
	}
	if !strings.Contains(payload.Text, "Hello from a text file.") {
		t.Fatalf("text = %q, want file content", payload.Text)
	}
	if payload.Truncated {
		t.Fatal("truncated = true, want false")
	}
}

func TestChatAttachment_TextFileStripsBOMAndNormalizesNewlines(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("line one\r\nline two\r\n")...)
	payload := decodeChatAttachmentResponse(t, doChatAttachmentUpload(t, mux, "notes.txt", data))

	if strings.Contains(payload.Text, "\ufeff") {
		t.Fatalf("text still contains BOM: %q", payload.Text)
	}
	if payload.Text != "line one\nline two" {
		t.Fatalf("text = %q, want normalized lines", payload.Text)
	}
}

func TestChatAttachment_TextFileTruncatesLargeContent(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	data := []byte(strings.Repeat("a", maxExtractedAttachmentText+2048))
	payload := decodeChatAttachmentResponse(t, doChatAttachmentUpload(t, mux, "big.log", data))

	if !payload.Truncated {
		t.Fatal("truncated = false, want true")
	}
	if len(payload.Text) > maxExtractedAttachmentText {
		t.Fatalf("len(text) = %d, want <= %d", len(payload.Text), maxExtractedAttachmentText)
	}
}

func TestChatAttachment_RejectsBinaryAndUnsupportedFiles(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	invalidUTF8 := append([]byte("hello"), 0xff, 0xfe, 0xfd)
	assertChatAttachmentError(t,
		doChatAttachmentUpload(t, mux, "notes.txt", invalidUTF8),
		http.StatusUnsupportedMediaType, "not_text")

	nulBytes := []byte("hello\x00world")
	assertChatAttachmentError(t,
		doChatAttachmentUpload(t, mux, "notes.txt", nulBytes),
		http.StatusUnsupportedMediaType, "not_text")

	assertChatAttachmentError(t,
		doChatAttachmentUpload(t, mux, "setup.exe", []byte("MZ\x90\x00binary")),
		http.StatusUnsupportedMediaType, "unsupported_type")
}

func TestChatAttachment_RejectsOversizedUpload(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	data := bytes.Repeat([]byte("a"), maxChatAttachmentBytes+2)
	rec := doChatAttachmentUpload(t, mux, "huge.txt", data)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestChatAttachment_SanitizesFilename(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	payload := decodeChatAttachmentResponse(t,
		doChatAttachmentUpload(t, mux, `..\..\evil\notes.txt`, []byte("content")))
	if payload.Filename != "notes.txt" {
		t.Fatalf("filename = %q, want notes.txt", payload.Filename)
	}
}

func TestChatAttachment_PDFText(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	payload := decodeChatAttachmentResponse(t,
		doChatAttachmentUpload(t, mux, "report.pdf", buildTestPDF("Hello PDF")))
	if payload.Kind != "pdf" {
		t.Fatalf("kind = %q, want pdf", payload.Kind)
	}
	if payload.ContentType != "application/pdf" {
		t.Fatalf("content_type = %q, want application/pdf", payload.ContentType)
	}
	if !strings.Contains(payload.Text, "Hello PDF") {
		t.Fatalf("text = %q, want PDF text", payload.Text)
	}
}

func TestChatAttachment_PDFWithoutText(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	assertChatAttachmentError(t,
		doChatAttachmentUpload(t, mux, "scan.pdf", buildTestPDF("")),
		http.StatusUnprocessableEntity, "no_extractable_text")
}

func TestChatAttachment_MalformedPDFDoesNotPanic(t *testing.T) {
	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	rec := doChatAttachmentUpload(t, mux, "broken.pdf", []byte("%PDF-1.4\nnot a real pdf"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestChatAttachment_AudioRequiresTranscriber(t *testing.T) {
	restore := detectTranscriber
	detectTranscriber = func(*config.Config) asr.Transcriber { return nil }
	t.Cleanup(func() { detectTranscriber = restore })

	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	assertChatAttachmentError(t,
		doChatAttachmentUpload(t, mux, "voice.mp3", []byte("ID3fake-audio")),
		http.StatusUnprocessableEntity, "asr_not_configured")
}

type fakeChatTranscriber struct {
	gotPath string
	gotData []byte
}

func (f *fakeChatTranscriber) Name() string { return "fake" }

func (f *fakeChatTranscriber) Transcribe(ctx context.Context, audioFilePath string) (*asr.TranscriptionResponse, error) {
	f.gotPath = audioFilePath
	data, err := os.ReadFile(audioFilePath)
	if err != nil {
		return nil, err
	}
	f.gotData = data
	return &asr.TranscriptionResponse{Text: "hello from audio\r\nsecond line"}, nil
}

func TestChatAttachment_AudioTranscript(t *testing.T) {
	transcriber := &fakeChatTranscriber{}
	restore := detectTranscriber
	detectTranscriber = func(*config.Config) asr.Transcriber { return transcriber }
	t.Cleanup(func() { detectTranscriber = restore })

	mux := newChatAttachmentMux(t, setupChatAttachmentTestEnv(t))

	rec := doChatAttachmentUpload(t, mux, "voice.mp3", []byte("ID3fake-audio"))
	payload := decodeChatAttachmentResponse(t, rec)

	if payload.Kind != "audio" || !payload.Transcript {
		t.Fatalf("payload = %#v, want audio transcript", payload)
	}
	if payload.Text != "hello from audio\nsecond line" {
		t.Fatalf("text = %q, want normalized transcript", payload.Text)
	}
	if transcriber.gotPath == "" {
		t.Fatal("transcriber did not receive a file path")
	}
	if string(transcriber.gotData) != "ID3fake-audio" {
		t.Fatalf("transcriber data = %q, want uploaded bytes", transcriber.gotData)
	}
	if _, err := os.Stat(transcriber.gotPath); !os.IsNotExist(err) {
		t.Fatalf("temp audio file %q was not cleaned up", transcriber.gotPath)
	}
}
