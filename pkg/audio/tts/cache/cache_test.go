package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachePutGetAndClear(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, time.Hour, 1<<20)
	key := Key("model", "voice", "1", "hello")
	if _, ok := c.Get(key); ok {
		t.Fatal("empty cache returned an entry")
	}
	if err := c.Put(key, []byte("audio"), Meta{ContentType: "audio/wav", Ext: ".wav"}); err != nil {
		t.Fatal(err)
	}
	entry, ok := c.Get(key)
	if !ok || string(entry.Data) != "audio" || entry.Meta.ContentType != "audio/wav" {
		t.Fatalf("entry = %#v ok=%v", entry, ok)
	}
	stats := c.Stats()
	if stats.Entries != 1 || stats.Bytes != 5 {
		t.Fatalf("stats = %#v", stats)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(key); ok {
		t.Fatal("cleared cache returned an entry")
	}
}

func TestCacheExpiresAndSweeps(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 40*time.Millisecond, 1<<20)
	key := Key("ttl")
	if err := c.Put(key, []byte("audio"), Meta{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := c.Get(key); ok {
		t.Fatal("expired entry was returned")
	}
	if stats := c.Stats(); stats.Entries != 0 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestCacheEvictsOldestWhenFull(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, time.Hour, 8)
	oldKey := Key("old")
	newKey := Key("new")
	if err := c.Put(oldKey, []byte("12345"), Meta{}); err != nil {
		t.Fatal(err)
	}
	oldPath, _ := c.paths(oldKey)
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(newKey, []byte("67890"), Meta{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(oldKey); ok {
		t.Fatal("oldest entry should have been evicted")
	}
	if _, ok := c.Get(newKey); !ok {
		t.Fatal("newest entry should remain")
	}
	if _, err := os.Stat(filepath.Join(dir, oldKey+".json")); !os.IsNotExist(err) {
		t.Fatalf("old metadata should be removed, err = %v", err)
	}
}

func TestKeyIsStableAndSeparated(t *testing.T) {
	a := Key("model", "voice", "1.0", "hello")
	b := Key("model", "voice", "1.0", "hello")
	c := Key("model", "voice", "1.5", "hello")
	if a != b {
		t.Fatal("same input produced different keys")
	}
	if a == c {
		t.Fatal("speed must change the key")
	}
}
