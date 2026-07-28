package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
	"github.com/jinguo998/claude-sessions/internal/storage"
)

func testArchivedItem() storage.ArchivedSession {
	return storage.ArchivedSession{
		ArchiveDir: "/tmp/archive-1",
		Metadata: storage.ArchivedSessionMetadata{
			ID:               "12345678-1234",
			Source:           session.SourceClaude,
			ArchivedAt:       "2026-04-14T01:02:03Z",
			OriginalFilePath: "/Users/example/demo/session.jsonl",
			ProjectPath:      "/Users/example/demo",
			Title:            "Restore me",
		},
		SessionFile: filepath.Join("/tmp/archive-1", "session.jsonl"),
	}
}

func TestTrashModelNavigationAndRestore(t *testing.T) {
	m := NewTrashModel().SetSize(100, 30).SetItems([]storage.ArchivedSession{
		testArchivedItem(),
		{
			ArchiveDir: "/tmp/archive-2",
			Metadata: storage.ArchivedSessionMetadata{
				ID:               "22345678-1234",
				Source:           session.SourceCodex,
				ArchivedAt:       "2026-04-13T01:02:03Z",
				OriginalFilePath: "/Users/example/demo/codex.jsonl",
			},
			SessionFile: filepath.Join("/tmp/archive-2", "codex.jsonl"),
		},
	}, nil)

	m, _ = m.Update(testKeyRunes("j"))
	if m.cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", m.cursor)
	}
	m, cmd := m.Update(testKeyRunes("r"))
	if got, ok := runCmd(t, cmd).(TrashRestoreMsg); !ok || got.Item.Metadata.ID != "22345678-1234" {
		t.Fatalf("restore cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	m, cmd = m.Update(testKeyRunes("p"))
	if got, ok := runCmd(t, cmd).(TrashPreviewMsg); !ok || got.Item.Metadata.ID != "22345678-1234" {
		t.Fatalf("preview cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	m, cmd = m.Update(testKeyRunes("x"))
	if got, ok := runCmd(t, cmd).(TrashDeleteMsg); !ok || got.Item.Metadata.ID != "22345678-1234" {
		t.Fatalf("delete cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	m, cmd = m.Update(testKeyRunes("D"))
	if _, ok := runCmd(t, cmd).(TrashEmptyMsg); !ok {
		t.Fatalf("empty cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}
}

func TestTrashModelView(t *testing.T) {
	m := NewTrashModel().SetSize(120, 30).SetItems([]storage.ArchivedSession{testArchivedItem()}, nil)
	got := stripANSI(m.View())
	for _, needle := range []string{"Trash", "Restore me", "Restore to:", "Enter/r", "Preview", "Delete", "Empty", "04-14 09:02"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("trash view missing %q in %q", needle, got)
		}
	}
}

func TestTrashModelMouseSelectionAndPreview(t *testing.T) {
	m := NewTrashModel().SetSize(100, 30).SetItems([]storage.ArchivedSession{
		testArchivedItem(),
		{
			ArchiveDir: "/tmp/archive-2",
			Metadata: storage.ArchivedSessionMetadata{
				ID:               "22345678-1234",
				Source:           session.SourceCodex,
				ArchivedAt:       "2026-04-13T01:02:03Z",
				OriginalFilePath: "/Users/example/demo/codex.jsonl",
			},
			SessionFile: filepath.Join("/tmp/archive-2", "codex.jsonl"),
		},
	}, nil)

	rowY := trashRowStart + 1
	m, _ = m.Update(testMouse(4, rowY, tea.MouseButtonLeft))
	if m.cursor != 1 {
		t.Fatalf("cursor after click = %d, want 1", m.cursor)
	}

	m, cmd := m.Update(testMouse(4, rowY, tea.MouseButtonLeft))
	if got, ok := runCmd(t, cmd).(TrashPreviewMsg); !ok || got.Item.Metadata.ID != "22345678-1234" {
		t.Fatalf("double-click preview cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}
}
