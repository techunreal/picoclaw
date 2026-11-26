package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadFileChecked(t *testing.T) {
	payload := []byte("picoclaw tts checksum test")
	digest := sha256.Sum256(payload)
	sum := hex.EncodeToString(digest[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dst := filepath.Join(t.TempDir(), "model.onnx")
	if err := downloadFileChecked(context.Background(), server.URL, dst, sum); err != nil {
		t.Fatalf("downloadFileChecked() error = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("content = %q", got)
	}

	bad := filepath.Join(t.TempDir(), "bad.onnx")
	err = downloadFileChecked(context.Background(), server.URL, bad, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(bad); !os.IsNotExist(statErr) {
		t.Fatalf("partial file should be removed, stat error = %v", statErr)
	}
}
