package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	apparchive "github.com/jinguo998/claude-sessions/internal/app/archive"
	session "github.com/jinguo998/claude-sessions/internal/app/model"
	"github.com/jinguo998/claude-sessions/internal/source"
	"github.com/jinguo998/claude-sessions/internal/source/claude"
	"github.com/jinguo998/claude-sessions/internal/source/codex"
	"github.com/jinguo998/claude-sessions/internal/storage"
	storagetrash "github.com/jinguo998/claude-sessions/internal/storage/trash"
)

func testArchiveService(home string) *apparchive.Service {
	claudeAdapter := claude.NewAdapter()
	codexAdapter := codex.NewAdapter()
	return apparchive.NewService([]source.ArchiveSpecifier{claudeAdapter, codexAdapter}, storagetrash.NewAt(home))
}

func TestModelSessionSelectionAndForkQuit(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
		want Result
	}{
		{
			name: "selected",
			msg: SessionSelectedMsg{Session: session.Session{
				ID:          "sess-1",
				ProjectPath: "/tmp/project",
				Source:      session.SourceClaude,
			}},
			want: Result{Dir: "/tmp/project", ID: "sess-1", PermissionMode: session.PermissionModeSafe, Source: session.SourceClaude},
		},
		{
			name: "selected fast",
			msg: SessionSelectedMsg{Session: session.Session{
				ID:          "sess-fast",
				ProjectPath: "/tmp/project",
				Source:      session.SourceClaude,
			}, PermissionMode: session.PermissionModeFast},
			want: Result{Dir: "/tmp/project", ID: "sess-fast", PermissionMode: session.PermissionModeFast, Source: session.SourceClaude},
		},
		{
			name: "fork",
			msg: SessionForkMsg{Session: session.Session{
				ID:          "sess-2",
				ProjectPath: "/tmp/project",
				Source:      session.SourceClaude,
			}, PermissionMode: session.PermissionModeFast},
			want: Result{Dir: "/tmp/project", ID: "sess-2", Fork: true, PermissionMode: session.PermissionModeFast, Source: session.SourceClaude},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel()
			updated, cmd := m.Update(tt.msg)
			gotModel := updated.(Model)
			if gotModel.Result() != tt.want {
				t.Fatalf("Result() = %#v, want %#v", gotModel.Result(), tt.want)
			}
			if _, ok := runCmd(t, cmd).(tea.QuitMsg); !ok {
				t.Fatalf("cmd should emit tea.QuitMsg, got %T", runCmd(t, cmd))
			}
		})
	}
}

func TestModelToggleMouseMode(t *testing.T) {
	m := InitialModel()
	m.view = viewPreview

	updated, cmd := m.Update(testKeyRunes("M"))
	got := updated.(Model)
	if got.mouseEnabled {
		t.Fatal("M should disable mouse mode")
	}
	if !strings.Contains(got.flash, "drag to select") {
		t.Fatalf("flash = %q, want select text hint", got.flash)
	}
	if cmd == nil {
		t.Fatal("disabling mouse should emit a command")
	}

	updated, cmd = got.Update(testKeyRunes("M"))
	got = updated.(Model)
	if !got.mouseEnabled {
		t.Fatal("second M should re-enable mouse mode")
	}
	if !strings.Contains(got.flash, "click and scroll") {
		t.Fatalf("flash = %q, want mouse enabled hint", got.flash)
	}
	if cmd == nil {
		t.Fatal("enabling mouse should emit a command")
	}
}

func applyPreviewLoadForTest(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("preview load command is nil")
	}
	msg := runCmd(t, cmd)
	if _, ok := msg.(PreviewLoadedMsg); !ok {
		t.Fatalf("preview load command emitted %T, want PreviewLoadedMsg", msg)
	}
	updated, next := m.Update(msg)
	if next != nil {
		t.Fatalf("preview loaded update should not emit command, got %v", next)
	}
	return updated.(Model)
}

