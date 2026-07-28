package tui

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func testKeyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func testKey(keyType tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: keyType}
}

func testMouse(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: button,
		Action: tea.MouseActionPress,
	}
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

func testSampleFilePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "sample.jsonl")
}

func testSessions() []session.Session {
	base := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	return []session.Session{
		{
			ID:          "alpha",
			ProjectDir:  "-Users-example-alpha",
			ProjectPath: "/Users/example/alpha",
			Source:      session.SourceClaude,
			FirstMsg:    "Fix alpha bug",
			LastMsg:     "done",
			SearchText:  "fix alpha search token",
			LastTime:    base.Add(-1 * time.Hour),
			MsgCount:    5,
			ToolCount:   1,
			TokenUsage:  session.TokenUsage{Input: 100, Output: 50},
		},
		{
			ID:          "bravo",
			ProjectDir:  "-Users-example-bravo",
			ProjectPath: "/Users/example/bravo",
			Source:      session.SourceCodex,
			FirstMsg:    "Refactor bravo",
			LastMsg:     "investigating",
			SearchText:  "refactor bravo something",
			LastTime:    base.Add(-3 * time.Hour),
			MsgCount:    12,
			ToolCount:   8,
			TokenUsage:  session.TokenUsage{Input: 2000, Output: 500},
		},
		{
			ID:          "charlie",
			ProjectDir:  "-Users-example-alpha",
			ProjectPath: "/Users/example/alpha",
			Source:      session.SourceClaude,
			FirstMsg:    "Search logs",
			LastMsg:     "finished",
			SearchText:  "look for gamma match",
			LastTime:    base.Add(-2 * time.Hour),
			MsgCount:    8,
			ToolCount:   3,
			TokenUsage:  session.TokenUsage{Input: 600, Output: 200},
		},
	}
}

func TestNewListModelInitialState(t *testing.T) {
	l := NewListModel()

	if l.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", l.cursor)
	}
	if l.searching {
		t.Fatal("searching should start false")
	}
	if l.sortMode != sortRecent {
		t.Fatalf("sortMode = %v, want %v", l.sortMode, sortRecent)
	}
	if l.filterProj != "" {
		t.Fatalf("filterProj = %q, want empty", l.filterProj)
	}
	if l.loaded {
		t.Fatal("loaded should start false")
	}
}

func TestListModelSetSessionsAppliesDefaultSort(t *testing.T) {
	l := NewListModel().SetSessions(testSessions())

	if !l.loaded {
		t.Fatal("SetSessions should mark model loaded")
	}

	got := []string{l.filtered[0].ID, l.filtered[1].ID, l.filtered[2].ID}
	want := []string{"alpha", "charlie", "bravo"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filtered order = %v, want %v", got, want)
		}
	}
}

func TestListModelSetSessionsPreservesActiveFilters(t *testing.T) {
	l := NewListModel()
	l.filterProj = "/Users/example/alpha"
	l.sourceFilter = sourceFilter(1)
	l.searchQuery = "gamma"

	l = l.SetSessions(testSessions())

	if len(l.filtered) != 1 || l.filtered[0].ID != "charlie" {
		t.Fatalf("filtered sessions after refresh = %#v, want only charlie", l.filtered)
	}
}

