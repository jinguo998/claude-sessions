package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jinguo998/claude-sessions/internal/storage"
)

const metadataCacheVersion = 4

type metadataCacheFile struct {
	Version int                   `json:"version"`
	Entries map[string]cacheEntry `json:"entries"`
}

type cacheEntry struct {
	Size    int64                 `json:"size"`
	ModTime int64                 `json:"mod_time"`
	Session storage.CachedSession `json:"session"`
}

type Store struct {
	path    string
	entries map[string]cacheEntry
	updated map[string]cacheEntry
	mu      sync.Mutex
}

func New() *Store {
	path, err := metadataCachePath()
	if err != nil {
		return &Store{entries: map[string]cacheEntry{}, updated: map[string]cacheEntry{}}
	}
	return NewAt(path)
}

func NewAt(path string) *Store {
	c := &Store{
		path:    path,
		entries: map[string]cacheEntry{},
		updated: map[string]cacheEntry{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var file metadataCacheFile
	if json.Unmarshal(data, &file) != nil || file.Version != metadataCacheVersion {
		return c
	}
	c.entries = file.Entries
	if c.entries == nil {
		c.entries = map[string]cacheEntry{}
	}
	return c
}

func metadataCachePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "claude-sessions", "metadata-cache.json"), nil
}

func (c *Store) Get(path string, size int64, modTime time.Time) (storage.CachedSession, bool) {
	if c == nil {
		return storage.CachedSession{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[path]
	if !ok || entry.Size != size || entry.ModTime != modTime.UnixNano() {
		return storage.CachedSession{}, false
	}
	c.updated[path] = entry
	return entry.Session, true
}

func (c *Store) Put(path string, size int64, modTime time.Time, sess storage.CachedSession) {
	if c == nil {
		return
	}
	entry := cacheEntry{
		Size:    size,
		ModTime: modTime.UnixNano(),
		Session: sess,
	}
	c.mu.Lock()
	c.updated[path] = entry
	c.mu.Unlock()
}

func (c *Store) Save() {
	if c == nil || c.path == "" {
		return
	}
	c.mu.Lock()
	file := metadataCacheFile{
		Version: metadataCacheVersion,
		Entries: c.updated,
	}
	c.mu.Unlock()

	data, err := json.Marshal(file)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path)
}

func (c *Store) get(path string, info os.FileInfo) (storage.CachedSession, bool) {
	return c.Get(path, info.Size(), info.ModTime())
}

func (c *Store) put(path string, info os.FileInfo, sess storage.CachedSession) {
	c.Put(path, info.Size(), info.ModTime(), sess)
}

func (c *Store) save() {
	c.Save()
}