func TestModelDebouncesWheelSidePreviewLoad(t *testing.T) {
	m := InitialModel()
	m.width = splitMinWidth
	m.height = 24
	m.list = m.list.SetSessions([]session.Session{
		{
			ID:          "wheel-a",
			ProjectPath: "/tmp/wheel",
			FilePath:    testSampleFilePath(t),
			Source:      session.SourceClaude,
			FirstMsg:    "Wheel preview",
		},
	})

	updated, cmd := m.Update(FilterChangedMsg{Debounce: true})
	got := updated.(Model)
	if got.sidePreviewSeq != 1 {
		t.Fatalf("sidePreviewSeq after debounce = %d, want 1", got.sidePreviewSeq)
	}
	if cmd == nil {
		t.Fatal("debounced filter change should schedule side preview load")
	}

	updated, cmd = got.Update(SidePreviewLoadMsg{Token: got.sidePreviewSeq})
	got = updated.(Model)
	if got.sidePreview.sessionID != "wheel-a" {
		t.Fatalf("side preview session after debounce = %q, want wheel-a", got.sidePreview.sessionID)
	}
	if cmd == nil {
		t.Fatal("current debounce token should start side preview load")
	}

	updated, cmd = got.Update(FilterChangedMsg{Debounce: true})
	got = updated.(Model)
	staleToken := got.sidePreviewSeq
	if cmd == nil {
		t.Fatal("second debounced filter change should schedule side preview load")
	}

	updated, _ = got.Update(FilterChangedMsg{})
	got = updated.(Model)
	if got.sidePreviewSeq != staleToken+1 {
		t.Fatalf("immediate filter change should invalidate pending debounce, seq = %d, want %d", got.sidePreviewSeq, staleToken+1)
	}

	_, cmd = got.Update(SidePreviewLoadMsg{Token: staleToken})
	if cmd != nil {
		t.Fatal("stale debounce token should not start side preview load")
	}
}

func TestModelMouseClickLoadsSidePreviewImmediately(t *testing.T) {
	m := InitialModel()
	m.width = splitMinWidth
	m.height = 24
	m.list = m.list.SetSessions([]session.Session{
		{
			ID:          "click-a",
			ProjectPath: "/tmp/click-a",
			FilePath:    testSampleFilePath(t),
			Source:      session.SourceClaude,
			FirstMsg:    "Click A",
		},
		{
			ID:          "click-b",
			ProjectPath: "/tmp/click-b",
			FilePath:    testSampleFilePath(t),
			Source:      session.SourceClaude,
			FirstMsg:    "Click B",
		},
	})

	updated, cmd := m.Update(testMouse(0, 3, tea.MouseButtonLeft))
	got := updated.(Model)
	if got.list.Cursor() != 1 {
		t.Fatalf("cursor after click = %d, want 1", got.list.Cursor())
	}
	msg := runCmd(t, cmd)
	if gotMsg, ok := msg.(FilterChangedMsg); !ok || gotMsg.Debounce {
		t.Fatalf("mouse click should emit immediate FilterChangedMsg, got %T %#v", msg, msg)
	}

	updated, cmd = got.Update(msg)
	got = updated.(Model)
	if got.sidePreviewSeq != 1 {
		t.Fatalf("sidePreviewSeq after click = %d, want immediate load seq 1", got.sidePreviewSeq)
	}
	if got.sidePreview.sessionID != "click-b" {
		t.Fatalf("side preview session after click = %q, want click-b", got.sidePreview.sessionID)
	}
	if cmd == nil {
		t.Fatal("mouse click should start side preview load immediately")
	}
}

func TestModelMouseClickInvalidatesOldFullSidePreview(t *testing.T) {
	m := InitialModel()
	m.width = splitMinWidth
	m.height = 24
	m.sidePreviewReq = 1
	m.sidePreviewSeq = 1
	m.sidePreview = NewSidePreviewModel().SetSize(m.height)
	m.sidePreview.sessionID = "click-a"
	m.sidePreview.requestToken = 1
	m.sidePreview.loadingMore = true
	m.sidePreview.content = "old tail"
	m.sidePreview.lines = []string{"old tail"}
	latestSidePreviewToken.Store(1)
	m.list = m.list.SetSessions([]session.Session{
		{
			ID:          "click-a",
			ProjectPath: "/tmp/click-a",
			FilePath:    testSampleFilePath(t),
			Source:      session.SourceClaude,
			FirstMsg:    "Click A",
		},
		{
			ID:          "click-b",
			ProjectPath: "/tmp/click-b",
			FilePath:    testSampleFilePath(t),
			Source:      session.SourceClaude,
			FirstMsg:    "Click B",
		},
	})

	updated, cmd := m.Update(testMouse(0, 3, tea.MouseButtonLeft))
	got := updated.(Model)
	msg := runCmd(t, cmd)
	updated, cmd = got.Update(msg)
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("mouse click should start new side preview load")
	}
	if got.sidePreview.sessionID != "click-b" || got.sidePreview.requestToken != 2 {
		t.Fatalf("new side preview = session %q token %d, want click-b token 2", got.sidePreview.sessionID, got.sidePreview.requestToken)
	}
	if latestSidePreviewToken.Load() != 2 {
		t.Fatalf("latest side preview token = %d, want 2", latestSidePreviewToken.Load())
	}

	updated, _ = got.Update(SidePreviewLoadedMsg{
		Token:     1,
		SessionID: "click-a",
		Content:   "old full content",
		Complete:  true,
	})
	got = updated.(Model)
	if strings.Contains(strings.Join(got.sidePreview.lines, "\n"), "old full content") {
		t.Fatalf("stale full load overwrote new side preview: %#v", got.sidePreview.lines)
	}
	if got.sidePreview.sessionID != "click-b" || got.sidePreview.requestToken != 2 {
		t.Fatalf("stale full load changed side preview state: %#v", got.sidePreview)
	}
}