func TestListModelApplySort(t *testing.T) {
	tests := []struct {
		name string
		mode sortMode
		want []string
	}{
		{name: "recent", mode: sortRecent, want: []string{"alpha", "charlie", "bravo"}},
		{name: "project", mode: sortProject, want: []string{"alpha", "charlie", "bravo"}},
		{name: "message count", mode: sortMsgCount, want: []string{"bravo", "charlie", "alpha"}},
		{name: "tool count", mode: sortToolCount, want: []string{"bravo", "charlie", "alpha"}},
		{name: "tokens", mode: sortTokens, want: []string{"bravo", "charlie", "alpha"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewListModel().SetSessions(testSessions())
			l.sortMode = tt.mode
			l.applySort()

			got := []string{l.filtered[0].ID, l.filtered[1].ID, l.filtered[2].ID}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("order = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestListModelProjectSortUsesProjectPath(t *testing.T) {
	base := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	l := NewListModel().SetSessions([]session.Session{
		{ID: "codex-alpha", ProjectDir: "alpha", ProjectPath: "/repo/alpha", LastTime: base.Add(-2 * time.Hour)},
		{ID: "claude-alpha", ProjectDir: "-repo-alpha", ProjectPath: "/repo/alpha", LastTime: base.Add(-1 * time.Hour)},
		{ID: "bravo", ProjectDir: "-repo-bravo", ProjectPath: "/repo/bravo", LastTime: base},
	})
	l.sortMode = sortProject
	l.applySort()

	got := []string{l.filtered[0].ID, l.filtered[1].ID, l.filtered[2].ID}
	want := []string{"claude-alpha", "codex-alpha", "bravo"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("project sort order = %v, want %v", got, want)
		}
	}
}

func TestListModelApplyFilter(t *testing.T) {
	l := NewListModel().SetSessions(testSessions())

	l.searchQuery = "gamma"
	l.applyFilter()
	if len(l.filtered) != 1 || l.filtered[0].ID != "charlie" {
		t.Fatalf("search filter result = %#v, want only charlie", l.filtered)
	}

	l.searchQuery = "bravo"
	l.applyFilter()
	if len(l.filtered) != 1 || l.filtered[0].ID != "bravo" {
		t.Fatalf("project short name filter result = %#v, want only bravo", l.filtered)
	}

	l.searchQuery = ""
	l.filterProj = "/Users/example/alpha"
	l.applyFilter()
	if len(l.filtered) != 2 {
		t.Fatalf("project filter len = %d, want 2", len(l.filtered))
	}
	for _, s := range l.filtered {
		if s.ProjectPath != "/Users/example/alpha" {
			t.Fatalf("project filter included %q", s.ProjectPath)
		}
	}
}

func TestListModelKeyboardNavigationAndActions(t *testing.T) {
	l := NewListModel().SetSessions(testSessions()).SetSize(120, 20)

	l, cmd := l.Update(testKeyRunes("j"))
	if l.cursor != 1 {
		t.Fatalf("cursor after j = %d, want 1", l.cursor)
	}
	if got, ok := runCmd(t, cmd).(FilterChangedMsg); !ok || !got.Debounce {
		t.Fatalf("j should emit debounced FilterChangedMsg, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	l, cmd = l.Update(testKeyRunes("k"))
	if l.cursor != 0 {
		t.Fatalf("cursor after k = %d, want 0", l.cursor)
	}
	if got, ok := runCmd(t, cmd).(FilterChangedMsg); !ok || !got.Debounce {
		t.Fatalf("k should emit debounced FilterChangedMsg, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	actions := []struct {
		name string
		key  tea.KeyMsg
		want any
	}{
		{name: "enter", key: testKey(tea.KeyEnter), want: SessionPreviewMsg{}},
		{name: "resume", key: testKeyRunes("r"), want: SessionSelectedMsg{}},
		{name: "safe resume", key: testKeyRunes("R"), want: SessionSelectedMsg{}},
		{name: "fork", key: testKeyRunes("n"), want: SessionForkMsg{}},
		{name: "delete", key: testKeyRunes("d"), want: SessionDeleteMsg{}},
		{name: "menu", key: testKeyRunes("m"), want: OpenContextMenuMsg{}},
		{name: "stats", key: testKeyRunes("t"), want: OpenStatsMsg{}},
		{name: "trash", key: testKeyRunes("T"), want: OpenTrashMsg{}},
		{name: "quit", key: testKeyRunes("q"), want: tea.QuitMsg{}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			updated, cmd := l.Update(tt.key)
			_ = updated
			msg := runCmd(t, cmd)
			switch tt.want.(type) {
			case SessionSelectedMsg:
				if got, ok := msg.(SessionSelectedMsg); !ok || got.Session.ID != "alpha" {
					t.Fatalf("got %T %#v, want SessionSelectedMsg for alpha", msg, msg)
				} else if tt.name == "resume" && got.PermissionMode != session.PermissionModeFast {
					t.Fatalf("default resume should use fast mode, got %#v", got)
				} else if tt.name == "safe resume" && got.PermissionMode != session.PermissionModeSafe {
					t.Fatalf("safe resume should use safe mode, got %#v", got)
				}
			case SessionForkMsg:
				if got, ok := msg.(SessionForkMsg); !ok || got.Session.ID != "alpha" {
					t.Fatalf("got %T %#v, want SessionForkMsg for alpha", msg, msg)
				} else if got.PermissionMode != session.PermissionModeFast {
					t.Fatalf("fork should use default fast mode for Claude, got %#v", got)
				}
			case SessionPreviewMsg:
				if got, ok := msg.(SessionPreviewMsg); !ok || got.Session.ID != "alpha" {
					t.Fatalf("got %T %#v, want SessionPreviewMsg for alpha", msg, msg)
				}
			case SessionDeleteMsg:
				if got, ok := msg.(SessionDeleteMsg); !ok || got.Index != 0 {
					t.Fatalf("got %T %#v, want SessionDeleteMsg index 0", msg, msg)
				}
			case OpenContextMenuMsg:
				if got, ok := msg.(OpenContextMenuMsg); !ok || got.Session.ID != "alpha" {
					t.Fatalf("got %T %#v, want OpenContextMenuMsg for alpha", msg, msg)
				}
			case OpenStatsMsg:
				if _, ok := msg.(OpenStatsMsg); !ok {
					t.Fatalf("got %T, want OpenStatsMsg", msg)
				}
			case OpenTrashMsg:
				if _, ok := msg.(OpenTrashMsg); !ok {
					t.Fatalf("got %T, want OpenTrashMsg", msg)
				}
			case tea.QuitMsg:
				if _, ok := msg.(tea.QuitMsg); !ok {
					t.Fatalf("got %T, want tea.QuitMsg", msg)
				}
			}
		})
	}

	l, cmd = l.Update(testKeyRunes("/"))
	if !l.searching {
		t.Fatal("/ should enable search mode")
	}
	if cmd == nil {
		t.Fatal("/ should return blink cmd")
	}

	prevSort := l.sortMode
	l.searching = false
	l, _ = l.Update(testKeyRunes("s"))
	if l.sortMode == prevSort {
		t.Fatalf("s should cycle sort mode from %v", prevSort)
	}

	l, cmd = l.Update(testKey(tea.KeyCtrlR))
	if _, ok := runCmd(t, cmd).(RefreshMsg); !ok {
		t.Fatalf("ctrl+r should emit RefreshMsg, got %T", runCmd(t, cmd))
	}

	l.filterProj = ""
	l, cmd = l.Update(testKeyRunes("f"))
	if _, ok := runCmd(t, cmd).(OpenProjectPickerMsg); !ok {
		t.Fatalf("f should emit OpenProjectPickerMsg, got %T", runCmd(t, cmd))
	}
}

func TestListModelDefaultResumeFallsBackToSafeForCodex(t *testing.T) {
	l := NewListModel().SetSessions(testSessions()).SetSize(120, 20)
	l.cursor = 2 // bravo, Codex after recent sort

	_, cmd := l.Update(testKeyRunes("r"))
	if got, ok := runCmd(t, cmd).(SessionSelectedMsg); !ok || got.PermissionMode != session.PermissionModeSafe {
		t.Fatalf("default resume on Codex should stay safe, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	if view := stripANSI(l.View()); strings.Contains(view, "Safe") {
		t.Fatalf("Codex list help should hide explicit safe resume, got %q", view)
	}
}

func TestListModelDefaultResumeUsesSafeForOpenCode(t *testing.T) {
	l := NewListModel().SetSessions([]session.Session{
		{
			ID:          "opencode",
			Source:      session.SourceOpenCode,
			ProjectPath: "/repo/opencode",
			FirstMsg:    "OpenCode work",
			SearchText:  "opencode work",
			LastTime:    time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC),
			MsgCount:    1,
		},
	}).SetSize(120, 20)

	_, cmd := l.Update(testKeyRunes("r"))
	if got, ok := runCmd(t, cmd).(SessionSelectedMsg); !ok || got.PermissionMode != session.PermissionModeSafe {
		t.Fatalf("default resume on OpenCode should use safe mode, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	_, cmd = l.Update(testKeyRunes("n"))
	if got, ok := runCmd(t, cmd).(SessionForkMsg); !ok || got.PermissionMode != session.PermissionModeSafe {
		t.Fatalf("fork on OpenCode should use safe mode, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}
}

func TestListModelArchiveUnsupportedForOpenCode(t *testing.T) {
	l := NewListModel().SetSessions([]session.Session{
		{
			ID:          "opencode",
			Source:      session.SourceOpenCode,
			ProjectPath: "/repo/opencode",
			FirstMsg:    "OpenCode work",
			SearchText:  "opencode work",
			LastTime:    time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC),
			MsgCount:    1,
		},
	}).SetSize(120, 20)

	_, cmd := l.Update(testKeyRunes("d"))
	if got, ok := runCmd(t, cmd).(SessionArchiveUnsupportedMsg); !ok || got.Session.Source != session.SourceOpenCode {
		t.Fatalf("d on OpenCode should emit unsupported archive, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}
}

func TestListModelSourceFilterCyclesThroughOpenCode(t *testing.T) {
	l := NewListModel().SetSessions([]session.Session{
		{ID: "claude", Source: session.SourceClaude, LastTime: time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC), MsgCount: 1},
		{ID: "codex", Source: session.SourceCodex, LastTime: time.Date(2026, 4, 12, 11, 0, 0, 0, time.UTC), MsgCount: 1},
		{ID: "opencode", Source: session.SourceOpenCode, LastTime: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC), MsgCount: 1},
	}).SetSize(120, 20)

	l, _ = l.Update(testKeyRunes("F"))
	if l.sourceFilterLabel() != "Claude" || len(l.filtered) != 1 || l.filtered[0].ID != "claude" {
		t.Fatalf("first source filter = %q %#v", l.sourceFilterLabel(), l.filtered)
	}
	l, _ = l.Update(testKeyRunes("F"))
	if l.sourceFilterLabel() != "Codex" || len(l.filtered) != 1 || l.filtered[0].ID != "codex" {
		t.Fatalf("second source filter = %q %#v", l.sourceFilterLabel(), l.filtered)
	}
	l, _ = l.Update(testKeyRunes("F"))
	if l.sourceFilterLabel() != "OpenCode" || len(l.filtered) != 1 || l.filtered[0].ID != "opencode" {
		t.Fatalf("third source filter = %q %#v", l.sourceFilterLabel(), l.filtered)
	}
}

func TestListModelCompactHelpFitsLeftWidth(t *testing.T) {
	const leftWidth = 82
	l := NewListModel().SetSessions(testSessions()).SetSize(leftWidth, 20)

	for _, line := range strings.Split(l.CompactView(leftWidth), "\n") {
		if strings.Contains(stripANSI(line), "q Quit") {
			if got := lipgloss.Width(line); got > leftWidth-2 {
				t.Fatalf("compact help width = %d, want <= %d; line=%q", got, leftWidth-2, stripANSI(line))
			}
			return
		}
	}
	t.Fatalf("compact help did not include q Quit: %q", stripANSI(l.CompactView(leftWidth)))
}

func TestListModelMouseInteractions(t *testing.T) {
	l := NewListModel().SetSessions(testSessions()).SetSize(120, 20)

	l, cmd := l.Update(testMouse(0, 2, tea.MouseButtonLeft))
	if l.cursor != 0 {
		t.Fatalf("left click cursor = %d, want 0", l.cursor)
	}
	msg := runCmd(t, cmd)
	got, ok := msg.(FilterChangedMsg)
	if !ok {
		t.Fatalf("left click should emit FilterChangedMsg, got %T", msg)
	}
	if got.Debounce {
		t.Fatalf("left click should request immediate side preview load")
	}

	_, cmd = l.Update(testMouse(0, 2, tea.MouseButtonLeft))
	if got, ok := runCmd(t, cmd).(SessionPreviewMsg); !ok || got.Session.ID != "alpha" {
		t.Fatalf("double click should emit SessionPreviewMsg for alpha, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	l, cmd = l.Update(testMouse(6, 3, tea.MouseButtonRight))
	if l.cursor != 1 {
		t.Fatalf("right click should move cursor to row 1, got %d", l.cursor)
	}
	if got, ok := runCmd(t, cmd).(OpenContextMenuMsg); !ok || got.Session.ID != "charlie" || got.X != 6 || got.Y != 3 {
		t.Fatalf("right click should emit OpenContextMenuMsg, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	l.cursor = 0
	l, cmd = l.Update(testMouse(0, 0, tea.MouseButtonWheelDown))
	if l.cursor != len(l.filtered)-1 {
		t.Fatalf("wheel down cursor = %d, want %d", l.cursor, len(l.filtered)-1)
	}
	if got, ok := runCmd(t, cmd).(FilterChangedMsg); !ok || !got.Debounce {
		t.Fatalf("wheel down should emit FilterChangedMsg, got %T", runCmd(t, cmd))
	}

	l, _ = l.Update(testMouse(0, 0, tea.MouseButtonWheelUp))
	if l.cursor != 0 {
		t.Fatalf("wheel up cursor = %d, want 0", l.cursor)
	}

	_, cmd = l.Update(testMouse(0, 0, tea.MouseButtonWheelUp))
	if cmd != nil {
		t.Fatal("wheel up at top should not emit a refresh command")
	}
}
