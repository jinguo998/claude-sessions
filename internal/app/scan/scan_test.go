package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
	"github.com/jinguo998/claude-sessions/internal/storage"
)

type fakeScanner struct {
	source     domain.Source
	candidates []source.Candidate
	scanned    map[string]source.ScannedSession
	errs       map[string]error
	discover   error
}

func (f fakeScanner) Source() domain.Source {
	return f.source
}

func (f fakeScanner) Discover(ctx context.Context) ([]source.Candidate, error) {
	_ = ctx
	if f.discover != nil {
		return f.candidates, f.discover
	}
	return f.candidates, nil
}

func (f fakeScanner) ScanFile(ctx context.Context, candidate source.Candidate) (source.ScannedSession, error) {
	_ = ctx
	if err := f.errs[candidate.Path]; err != nil {
		return source.ScannedSession{}, err
	}
	return f.scanned[candidate.Path], nil
}

type fakeCache struct {
	items map[string]storage.CachedSession
	puts  map[string]storage.CachedSession
	saved bool
}

func (f *fakeCache) Get(path string, size int64, modTime time.Time) (storage.CachedSession, bool) {
	_ = size
	_ = modTime
	item, ok := f.items[path]
	return item, ok
}

func (f *fakeCache) Put(path string, size int64, modTime time.Time, sess storage.CachedSession) {
	_ = size
	_ = modTime
	f.puts[path] = sess
}

func (f *fakeCache) Save() {
	f.saved = true
}

func TestRepositoryScanUsesCacheAndRecordsPartialWarnings(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	candidates := []source.Candidate{
		{Source: domain.SourceClaude, Path: "/cached.jsonl", FileSize: 1, ModTime: now},
		{Source: domain.SourceClaude, Path: "/fresh.jsonl", FileSize: 2, ModTime: now},
		{Source: domain.SourceClaude, Path: "/broken.jsonl", FileSize: 3, ModTime: now},
	}
	cache := &fakeCache{
		items: map[string]storage.CachedSession{
			"/cached.jsonl": {
				Session:    domain.Session{ID: "cached", Source: domain.SourceClaude, MsgCount: 1},
				SearchText: "cached corpus",
			},
		},
		puts: map[string]storage.CachedSession{},
	}
	repo := NewRepository([]source.Scanner{fakeScanner{
		source:     domain.SourceClaude,
		candidates: candidates,
		scanned: map[string]source.ScannedSession{
			"/fresh.jsonl": {
				Session:     domain.Session{ID: "fresh", Source: domain.SourceClaude, MsgCount: 1},
				SearchParts: []string{" Fresh   Message ", "Tool Output"},
			},
		},
		errs: map[string]error{"/broken.jsonl": errors.New("bad json")},
	}}, cache)

	result := repo.Scan(context.Background())
	if result.Err != nil {
		t.Fatalf("Scan() fatal error = %v", result.Err)
	}
	if !cache.saved {
		t.Fatal("cache Save() was not called")
	}
	byID := map[string]string{}
	for _, sess := range result.Sessions {
		byID[sess.ID] = sess.SearchText
	}
	if byID["cached"] != "cached corpus" {
		t.Fatalf("cached session corpus = %q", byID["cached"])
	}
	if byID["fresh"] != "fresh message tool output" {
		t.Fatalf("fresh session corpus = %q", byID["fresh"])
	}
	if cache.puts["/fresh.jsonl"].SearchText != "fresh message tool output" {
		t.Fatalf("cached fresh corpus = %q", cache.puts["/fresh.jsonl"].SearchText)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Kind != WarningPartialScan {
		t.Fatalf("warnings = %#v, want one partial scan warning", result.Warnings)
	}
}

func TestRepositoryScanInvalidatesCacheWhenSourceMetadataChanges(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	cache := &fakeCache{
		items: map[string]storage.CachedSession{
			"/codex.jsonl": {
				Session:     domain.Session{ID: "old", Source: domain.SourceCodex, MsgCount: 1, Title: "Old title"},
				SearchText:  "old title",
				MetadataKey: "Old title",
			},
		},
		puts: map[string]storage.CachedSession{},
	}
	repo := NewRepository([]source.Scanner{fakeScanner{
		source: domain.SourceCodex,
		candidates: []source.Candidate{{
			Source:      domain.SourceCodex,
			Path:        "/codex.jsonl",
			FileSize:    1,
			ModTime:     now,
			MetadataKey: "New title",
		}},
		scanned: map[string]source.ScannedSession{
			"/codex.jsonl": {
				Session:     domain.Session{ID: "new", Source: domain.SourceCodex, MsgCount: 1, Title: "New title"},
				SearchParts: []string{"New title"},
			},
		},
	}}, cache)

	result := repo.Scan(context.Background())
	if len(result.Sessions) != 1 || result.Sessions[0].Title != "New title" {
		t.Fatalf("sessions = %#v, want refreshed new title", result.Sessions)
	}
	if cache.puts["/codex.jsonl"].MetadataKey != "New title" {
		t.Fatalf("cached metadata key = %q", cache.puts["/codex.jsonl"].MetadataKey)
	}
}

func TestRepositoryScanContinuesWithDiscoverWarningAndCandidates(t *testing.T) {
	repo := NewRepository([]source.Scanner{fakeScanner{
		source:   domain.SourceClaude,
		discover: errors.New("one project unreadable"),
		candidates: []source.Candidate{{
			Source:   domain.SourceClaude,
			Path:     "/ok.jsonl",
			FileSize: 1,
			ModTime:  time.Now(),
		}},
		scanned: map[string]source.ScannedSession{
			"/ok.jsonl": {
				Session:     domain.Session{ID: "ok", Source: domain.SourceClaude, MsgCount: 1},
				SearchParts: []string{"ok"},
			},
		},
	}}, nil)

	result := repo.Scan(context.Background())
	if len(result.Sessions) != 1 || result.Sessions[0].ID != "ok" {
		t.Fatalf("sessions = %#v, want partial result", result.Sessions)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Kind != WarningPartialScan {
		t.Fatalf("warnings = %#v, want one partial warning", result.Warnings)
	}
}

func TestRepositoryScanReportsSourceUnavailable(t *testing.T) {
	repo := NewRepository([]source.Scanner{fakeScanner{
		source:   domain.SourceCodex,
		discover: errors.New("permission denied"),
	}}, nil)

	result := repo.Scan(context.Background())
	if len(result.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none", result.Sessions)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Kind != WarningSourceUnavailable {
		t.Fatalf("warnings = %#v, want source unavailable", result.Warnings)
	}
}