func TestModelRightPaneWheelUsesSidePreviewScrollStep(t *testing.T) {
	m := InitialModel()
	m.width = splitMinWidth
	m.height = 24
	m.sidePreview = NewSidePreviewModel().SetSize(m.height)
	m.sidePreview.lines = make([]string, 100)

	updated, cmd := m.Update(testMouse(m.width/2+1, 5, tea.MouseButtonWheelDown))
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("right pane wheel should not emit command, got %v", cmd)
	}
	if got.sidePreview.scroll != sidePreviewScrollStep {
		t.Fatalf("right pane wheel down scroll = %d, want %d", got.sidePreview.scroll, sidePreviewScrollStep)
	}

	updated, _ = got.Update(testMouse(m.width/2+1, 5, tea.MouseButtonWheelUp))
	got = updated.(Model)
	if got.sidePreview.scroll != 0 {
		t.Fatalf("right pane wheel up scroll = %d, want 0", got.sidePreview.scroll)
	}
}

func TestModelRightPaneWheelAtTopLoadsFullSidePreview(t *testing.T) {
	m := InitialModel()
	m.width = splitMinWidth
	m.height = 24
	m.sidePreview = NewSidePreviewModel().SetSize(m.height)
	m.sidePreview.sessionID = "lazy"
	m.sidePreview.filePath = testSampleFilePath(t)
	m.sidePreview.source = session.SourceClaude
	m.sidePreview.lines = []string{"recent 1", "recent 2"}
	m.sidePreview.content = strings.Join(m.sidePreview.lines, "\n")
	m.sidePreview.complete = false
	m.sidePreview.scroll = 0

	updated, cmd := m.Update(testMouse(m.width/2+1, 5, tea.MouseButtonWheelUp))
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("wheel up at top should start full side preview load")
	}
	if !got.sidePreview.loadingMore {
		t.Fatal("side preview should mark full load in progress")
	}
	if got.sidePreview.requestToken != 1 {
		t.Fatalf("full side preview token = %d, want 1", got.sidePreview.requestToken)
	}
}

func TestModelIgnoresStaleSessionsLoadedMsg(t *testing.T) {
	m := InitialModel()
	m.scanSeq = 2

	updated, cmd := m.Update(SessionsLoadedMsg{
		Token: 1,
		Sessions: []session.Session{
			{ID: "old", FirstMsg: "Old scan", MsgCount: 1},
		},
	})
	got := updated.(Model)
	if len(got.list.sessions) != 0 {
		t.Fatalf("stale scan updated sessions: %#v", got.list.sessions)
	}
	if cmd != nil {
		t.Fatalf("stale scan should not return cmd, got %v", cmd)
	}

	updated, _ = got.Update(SessionsLoadedMsg{
		Token: 2,
		Sessions: []session.Session{
			{ID: "new", FirstMsg: "New scan", MsgCount: 1},
		},
	})
	got = updated.(Model)
	if len(got.list.sessions) != 1 || got.list.sessions[0].ID != "new" {
		t.Fatalf("current scan sessions = %#v, want new", got.list.sessions)
	}
}

func TestModelRefreshIncrementsScanToken(t *testing.T) {
	m := InitialModel()
	m.scanSeq = 10

	updated, cmd := m.Update(RefreshMsg{})
	got := updated.(Model)
	if got.scanSeq != 11 {
		t.Fatalf("scanSeq after refresh = %d, want 11", got.scanSeq)
	}
	if cmd == nil {
		t.Fatal("refresh should return scan command")
	}
}

func TestModelResizeIntoWideLoadsSidePreview(t *testing.T) {
	sess := session.Session{
		ID:          "resize-wide",
		ProjectPath: "/tmp/resize",
		FilePath:    testSampleFilePath(t),
		Source:      session.SourceClaude,
		FirstMsg:    "Resize preview",
	}
	m := InitialModel()
	m.width = splitMinWidth - 1
	m.height = 24
	m.list = m.list.SetSize(m.width, m.height).SetSessions([]session.Session{sess})

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: splitMinWidth, Height: 24})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("resizing from narrow into wide mode should start side preview load")
	}
	if got.sidePreview.sessionID != sess.ID {
		t.Fatalf("side preview sessionID = %q, want %q", got.sidePreview.sessionID, sess.ID)
	}
	if !got.sidePreview.loading {
		t.Fatal("side preview should enter loading state after wide resize")
	}
}

