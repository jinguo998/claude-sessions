package tui

import (
	"strings"
	"testing"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func TestSidePreviewLoadSessionReplacesStaleLinesWithLoading(t *testing.T) {
	m := NewSidePreviewModel()
	m.sessionID = "old"
	m.content = "old content"
	m.lines = []string{"old content"}

	m, cmd := m.LoadSession(nil, session.Session{
		ID:          "new",
		FirstMsg:    "New session",
		ProjectPath: "/tmp/project",
		FilePath:    "/tmp/missing.jsonl",
		Source:      session.SourceClaude,
	}, 80, 7)

	if cmd == nil {
		t.Fatal("LoadSession should return a load command")
	}
	if m.sessionID != "new" {
		t.Fatalf("sessionID = %q, want new", m.sessionID)
	}
	if m.requestToken != 7 {
		t.Fatalf("requestToken = %d, want 7", m.requestToken)
	}
	if strings.Contains(strings.Join(m.lines, "\n"), "old content") {
		t.Fatalf("loading lines still contain stale content: %#v", m.lines)
	}
	if !strings.Contains(stripANSI(m.View(80)), "Loading") {
		t.Fatalf("loading view = %q, want Loading", stripANSI(m.View(80)))
	}
}

func TestSidePreviewNeedsReloadDoesNotDuplicateSameSession(t *testing.T) {
	m := NewSidePreviewModel()
	m.sessionID = "same"
	m.loading = true
	if m.NeedsReload("same") {
		t.Fatal("loading preview should not duplicate a load for the same session")
	}

	m.loading = false
	if m.NeedsReload("same") {
		t.Fatal("loaded preview should not reload the same session")
	}
	if !m.NeedsReload("other") {
		t.Fatal("different session should reload")
	}
}

func TestSidePreviewIgnoresStaleLoadToken(t *testing.T) {
	m := NewSidePreviewModel()
	m.sessionID = "same"
	m.requestToken = 2
	m.loading = true
	m.content = "Loading..."
	m.lines = []string{"Loading..."}

	m, _ = m.Update(SidePreviewLoadedMsg{
		Token:     1,
		SessionID: "same",
		Content:   "stale content",
	})
	if strings.Contains(strings.Join(m.lines, "\n"), "stale content") {
		t.Fatalf("stale token updated lines: %#v", m.lines)
	}
	if !m.loading {
		t.Fatal("stale token should leave preview loading")
	}

	m, _ = m.Update(SidePreviewLoadedMsg{
		Token:     2,
		SessionID: "same",
		Content:   "fresh content",
	})
	if !strings.Contains(strings.Join(m.lines, "\n"), "fresh content") {
		t.Fatalf("fresh token did not update lines: %#v", m.lines)
	}
	if m.loading {
		t.Fatal("fresh token should clear loading")
	}
}

func TestSidePreviewScrollClampsToContent(t *testing.T) {
	m := NewSidePreviewModel().SetSize(10)
	m.lines = []string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
		"line 9",
		"line 10",
	}

	m = m.ScrollDown(100)
	if m.scroll != 6 {
		t.Fatalf("scroll after overscroll = %d, want 6", m.scroll)
	}

	m = m.ScrollUp(1)
	if m.scroll != 5 {
		t.Fatalf("scroll after wheel up from bottom = %d, want 5", m.scroll)
	}

	m = m.SetSize(20)
	if m.scroll != 0 {
		t.Fatalf("scroll after resize beyond content = %d, want 0", m.scroll)
	}
}

func TestSidePreviewFullLoadPreservesTailAnchor(t *testing.T) {
	m := NewSidePreviewModel().SetSize(7)
	m.sessionID = "same"
	m.requestToken = 3
	m.lines = []string{"tail 1", "tail 2", "tail 3"}
	m.content = strings.Join(m.lines, "\n")
	full := strings.Join([]string{
		"old 1",
		"old 2",
		"old 3",
		"old 4",
		"old 5",
		"old 6",
		"old 7",
		"tail 1",
		"tail 2",
		"tail 3",
	}, "\n")

	m, _ = m.Update(SidePreviewLoadedMsg{
		Token:             3,
		SessionID:         "same",
		Content:           full,
		Complete:          true,
		PreserveTailLines: 3,
	})

	if !m.complete {
		t.Fatal("full load should mark side preview complete")
	}
	if m.scroll != 7 {
		t.Fatalf("scroll after full load = %d, want 7", m.scroll)
	}
}

func TestSidePreviewStatusText(t *testing.T) {
	tests := []struct {
		name string
		m    SidePreviewModel
		want string
	}{
		{name: "loading", m: SidePreviewModel{loading: true}, want: "Loading preview"},
		{name: "loading more", m: SidePreviewModel{loadingMore: true}, want: "Loading full history"},
		{name: "complete", m: SidePreviewModel{complete: true}, want: "Full history"},
		{name: "tail", m: SidePreviewModel{}, want: "Tail preview"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.statusText(); !strings.Contains(got, tt.want) {
				t.Fatalf("statusText() = %q, want %q", got, tt.want)
			}
		})
	}
}
