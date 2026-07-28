package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/storage"
)

func TestMetadataCacheUsesSizeAndModTime(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(sessionPath, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatal(err)
	}

	cache := &Store{
		entries: map[string]cacheEntry{},
		updated: map[string]cacheEntry{},
	}
	want := storage.CachedSession{Session: domain.Session{ID: "cached", MsgCount: 1, FilePath: sessionPath}, SearchText: "cached text"}
	cache.put(sessionPath, info, want)
	cache.entries = cache.updated
	cache.updated = map[string]cacheEntry{}

	got, ok := cache.get(sessionPath, info)
	if !ok {
		t.Fatal("cache should hit for unchanged file")
	}
	if got.Session.ID != want.Session.ID || got.SearchText != want.SearchText {
		t.Fatalf("cache session = %#v, want %#v", got, want)
	}

	if err := os.WriteFile(sessionPath, []byte("two changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	changedInfo, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.get(sessionPath, changedInfo); ok {
		t.Fatal("cache should miss after file size/mtime changes")
	}
}