func TestModelResizeNarrowInvalidatesSidePreview(t *testing.T) {
	m := InitialModel()
	m.width = splitMinWidth
	m.height = 24
	m.sidePreviewSeq = 4
	m.sidePreviewReq = 8
	m.sidePreview = NewSidePreviewModel().SetSize(24)
	m.sidePreview.sessionID = "active"
	m.sidePreview.requestToken = 8
	m.sidePreview.loading = true
	latestSidePreviewToken.Store(8)

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: splitMinWidth - 1, Height: 24})
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("resize to narrow should not return cmd, got %v", cmd)
	}
	if got.sidePreview.sessionID != "" || got.sidePreview.loading {
		t.Fatalf("side preview after narrow resize = %#v, want reset", got.sidePreview)
	}
	if got.sidePreviewSeq != 5 {
		t.Fatalf("sidePreviewSeq after narrow resize = %d, want 5", got.sidePreviewSeq)
	}
	if got.sidePreviewReq != 9 {
		t.Fatalf("sidePreviewReq after narrow resize = %d, want 9", got.sidePreviewReq)
	}
	if latestSidePreviewToken.Load() != 9 {
		t.Fatalf("latestSidePreviewToken = %d, want 9", latestSidePreviewToken.Load())
	}
}

func TestModelViewStateRouting(t *testing.T) {
	sess := session.Session{
		ID:          "preview-1",
		ProjectDir:  "-Users-example-demo",
		ProjectPath: "/Users/example/demo/project",
		FirstMsg:    "Open preview",
		FilePath:    testSampleFilePath(t),
	}

	m := InitialModel()
	m.width = 100

	updated, _ := m.Update(SessionPreviewMsg{Session: sess})
	var cmd tea.Cmd
	got := updated.(Model)
	if got.view != viewPreview {
		t.Fatalf("view after preview = %v, want %v", got.view, viewPreview)
	}
	if got.preview.title == "" {
		t.Fatal("preview title should be populated")
	}

	updated, _ = got.Update(SessionDeleteMsg{Index: 0})
	got = updated.(Model)
	if got.view != viewConfirmDelete {
		t.Fatalf("view after delete msg = %v, want %v", got.view, viewConfirmDelete)
	}

	updated, _ = got.Update(OpenContextMenuMsg{Session: sess, X: 4, Y: 6})
	got = updated.(Model)
	if got.view != viewContextMenu {
		t.Fatalf("view after open context menu = %v, want %v", got.view, viewContextMenu)
	}

	updated, _ = got.Update(MenuCloseMsg{})
	got = updated.(Model)
	if got.view != viewList {
		t.Fatalf("view after menu close = %v, want %v", got.view, viewList)
	}

	updated, _ = got.Update(OpenStatsMsg{})
	got = updated.(Model)
	if got.view != viewStats {
		t.Fatalf("view after open stats = %v, want %v", got.view, viewStats)
	}
	if got.stats.Cursor() != 0 {
		t.Fatalf("stats cursor = %d, want 0", got.stats.Cursor())
	}

	updated, cmd = got.Update(testKeyRunes("q"))
	got = updated.(Model)
	updated, _ = got.Update(runCmd(t, cmd))
	got = updated.(Model)
	if got.view != viewList {
		t.Fatalf("view after stats close = %v, want %v", got.view, viewList)
	}

	updated, cmd = got.Update(OpenTrashMsg{})
	got = updated.(Model)
	if got.view != viewTrash {
		t.Fatalf("view after open trash = %v, want %v", got.view, viewTrash)
	}
	if cmd == nil {
		t.Fatal("open trash should trigger load cmd")
	}

	got.view = viewPreview
	updated, _ = got.Update(PreviewCloseMsg{})
	got = updated.(Model)
	if got.view != viewList {
		t.Fatalf("view after preview close = %v, want %v", got.view, viewList)
	}
}

