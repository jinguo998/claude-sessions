package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func testMenuSession() session.Session {
	return session.Session{
		ID:       "12345678-abcdef",
		FirstMsg: "Very long session title",
		Source:   session.SourceClaude,
	}
}

func TestNewContextMenuModelDefaultItems(t *testing.T) {
	c := NewContextMenuModel()

	if len(c.items) != 6 {
		t.Fatalf("len(items) = %d, want 6", len(c.items))
	}
}

func TestContextMenuOpenResetsState(t *testing.T) {
	c := NewContextMenuModel()
	sess := testMenuSession()

	c = c.Open(sess, 7, 9)
	c.cursor = 3
	c = c.Open(sess, 2, 4)

	if c.x != 2 || c.y != 4 {
		t.Fatalf("position = (%d,%d), want (2,4)", c.x, c.y)
	}
	if c.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", c.cursor)
	}
	if c.session.ID != sess.ID {
		t.Fatalf("session = %q, want %q", c.session.ID, sess.ID)
	}
}

func TestContextMenuKeyboard(t *testing.T) {
	c := NewContextMenuModel().Open(testMenuSession(), 0, 0)

	c, _ = c.Update(testKeyRunes("k"))
	if c.cursor != 0 {
		t.Fatalf("cursor after clamped up = %d, want 0", c.cursor)
	}

	c, _ = c.Update(testKeyRunes("j"))
	c, _ = c.Update(testKeyRunes("j"))
	if c.cursor != 2 {
		t.Fatalf("cursor after down = %d, want 2", c.cursor)
	}

	c.cursor = len(c.items) - 1
	c, _ = c.Update(testKeyRunes("j"))
	if c.cursor != len(c.items)-1 {
		t.Fatalf("cursor after clamped down = %d, want %d", c.cursor, len(c.items)-1)
	}

	c.cursor = 1
	_, cmd := c.Update(testKey(tea.KeyEnter))
	if got, ok := runCmd(t, cmd).(MenuActionMsg); !ok || got.Action != ActionResumeSafe {
		t.Fatalf("enter should emit safe resume action, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	_, cmd = c.Update(testKey(tea.KeyEsc))
	if _, ok := runCmd(t, cmd).(MenuCloseMsg); !ok {
		t.Fatalf("esc should emit MenuCloseMsg, got %T", runCmd(t, cmd))
	}
}

func TestContextMenuMouse(t *testing.T) {
	c := NewContextMenuModel().Open(testMenuSession(), 5, 2)

	_, cmd := c.Update(testMouse(6, 5, tea.MouseButtonLeft))
	if got, ok := runCmd(t, cmd).(MenuActionMsg); !ok || got.Action != ActionResumeFast {
		t.Fatalf("inside click should emit first MenuActionMsg, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	_, cmd = c.Update(testMouse(0, 0, tea.MouseButtonLeft))
	if _, ok := runCmd(t, cmd).(MenuCloseMsg); !ok {
		t.Fatalf("outside click should emit MenuCloseMsg, got %T", runCmd(t, cmd))
	}
}

func TestContextMenuViewContainsTitleAndItems(t *testing.T) {
	c := NewContextMenuModel().Open(testMenuSession(), 0, 0)
	view := stripANSI(c.View())

	if !strings.Contains(view, "Very long se...") {
		t.Fatalf("View() missing truncated title: %q", view)
	}
	for _, item := range menuItemsForSession(session.SourceRegistry{}, testMenuSession()) {
		if !strings.Contains(view, item.Label) {
			t.Fatalf("View() missing label %q in %q", item.Label, view)
		}
	}
}

func TestContextMenuViewHandlesShortIDFallback(t *testing.T) {
	c := NewContextMenuModel().Open(session.Session{ID: "abc", Source: session.SourceClaude}, 0, 0)
	view := stripANSI(c.View())

	if !strings.Contains(view, "abc") {
		t.Fatalf("View() missing short ID fallback: %q", view)
	}
}

func TestContextMenuOpenHidesFastResumeForCodex(t *testing.T) {
	sess := session.Session{
		ID:       "codex-1",
		FirstMsg: "Codex session",
		Source:   session.SourceCodex,
	}

	c := NewContextMenuModel().Open(sess, 0, 0)
	if len(c.items) != 5 {
		t.Fatalf("len(items) = %d, want 5", len(c.items))
	}

	view := stripANSI(c.View())
	if strings.Contains(view, "Resume (safe)") {
		t.Fatalf("Codex context menu should hide explicit safe resume, got %q", view)
	}
}

func TestContextMenuHidesArchiveForUnsupportedSource(t *testing.T) {
	sess := session.Session{
		ID:       "opencode-1",
		FirstMsg: "OpenCode session",
		Source:   session.SourceOpenCode,
	}

	c := NewContextMenuModel().Open(sess, 0, 0)
	view := stripANSI(c.View())
	if strings.Contains(view, "Archive") {
		t.Fatalf("OpenCode context menu should hide archive, got %q", view)
	}
	if len(c.items) != 4 {
		t.Fatalf("len(items) = %d, want 4", len(c.items))
	}
}

func TestContextMenuOverlayOnPadsAndClamps(t *testing.T) {
	baseLines := []string{
		strings.Repeat(".", 50),
		strings.Repeat(".", 50),
		strings.Repeat(".", 50),
		strings.Repeat(".", 50),
		strings.Repeat(".", 50),
		strings.Repeat(".", 50),
	}
	base := strings.Join(baseLines, "\n")

	c := NewContextMenuModel().Open(testMenuSession(), 2, 99)
	overlay := c.OverlayOn(base)
	lines := strings.Split(overlay, "\n")

	if len(lines) != len(baseLines) {
		t.Fatalf("line count = %d, want %d", len(lines), len(baseLines))
	}
	for i, line := range lines {
		if lipgloss.Width(line) != lipgloss.Width(baseLines[i]) {
			t.Fatalf("line %d width = %d, want %d", i, lipgloss.Width(line), lipgloss.Width(baseLines[i]))
		}
	}
	if !strings.HasPrefix(stripANSI(lines[0]), "  ") {
		t.Fatalf("overlay should honor x padding on clamped row, got %q", stripANSI(lines[0]))
	}
}
