package api

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestPersistBrowserDomain_AddsOnce(t *testing.T) {
	configPath := setupChatAttachmentTestEnv(t)
	h := NewHandler(configPath)

	if err := h.persistBrowserDomain("https://News.Example/path"); err != nil {
		t.Fatalf("persistBrowserDomain() error = %v", err)
	}
	if err := h.persistBrowserDomain("news.example"); err != nil {
		t.Fatalf("persistBrowserDomain() second error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	count := 0
	for _, domain := range cfg.Tools.Browser.AllowedDomains {
		if domain == "news.example" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("domains = %v, want one news.example", cfg.Tools.Browser.AllowedDomains)
	}
}