func TestModelStatsActions(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	m := InitialModel()
	m.list = m.list.SetSessions([]session.Session{
		{
			ID:          "alpha",
			ProjectPath: "/Users/example/alpha",
			Source:      session.SourceClaude,
			FirstMsg:    "Alpha work",
			LastTime:    now.Add(-1 * time.Hour),
			MsgCount:    10,
			ToolCount:   8,
			TokenUsage:  session.TokenUsage{Input: 1000, Output: 500},
		},
		{
			ID:          "bravo",
			ProjectPath: "/Users/example/bravo",
			Source:      session.SourceClaude,
			FirstMsg:    "Bravo work",
			LastTime:    now.Add(-2 * time.Hour),
			MsgCount:    6,
			ToolCount:   2,
			TokenUsage:  session.TokenUsage{Input: 100, Output: 50},
		},
	})
	m.view = viewStats

	updated, _ := m.Update(testKeyRunes("j"))
	got := updated.(Model)
	if got.stats.Cursor() != 1 {
		t.Fatalf("stats cursor after j = %d, want 1", got.stats.Cursor())
	}

	updated, cmd := got.Update(testKey(tea.KeyEnter))
	got = updated.(Model)
	msg := runCmd(t, cmd)
	updated, cmd = got.Update(msg)
	got = updated.(Model)
	if got.view != viewPreview {
		t.Fatalf("stats enter should open preview, got view %v", got.view)
	}
	if got.previewReturn != viewStats {
		t.Fatalf("previewReturn = %v, want %v", got.previewReturn, viewStats)
	}
	if cmd == nil {
		t.Fatal("stats enter should emit async preview load command")
	}

	updated, _ = got.Update(PreviewCloseMsg{})
	got = updated.(Model)
	if got.view != viewStats {
		t.Fatalf("preview close should return to stats, got view %v", got.view)
	}

	updated, cmd = got.Update(testKeyRunes("r"))
	got = updated.(Model)
	msg = runCmd(t, cmd)
	gotSel, ok := msg.(SessionSelectedMsg)
	if !ok {
		t.Fatalf("stats r should emit SessionSelectedMsg, got %T", msg)
	}
	if gotSel.Session.ID != "bravo" || gotSel.PermissionMode != session.PermissionModeFast {
		t.Fatalf("unexpected stats resume msg: %#v", gotSel)
	}

	updated, cmd = got.Update(testKeyRunes("R"))
	got = updated.(Model)
	msg = runCmd(t, cmd)
	gotSel, ok = msg.(SessionSelectedMsg)
	if !ok {
		t.Fatalf("stats R should emit SessionSelectedMsg, got %T", msg)
	}
	if gotSel.Session.ID != "bravo" || gotSel.PermissionMode != session.PermissionModeSafe {
		t.Fatalf("unexpected stats safe resume msg: %#v", gotSel)
	}

	updated, cmd = got.Update(testKeyRunes("y"))
	got = updated.(Model)
	msg = runCmd(t, cmd)
	copyMsg, ok := msg.(CopyIDMsg)
	if !ok || copyMsg.ID != "bravo" {
		t.Fatalf("stats y should emit CopyIDMsg for bravo, got %T %#v", msg, msg)
	}

	updated, cmd = got.Update(testKeyRunes("o"))
	got = updated.(Model)
	if cmd != nil {
		t.Fatalf("stats o with empty file path should not emit a command, got %v", cmd)
	}

	got.list = got.list.SetSessions([]session.Session{
		{
			ID:          "alpha",
			ProjectPath: "/Users/example/alpha",
			Source:      session.SourceClaude,
			FirstMsg:    "Alpha work",
			LastTime:    now.Add(-1 * time.Hour),
			MsgCount:    10,
			ToolCount:   8,
			TokenUsage:  session.TokenUsage{Input: 1000, Output: 500},
		},
		{
			ID:          "bravo",
			ProjectPath: "/Users/example/bravo",
			FilePath:    "/tmp/bravo.jsonl",
			Source:      session.SourceClaude,
			FirstMsg:    "Bravo work",
			LastTime:    now.Add(-2 * time.Hour),
			MsgCount:    6,
			ToolCount:   2,
			TokenUsage:  session.TokenUsage{Input: 100, Output: 50},
		},
	})
	got.view = viewStats
	got.stats.cursor = 1

	updated, cmd = got.Update(testKeyRunes("o"))
	got = updated.(Model)
	msg = runCmd(t, cmd)
	openMsg, ok := msg.(OpenEditorMsg)
	if !ok || openMsg.FilePath != "/tmp/bravo.jsonl" {
		t.Fatalf("stats o should emit OpenEditorMsg for bravo, got %T %#v", msg, msg)
	}

	updated, cmd = got.Update(testKeyRunes("f"))
	got = updated.(Model)
	updated, cmd = got.Update(runCmd(t, cmd))
	got = updated.(Model)
	if got.list.filterProj != "/Users/example/bravo" {
		t.Fatalf("stats f filterProj = %q, want bravo project", got.list.filterProj)
	}
	if len(got.list.Filtered()) != 1 || got.list.Filtered()[0].ID != "bravo" {
		t.Fatalf("stats f filtered sessions = %#v, want only bravo", got.list.Filtered())
	}
	if got.stats.Cursor() != 0 {
		t.Fatalf("stats f should reset cursor, got %d", got.stats.Cursor())
	}
	if cmd == nil {
		t.Fatal("stats f should return flash command")
	}

	got.view = viewStats
	got.stats.cursor = 0
	updated, cmd = got.Update(testKeyRunes("l"))
	got = updated.(Model)
	updated, cmd = got.Update(runCmd(t, cmd))
	got = updated.(Model)
	if got.view != viewList {
		t.Fatalf("stats l should return to list, got view %v", got.view)
	}
	if got.list.Cursor() != 0 || len(got.list.Filtered()) != 1 || got.list.Filtered()[0].ID != "bravo" {
		t.Fatalf("stats l should focus filtered bravo session, got cursor=%d filtered=%#v", got.list.Cursor(), got.list.Filtered())
	}
	if cmd == nil {
		t.Fatal("stats l should return a flash clear command")
	}
}

