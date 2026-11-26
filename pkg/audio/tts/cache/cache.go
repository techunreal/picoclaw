// Package cache stores synthesized TTS audio on disk with a TTL and an LRU
// size budget. Cache keys are SHA-256 digests, so the plain text is never
// written to disk.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Meta struct {
	ContentType string `json:"content_type"`
	Ext         string `json:"ext"`
}

type Entry struct {
	Data []byte
	Meta Meta
}

type Stats struct {
	Entries    int   `json:"entries"`
	Bytes      int64 `json:"bytes"`
	TTLMinutes int   `json:"ttl_minutes"`
	MaxBytes   int64 `json:"max_bytes"`
	Expired    int   `json:"expired,omitempty"`
}

type Cache struct {
	dir      string
	ttl      time.Duration
	maxBytes int64

	mu          sync.Mutex
	janitorOnce sync.Once
}

func New(dir string, ttl time.Duration, maxBytes int64) *Cache {
	return &Cache{dir: dir, ttl: ttl, maxBytes: maxBytes}
}

// StartJanitor removes expired entries on a timer, so the TTL is enforced even
// when nobody opens the TTS settings page.
func (c *Cache) StartJanitor(interval time.Duration) {
	if c == nil || interval <= 0 {
		return
	}
	c.janitorOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				_ = c.Sweep()
			}
		}()
	})
}

func Key(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) Get(key string) (Entry, bool) {
	if c == nil || key == "" {
		return Entry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	binPath, metaPath := c.paths(key)
	info, err := os.Stat(binPath)
	if err != nil {
		return Entry{}, false
	}
	if c.expired(info.ModTime()) {
		_ = os.Remove(binPath)
		_ = os.Remove(metaPath)
		return Entry{}, false
	}
	data, err := os.ReadFile(binPath)
	if err != nil {
		return Entry{}, false
	}
	entry := Entry{Data: data, Meta: Meta{ContentType: "audio/wav", Ext: ".wav"}}
	if metaData, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(metaData, &entry.Meta)
	}
	if entry.Meta.ContentType == "" {
		entry.Meta.ContentType = "audio/wav"
	}
	if entry.Meta.Ext == "" {
		entry.Meta.Ext = ".wav"
	}
	return entry, true
}

func (c *Cache) Put(key string, data []byte, meta Meta) error {
	if c == nil || key == "" || len(data) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	binPath, metaPath := c.paths(key)
	if meta.ContentType == "" {
		meta.ContentType = "audio/wav"
	}
	if meta.Ext == "" {
		meta.Ext = ".wav"
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := writeAtomic(binPath, data); err != nil {
		return err
	}
	if err := writeAtomic(metaPath, metaData); err != nil {
		return err
	}
	c.enforceLocked()
	return nil
}

func (c *Cache) Stats() Stats {
	if c == nil {
		return Stats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	stats := Stats{
		TTLMinutes: int(c.ttl / time.Minute),
		MaxBytes:   c.maxBytes,
	}
	_ = c.walkLocked(func(path string, info os.FileInfo) {
		if c.expired(info.ModTime()) {
			stats.Expired++
			return
		}
		stats.Entries++
		stats.Bytes += info.Size()
	})
	return stats
}

func (c *Cache) Sweep() Stats {
	if c == nil {
		return Stats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enforceLocked()
	stats := Stats{TTLMinutes: int(c.ttl / time.Minute), MaxBytes: c.maxBytes}
	_ = c.walkLocked(func(path string, info os.FileInfo) {
		if !c.expired(info.ModTime()) {
			stats.Entries++
			stats.Bytes += info.Size()
		}
	})
	return stats
}

func (c *Cache) Clear() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(c.dir, entry.Name()))
	}
	return nil
}

func (c *Cache) paths(key string) (string, string) {
	base := filepath.Join(c.dir, key)
	return base + ".bin", base + ".json"
}

func (c *Cache) expired(mod time.Time) bool {
	return c.ttl > 0 && time.Since(mod) > c.ttl
}

func (c *Cache) enforceLocked() {
	type item struct {
		path string
		info os.FileInfo
	}
	var items []item
	var total int64
	_ = c.walkLocked(func(path string, info os.FileInfo) {
		if c.expired(info.ModTime()) {
			_ = os.Remove(path)
			_ = os.Remove(strings.TrimSuffix(path, ".bin") + ".json")
			return
		}
		items = append(items, item{path: path, info: info})
		total += info.Size()
	})
	if c.maxBytes <= 0 || total <= c.maxBytes {
		return
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].info.ModTime().Before(items[j].info.ModTime())
	})
	for _, it := range items {
		if total <= c.maxBytes {
			break
		}
		if err := os.Remove(it.path); err == nil {
			_ = os.Remove(strings.TrimSuffix(it.path, ".bin") + ".json")
			total -= it.info.Size()
		}
	}
}

func (c *Cache) walkLocked(fn func(path string, info os.FileInfo)) error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bin") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fn(filepath.Join(c.dir, entry.Name()), info)
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cache write failed: %w", err)
	}
	return nil
}
