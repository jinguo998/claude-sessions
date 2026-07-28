package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func TestNewPreviewModelInitialState(t *testing.T) {
	p := NewPreviewModel()

	if p.title != "" {
		t.Fatalf("title = %q, want empty", p.title)
	}
	if p.viewport.Width != initialViewportW || p.viewport.Height != initialViewportH {
		t.Fatalf("viewport size = (%d,%d), want (%d,%d)", p.viewport.Width, p.viewport.Height, initialViewportW, initialViewportH)
	}
	if p.search.active {
		t.Fatal("search should start inactive")
	}
	if !p.markdown {
		t.Fatal("markdown rendering should start enabled")
	}
}

func TestPreviewModelSetContentAndSize(t *testing.T) {
	p := NewPreviewModel()
	content := "line one\nline two"

	p = p.SetContent("My Title", previewResult{content: content}, session.Session{FilePath: "/tmp/session.jsonl"})
	if p.title != "My Title" {
		t.Fatalf("title = %q, want %q", p.title, "My Title")
	}
	if p.search.content != content {
		t.Fatalf("search content = %q, want %q", p.search.content, content)
	}
	if !strings.Contains(p.viewport.View(), "line one") {
		t.Fatalf("viewport content = %q, want line one", p.viewport.View())
	}

	p = p.SetSize(100, 30)
	if p.width != 100 || p.height != 30 {
		t.Fatalf("stored size = (%d,%d), want (100,30)", p.width, p.height)
	}
	if p.viewport.Width != 96 || p.viewport.Height != 24 {
		t.Fatalf("viewport size = (%d,%d), want (96,24)", p.viewport.Width, p.viewport.Height)
	}
}

func TestPreviewModelCloseKeys(t *testing.T) {
	for _, key := range []tea.KeyMsg{testKey(tea.KeyEsc), testKeyRunes("q"), testKeyRunes("p")} {
		p := NewPreviewModel()
		_, cmd := p.Update(key)
		if _, ok := runCmd(t, cmd).(PreviewCloseMsg); !ok {
			t.Fatalf("key %q should emit PreviewCloseMsg, got %T", key.String(), runCmd(t, cmd))
		}
	}
}

func TestPreviewModelResumeKeys(t *testing.T) {
	p := NewPreviewModel().SetContent("Title", previewResult{content: "body"}, session.Session{
		ID:          "resume-me",
		ProjectPath: "/tmp/project",
		Source:      session.SourceClaude,
	})

	_, cmd := p.Update(testKeyRunes("r"))
	if got, ok := runCmd(t, cmd).(SessionSelectedMsg); !ok || got.PermissionMode != session.PermissionModeFast {
		t.Fatalf("r should emit default fast SessionSelectedMsg, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	_, cmd = p.Update(testKeyRunes("R"))
	if got, ok := runCmd(t, cmd).(SessionSelectedMsg); !ok || got.PermissionMode != session.PermissionModeSafe {
		t.Fatalf("R should emit safe SessionSelectedMsg, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}
}

func TestPreviewModelMarkdownToggle(t *testing.T) {
	p := NewPreviewModel().SetContent("Title", previewResult{content: "body"}, session.Session{
		FilePath: "/tmp/session.jsonl",
		Source:   session.SourceClaude,
	})

	p, cmd := p.Update(testKeyRunes("m"))
	if p.markdown {
		t.Fatal("m should disable markdown rendering")
	}
	if got, ok := runCmd(t, cmd).(PreviewReloadMsg); !ok || got.Verbose {
		t.Fatalf("m should emit PreviewReloadMsg preserving verbose=false, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}
	if view := stripANSI(p.View()); !strings.Contains(view, "[RAW]") || !strings.Contains(view, "Markdown") {
		t.Fatalf("raw preview should show raw mode and markdown toggle, got %q", view)
	}

	p, cmd = p.Update(testKeyRunes("m"))
	if !p.markdown {
		t.Fatal("second m should re-enable markdown rendering")
	}
	if _, ok := runCmd(t, cmd).(PreviewReloadMsg); !ok {
		t.Fatalf("second m should emit PreviewReloadMsg, got %T", runCmd(t, cmd))
	}
}

func TestPreviewModelHelpIncludesMouseSelectToggle(t *testing.T) {
	p := NewPreviewModel().SetContent("Title", previewResult{content: "body"}, session.Session{
		FilePath: "/tmp/session.jsonl",
		Source:   session.SourceClaude,
	})

	view := stripANSI(p.View())
	if !strings.Contains(view, "M Select text") {
		t.Fatalf("preview help missing mouse select toggle: %q", view)
	}
}

func TestPreviewModelDefaultResumeFallsBackToSafeForCodex(t *testing.T) {
	p := NewPreviewModel().SetContent("Title", previewResult{content: "body"}, session.Session{
		ID:          "codex-resume",
		ProjectPath: "/tmp/project",
		Source:      session.SourceCodex,
	})

	_, cmd := p.Update(testKeyRunes("r"))
	if got, ok := runCmd(t, cmd).(SessionSelectedMsg); !ok || got.PermissionMode != session.PermissionModeSafe {
		t.Fatalf("default resume on Codex should stay safe, got %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	if view := stripANSI(p.View()); strings.Contains(view, "Safe") {
		t.Fatalf("Codex preview help should hide explicit safe resume, got %q", view)
	}
}

func TestPreviewModelScrollAndSearch(t *testing.T) {
	p := NewPreviewModel().SetSize(40, 10)
	content := strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "line 9", "line 10",
	}, "\n")
	p = p.SetContent("Title", previewResult{content: content}, session.Session{FilePath: "/tmp/session.jsonl"})

	start := p.viewport.YOffset
	p, _ = p.Update(testKeyRunes("k"))
	if p.viewport.YOffset >= start {
		t.Fatalf("k should scroll up from %d, got %d", start, p.viewport.YOffset)
	}

	afterUp := p.viewport.YOffset
	p, _ = p.Update(testKeyRunes("j"))
	if p.viewport.YOffset <= afterUp {
		t.Fatalf("j should scroll down from %d, got %d", afterUp, p.viewport.YOffset)
	}

	p, cmd := p.Update(testKeyRunes("/"))
	if !p.search.active {
		t.Fatal("/ should open preview search")
	}
	if cmd == nil {
		t.Fatal("/ should return blink cmd")
	}
}

func TestPreviewModelVerboseToggle(t *testing.T) {
	p := NewPreviewModel()

	updated, cmd := p.Update(testKeyRunes("v"))
	p = updated
	if !p.verbose {
		t.Fatal("first v should enable verbose mode")
	}
	msg, ok := runCmd(t, cmd).(PreviewReloadMsg)
	if !ok {
		t.Fatalf("first v should emit PreviewReloadMsg, got %T", runCmd(t, cmd))
	}
	if !msg.Verbose {
		t.Fatalf("first v should emit verbose=true, got %#v", msg)
	}

	updated, cmd = p.Update(testKeyRunes("v"))
	p = updated
	if p.verbose {
		t.Fatal("second v should disable verbose mode")
	}
	msg, ok = runCmd(t, cmd).(PreviewReloadMsg)
	if !ok {
		t.Fatalf("second v should emit PreviewReloadMsg, got %T", runCmd(t, cmd))
	}
	if msg.Verbose {
		t.Fatalf("second v should emit verbose=false, got %#v", msg)
	}
}