func TestModelStatsMouseSelectionAndPreview(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	m := InitialModel()
	m.width = 140
	m.list = m.list.SetSessions([]session.Session{
		{
			ID:          "alpha",
			ProjectPath: "/Users/example/alpha",
			Source:      session.SourceClaude,
			FirstMsg:    "Alpha work",
			LastTime:    now.Add(-1 * time.Hour),
			MsgCount:    10,
			ToolCount:   8,
			TokenUsage:  session.TokenUsage{Input: 1000, Output: 500},
		},
		{
			ID:          "bravo",
			ProjectPath: "/Users/example/bravo",
			Source:      session.SourceClaude,
			FirstMsg:    "Bravo work",
			LastTime:    now.Add(-2 * time.Hour),
			MsgCount:    6,
			ToolCount:   2,
			TokenUsage:  session.TokenUsage{Input: 100, Output: 50},
		},
	})
	m.view = viewStats
	m.stats = m.stats.SetSize(m.width, m.height).SetSessions(m.list.Filtered(), len(m.list.sessions), m.statsScopeSummary())
	rendered := m.stats.RenderResultForTest(now)
	rowY := rendered.queueRowStart + 1

	updated, _ := m.Update(testMouse(5, rowY, tea.MouseButtonLeft))
	got := updated.(Model)
	if got.stats.Cursor() != 1 {
		t.Fatalf("stats cursor after click = %d, want 1", got.stats.Cursor())
	}

	updated, cmd := got.Update(testMouse(5, rowY, tea.MouseButtonLeft))
	got = updated.(Model)
	updated, _ = got.Update(runCmd(t, cmd))
	got = updated.(Model)
	if got.view != viewPreview {
		t.Fatalf("double click should open preview, got view %v", got.view)
	}
	if got.preview.session.ID != "bravo" {
		t.Fatalf("preview session = %q, want bravo", got.preview.session.ID)
	}
}

func TestModelWindowSizeUpdatesComponents(t *testing.T) {
	m := InitialModel()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	got := updated.(Model)

	if got.width != 140 || got.height != 40 {
		t.Fatalf("model size = (%d,%d), want (140,40)", got.width, got.height)
	}
	if got.list.width != 140 || got.list.height != 40 {
		t.Fatalf("list size = (%d,%d), want (140,40)", got.list.width, got.list.height)
	}
	if got.preview.width != 140 || got.preview.height != 40 {
		t.Fatalf("preview size = (%d,%d), want (140,40)", got.preview.width, got.preview.height)
	}
	if got.trash.width != 140 || got.trash.height != 40 {
		t.Fatalf("trash size = (%d,%d), want (140,40)", got.trash.width, got.trash.height)
	}
}

