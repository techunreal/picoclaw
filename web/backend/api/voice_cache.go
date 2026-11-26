package api

import (
	"net/http"
	"path/filepath"
	"sync"
	"time"

	ttscache "github.com/sipeed/picoclaw/pkg/audio/tts/cache"
	"github.com/sipeed/picoclaw/pkg/config"
)

const (
	voiceCacheTTL      = time.Hour
	voiceCacheMaxBytes = int64(200 << 20)
)

var (
	voiceCacheMu   sync.Mutex
	voiceCacheInst *ttscache.Cache
	voiceCacheRoot string
)

func getVoiceCache() *ttscache.Cache {
	root := filepath.Join(config.GetHome(), "tts", "cache")
	voiceCacheMu.Lock()
	defer voiceCacheMu.Unlock()
	if voiceCacheInst == nil || voiceCacheRoot != root {
		voiceCacheInst = ttscache.New(root, voiceCacheTTL, voiceCacheMaxBytes)
		voiceCacheRoot = root
		voiceCacheInst.StartJanitor(10 * time.Minute)
	}
	return voiceCacheInst
}

func (h *Handler) handleVoiceCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := getVoiceCache().Sweep()
	writeBrowserSettingsJSON(w, http.StatusOK, stats)
}

func (h *Handler) handleVoiceCacheClear(w http.ResponseWriter, r *http.Request) {
	cache := getVoiceCache()
	if err := cache.Clear(); err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "cache_clear_failed", err.Error())
		return
	}
	writeBrowserSettingsJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
