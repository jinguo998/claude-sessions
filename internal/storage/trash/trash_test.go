package trash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/storage"
)

func TestArchiveSessionAt(t *testing.T) {
	home := t.TempDir()
	workdir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	filePath := filepath.Join(workdir, "session.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sideDir := filepath.Join(workdir, "session")
	if err := os.Mkdir(sideDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	sess := domain.Session{
		ID:          "12345678-1234-1234-1234-123456789012",
		Source:      domain.SourceClaude,
		FilePath:    filePath,
		ProjectPath: "/Users/example/demo",
		Title:       "Thread title",
	}

	now := time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)
	archiveDir, err := archiveSessionAt(sess, home, sideDir, now)
	if err != nil {
		t.Fatalf("archiveSessionAt() error = %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("original file should be moved, stat err = %v", err)
	}
	if _, err := os.Stat(sideDir); !os.IsNotExist(err) {
		t.Fatalf("original side dir should be moved, stat err = %v", err)
	}
	for _, path := range []string{
		filepath.Join(archiveDir, "session.jsonl"),
		filepath.Join(archiveDir, "session"),
		filepath.Join(archiveDir, "metadata.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected archived path %q, stat err = %v", path, err)
		}
	}

	metaBytes, err := os.ReadFile(filepath.Join(archiveDir, "metadata.json"))
	if err != nil {
		t.Fatalf("ReadFile(metadata) error = %v", err)
	}
	var meta storage.ArchivedSessionMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if meta.ID != sess.ID || meta.Source != sess.Source || meta.OriginalFilePath != filePath {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
}

func TestLoadAndRestoreArchivedSessions(t *testing.T) {
	home := t.TempDir()
	workdir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	filePath := filepath.Join(workdir, "session.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sideDir := filepath.Join(workdir, "session")
	if err := os.Mkdir(sideDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	sess := domain.Session{
		ID:          "12345678-1234-1234-1234-123456789012",
		Source:      domain.SourceCodex,
		FilePath:    filePath,
		ProjectPath: "/Users/example/demo",
		Title:       "Restore target",
	}

	_, err := archiveSessionAt(sess, home, sideDir, time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("archiveSessionAt() error = %v", err)
	}

	items, err := loadArchivedSessionsFromHome(home)
	if err != nil {
		t.Fatalf("loadArchivedSessionsFromHome() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Metadata.Title != "Restore target" {
		t.Fatalf("unexpected loaded item: %#v", items[0])
	}

	if err := restoreArchivedSession(items[0]); err != nil {
		t.Fatalf("restoreArchivedSession() error = %v", err)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("restored file missing, stat err = %v", err)
	}
	if _, err := os.Stat(sideDir); err != nil {
		t.Fatalf("restored side dir missing, stat err = %v", err)
	}

	items, err = loadArchivedSessionsFromHome(home)
	if err != nil {
		t.Fatalf("loadArchivedSessionsFromHome() after restore error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items after restore = %d, want 0", len(items))
	}
}

func TestRestoreArchivedSessionDestinationConflict(t *testing.T) {
	home := t.TempDir()
	workdir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	filePath := filepath.Join(workdir, "session.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sess := domain.Session{
		ID:       "conflict-1",
		Source:   domain.SourceClaude,
		FilePath: filePath,
	}
	if _, err := archiveSessionAt(sess, home, "", time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)); err != nil {
		t.Fatalf("archiveSessionAt() error = %v", err)
	}
	items, err := loadArchivedSessionsFromHome(home)
	if err != nil {
		t.Fatalf("loadArchivedSessionsFromHome() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	if err := os.WriteFile(filePath, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict) error = %v", err)
	}
	if err := restoreArchivedSession(items[0]); err == nil {
		t.Fatal("restoreArchivedSession() should fail when destination exists")
	}
	if _, err := os.Stat(items[0].SessionFile); err != nil {
		t.Fatalf("archive file should remain after conflict, stat err = %v", err)
	}
}

func TestDeleteArchivedSession(t *testing.T) {
	home := t.TempDir()
	workdir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	filePath := filepath.Join(workdir, "session.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sess := domain.Session{
		ID:       "delete-archive-1",
		Source:   domain.SourceClaude,
		FilePath: filePath,
	}
	archiveDir, err := archiveSessionAt(sess, home, "", time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("archiveSessionAt() error = %v", err)
	}

	items, err := loadArchivedSessionsFromHome(home)
	if err != nil {
		t.Fatalf("loadArchivedSessionsFromHome() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if err := deleteArchivedSession(items[0]); err != nil {
		t.Fatalf("deleteArchivedSession() error = %v", err)
	}
	if _, err := os.Stat(archiveDir); !os.IsNotExist(err) {
		t.Fatalf("archive dir should be deleted, stat err = %v", err)
	}
}