func TestModelDeleteConfirm(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tmpDir := filepath.Join(tmpHome, "workspace")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	filePath := filepath.Join(tmpDir, "delete-me.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sideDir := filepath.Join(tmpDir, "delete-me")
	if err := os.Mkdir(sideDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	sess := session.Session{
		ID:          "delete-1",
		ProjectDir:  "-Users-example-demo",
		ProjectPath: "/Users/example/demo",
		FilePath:    filePath,
		Source:      session.SourceClaude,
	}

	m := InitialModel(Services{Archive: testArchiveService(tmpHome)})
	m.list = m.list.SetSessions([]session.Session{sess})
	m.deleteIdx = 0
	m.view = viewConfirmDelete

	updated, cmd := m.Update(testKeyRunes("y"))
	got := updated.(Model)
	if got.view != viewList {
		t.Fatalf("view after confirm delete = %v, want %v", got.view, viewList)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("session file should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(sideDir); !os.IsNotExist(err) {
		t.Fatalf("side directory should be removed, stat err = %v", err)
	}
	if cmd == nil {
		t.Fatal("confirm delete should trigger reload cmd")
	}
	archiveRoot := filepath.Join(tmpHome, ".claude-sessions-trash")
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		t.Fatalf("ReadDir(archiveRoot) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive entries = %d, want 1", len(entries))
	}
	archivedDir := filepath.Join(archiveRoot, entries[0].Name())
	for _, path := range []string{
		filepath.Join(archivedDir, "delete-me.jsonl"),
		filepath.Join(archivedDir, "delete-me"),
		filepath.Join(archivedDir, "metadata.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected archived path %q, stat err = %v", path, err)
		}
	}

	filePath = filepath.Join(tmpDir, "keep.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	keep := session.Session{ID: "keep-1", FilePath: filePath}
	m = InitialModel(Services{Archive: testArchiveService(tmpHome)})
	m.list = m.list.SetSessions([]session.Session{keep})
	m.deleteIdx = 0
	m.view = viewConfirmDelete

	updated, cmd = m.Update(testKeyRunes("n"))
	got = updated.(Model)
	if got.view != viewList {
		t.Fatalf("view after cancel delete = %v, want %v", got.view, viewList)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("cancel delete should keep file, stat err = %v", err)
	}
	if cmd != nil {
		t.Fatalf("cancel delete should not reload, got cmd %v", cmd)
	}
}

func TestModelRenderConfirmAndHelpHandleNarrowInputs(t *testing.T) {
	m := InitialModel()
	m.width = 10
	m.list = m.list.SetSessions([]session.Session{{
		ID:          "abc",
		ProjectPath: "/tmp/project",
		FirstMsg:    "Short ID",
	}})
	m.deleteIdx = 0

	if got := stripANSI(m.renderDeleteConfirm()); !strings.Contains(got, "abc") {
		t.Fatalf("renderDeleteConfirm() = %q, want short ID", got)
	}
	if got := stripANSI(m.renderHelp()); !strings.Contains(got, "Keyboard Shortcuts") {
		t.Fatalf("renderHelp() = %q, want title", got)
	}
}

func TestModelTrashPreviewAndPermanentDelete(t *testing.T) {
	archiveDir := t.TempDir()
	item := storage.ArchivedSession{
		ArchiveDir: archiveDir,
		Metadata: storage.ArchivedSessionMetadata{
			ID:               "trash-delete-1",
			Source:           session.SourceClaude,
			OriginalFilePath: "/Users/example/demo/session.jsonl",
			ProjectPath:      "/Users/example/demo",
			Title:            "Archived preview",
		},
		SessionFile: filepath.Join(archiveDir, "session.jsonl"),
	}
	if err := os.WriteFile(item.SessionFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	m := InitialModel(Services{Archive: testArchiveService(t.TempDir())})
	m.width = 100
	m.view = viewTrash
	m.trash = m.trash.SetItems([]storage.ArchivedSession{item}, nil)

	updated, _ := m.Update(TrashPreviewMsg{Item: item})
	got := updated.(Model)
	if got.view != viewPreview {
		t.Fatalf("trash preview view = %v, want %v", got.view, viewPreview)
	}
	if got.previewReturn != viewTrash {
		t.Fatalf("trash preview return = %v, want %v", got.previewReturn, viewTrash)
	}

	updated, _ = got.Update(PreviewCloseMsg{})
	got = updated.(Model)
	if got.view != viewTrash {
		t.Fatalf("trash preview close view = %v, want %v", got.view, viewTrash)
	}

	updated, _ = got.Update(TrashDeleteMsg{Item: item})
	got = updated.(Model)
	if got.view != viewConfirmTrashDelete {
		t.Fatalf("trash delete view = %v, want %v", got.view, viewConfirmTrashDelete)
	}

	updated, cmd := got.Update(testKeyRunes("y"))
	got = updated.(Model)
	if got.view != viewTrash {
		t.Fatalf("trash delete confirm view = %v, want %v", got.view, viewTrash)
	}
	if _, err := os.Stat(archiveDir); !os.IsNotExist(err) {
		t.Fatalf("archive dir should be deleted, stat err = %v", err)
	}
	if cmd == nil {
		t.Fatal("trash delete confirm should reload trash")
	}
}

func TestModelMenuActionRouting(t *testing.T) {
	sess := session.Session{
		ID:          "menu-1",
		ProjectDir:  "-Users-example-demo",
		ProjectPath: "/Users/example/demo/project",
		FirstMsg:    "Inspect preview",
		FilePath:    testSampleFilePath(t),
		Source:      session.SourceClaude,
	}

	tests := []struct {
		name string
		msg  MenuActionMsg
		want Result
	}{
		{
			msg:  MenuActionMsg{Action: ActionResumeFast, Session: sess},
			want: Result{Dir: sess.ProjectPath, ID: sess.ID, PermissionMode: session.PermissionModeFast, Source: sess.Source},
		},
		{
			name: "resume safe",
			msg:  MenuActionMsg{Action: ActionResumeSafe, Session: sess},
			want: Result{Dir: sess.ProjectPath, ID: sess.ID, PermissionMode: session.PermissionModeSafe, Source: sess.Source},
		},
		{
			name: "fork",
			msg:  MenuActionMsg{Action: ActionFork, Session: sess},
			want: Result{Dir: sess.ProjectPath, ID: sess.ID, Fork: true, PermissionMode: session.PermissionModeFast, Source: sess.Source},
		},
		{
			name: "cd",
			msg:  MenuActionMsg{Action: ActionCd, Session: sess},
			want: Result{Dir: sess.ProjectPath, ID: sess.ID, CdOnly: true, PermissionMode: session.PermissionModeSafe, Source: sess.Source},
		},
	}

	t.Run("resume default codex falls back to safe", func(t *testing.T) {
		m := InitialModel()
		codexSess := sess
		codexSess.Source = session.SourceCodex

		updated, cmd := m.Update(MenuActionMsg{Action: ActionResumeFast, Session: codexSess})
		got := updated.(Model)
		want := Result{Dir: codexSess.ProjectPath, ID: codexSess.ID, PermissionMode: session.PermissionModeSafe, Source: codexSess.Source}
		if got.Result() != want {
			t.Fatalf("Result() = %#v, want %#v", got.Result(), want)
		}
		if _, ok := runCmd(t, cmd).(tea.QuitMsg); !ok {
			t.Fatalf("cmd should emit tea.QuitMsg, got %T", runCmd(t, cmd))
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel()

			updated, cmd := m.Update(tt.msg)
			got := updated.(Model)
			if got.Result() != tt.want {
				t.Fatalf("Result() = %#v, want %#v", got.Result(), tt.want)
			}

			msg := runCmd(t, cmd)
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Fatalf("cmd should emit tea.QuitMsg, got %T", msg)
			}
		})
	}

	t.Run("preview", func(t *testing.T) {
		m := InitialModel()
		m.width = 100

		updated, cmd := m.Update(MenuActionMsg{Action: ActionPreview, Session: sess})
		got := updated.(Model)
		if got.view != viewPreview {
			t.Fatalf("view after preview = %v, want %v", got.view, viewPreview)
		}
		if got.preview.title == "" {
			t.Fatal("preview title should be populated")
		}
		if got.preview.filePath != sess.FilePath {
			t.Fatalf("preview filePath = %q, want %q", got.preview.filePath, sess.FilePath)
		}
		if cmd == nil {
			t.Fatal("preview action should emit async preview load command")
		}
	})

	t.Run("delete", func(t *testing.T) {
		m := InitialModel()
		m.list = m.list.SetSessions([]session.Session{sess})

		updated, cmd := m.Update(MenuActionMsg{Action: ActionDelete, Session: sess})
		got := updated.(Model)
		if got.view != viewConfirmDelete {
			t.Fatalf("view after delete = %v, want %v", got.view, viewConfirmDelete)
		}
		if got.deleteIdx != 0 {
			t.Fatalf("deleteIdx = %d, want 0", got.deleteIdx)
		}
		if cmd != nil {
			t.Fatalf("delete action should not return cmd, got %v", cmd)
		}
	})
}

func TestModelPreviewReloadMsg(t *testing.T) {
	filePath := filepath.Join(filepath.Dir(testSampleFilePath(t)), "verbose_sample.jsonl")

	sess := session.Session{
		ID:          "preview-reload",
		ProjectDir:  "-Users-example-demo",
		ProjectPath: "/Users/example/demo/project",
		FirstMsg:    "Verbose reload",
		FilePath:    filePath,
		Source:      session.SourceClaude,
	}

	m := InitialModel(Services{Preview: testPreviewService()})
	m.width = 80

	updated, cmd := m.Update(SessionPreviewMsg{Session: sess})
	got := updated.(Model)
	if !got.preview.loading {
		t.Fatal("initial preview should enter loading state")
	}
	got = applyPreviewLoadForTest(t, got, cmd)
	before := stripANSI(got.preview.search.content)
	if before == "" {
		t.Fatal("initial preview content should not be empty")
	}
	if strings.Contains(before, "AgentTeamConfig") {
		t.Fatalf("summary preview should not include tool_result content, got %q", before)
	}

	updated, cmd = got.Update(PreviewReloadMsg{Verbose: true})
	got = updated.(Model)
	if !got.preview.loading {
		t.Fatal("preview reload should enter loading state")
	}
	got = applyPreviewLoadForTest(t, got, cmd)
	after := stripANSI(got.preview.search.content)
	if after == "" {
		t.Fatal("reloaded preview content should not be empty")
	}
	if !strings.Contains(after, "AgentTeamConfig") {
		t.Fatalf("verbose preview should include tool_result content, got %q", after)
	}
	if !strings.Contains(after, "Read /Users/example/go/src/example.com/sample-org/sample-service/biz/han") {
		t.Fatalf("verbose preview should still include subsequent tool detail, got %q", after)
	}
	if after == before {
		t.Fatalf("verbose reload should change preview content, before=%q after=%q", before, after)
	}
}
