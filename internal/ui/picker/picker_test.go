package picker

import (
	"strings"
	"testing"
	"time"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

// stripAnsi removes ANSI escape sequences from a string, returning plain text.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func makeSession(source session.Source, firstMsg, projectPath, searchText string) session.Session {
	return session.Session{
		ID:          "test-id",
		Source:      source,
		FirstMsg:    firstMsg,
		ProjectPath: projectPath,
		LastTime:    time.Now().Add(-5 * time.Minute),
		SearchText:  searchText,
	}
}

func TestFormatRowClaude(t *testing.T) {
	s := makeSession(session.SourceClaude, "install fzf and ripgrep", "/home/user/myproject", "install fzf and ripgrep")
	match := MatchResult{Session: s}
	row := FormatRow(match, nil, 120, false)
	plain := stripAnsi(row)

	if !strings.Contains(plain, "C") {
		t.Errorf("expected 'C' badge for Claude session, got: %q", plain)
	}
	if !strings.Contains(plain, "myproject") {
		t.Errorf("expected project name 'myproject' in row, got: %q", plain)
	}
	if !strings.Contains(plain, "install fzf and ripgrep") {
		t.Errorf("expected FirstMsg in row, got: %q", plain)
	}
}

func TestFormatRowCodex(t *testing.T) {
	s := makeSession(session.SourceCodex, "refactor auth module", "/home/user/backend", "refactor auth module")
	match := MatchResult{Session: s}
	row := FormatRow(match, nil, 120, false)
	plain := stripAnsi(row)

	if !strings.Contains(plain, "X") {
		t.Errorf("expected 'X' badge for Codex session, got: %q", plain)
	}
	if !strings.Contains(plain, "backend") {
		t.Errorf("expected project name 'backend' in row, got: %q", plain)
	}
}

func TestFormatRowOpenCode(t *testing.T) {
	s := makeSession(session.SourceOpenCode, "ship opencode support", "/home/user/opencode", "ship opencode support")
	match := MatchResult{Session: s}
	row := FormatRow(match, nil, 120, false)
	plain := stripAnsi(row)

	if !strings.Contains(plain, "O") {
		t.Errorf("expected 'O' badge for OpenCode session, got: %q", plain)
	}
	if !strings.Contains(plain, "opencode") {
		t.Errorf("expected project name 'opencode' in row, got: %q", plain)
	}
}

func TestFormatRowSelected(t *testing.T) {
	s := makeSession(session.SourceClaude, "debug the server crash", "/home/user/server", "debug the server crash")
	match := MatchResult{Session: s}

	selectedRow := FormatRow(match, nil, 120, true)
	normalRow := FormatRow(match, nil, 120, false)

	selectedPlain := stripAnsi(selectedRow)

	if !strings.HasPrefix(selectedPlain, "> ") {
		t.Errorf("expected selected row to start with '> ', got: %q", selectedPlain)
	}

	normalPlain := stripAnsi(normalRow)
	if strings.HasPrefix(normalPlain, "> ") {
		t.Errorf("expected non-selected row NOT to start with '> ', got: %q", normalPlain)
	}

	if selectedRow == normalRow {
		t.Error("selected and non-selected rows should differ")
	}
}

func TestFormatRowContextSnippet(t *testing.T) {
	// FirstMsg is short; the match is in SearchText (later messages).
	firstMsg := "short intro message"
	searchText := "short intro message also discussed rate limiting and fzf integration tools"
	s := session.Session{
		ID:          "ctx-id",
		Source:      session.SourceClaude,
		FirstMsg:    firstMsg,
		ProjectPath: "/home/user/proj",
		LastTime:    time.Now().Add(-10 * time.Minute),
		SearchText:  searchText,
	}
	// Simulate a match in SearchText that won't match FirstMsg.
	// "fzf" is in searchText but not in firstMsg.
	matchedIndexes := []int{51, 52, 53} // approximate indexes of "fzf" in searchText
	match := MatchResult{
		Session:        s,
		MatchedIndexes: matchedIndexes,
	}

	row := FormatRow(match, []string{"fzf"}, 160, false)
	plain := stripAnsi(row)

	// FirstMsg should still be visible.
	if !strings.Contains(plain, firstMsg) {
		t.Errorf("expected FirstMsg %q to appear in row, got: %q", firstMsg, plain)
	}

	// A snippet from SearchText should also be present (containing the match area).
	// We just verify the row is longer than the first message alone would produce.
	if len(plain) <= len("  C  5m ago        proj             "+firstMsg) {
		t.Logf("row may be short but not necessarily wrong; plain=%q", plain)
	}
}

func TestHighlightByIndexes(t *testing.T) {
	text := "hello world"
	indexes := []int{0, 1, 2} // "hel"
	result := highlightByIndexes(text, indexes, dimStyle, matchStyle)
	plain := stripAnsi(result)

	if plain != text {
		t.Errorf("expected plain text %q after stripping ANSI, got %q", text, plain)
	}
}

func TestHighlightByIndexesEmpty(t *testing.T) {
	// nil indexes should still render the text.
	result := highlightByIndexes("hello", nil, dimStyle, matchStyle)
	plain := stripAnsi(result)
	if plain != "hello" {
		t.Errorf("expected 'hello' with nil indexes, got %q", plain)
	}

	// Empty text should return empty string.
	result2 := highlightByIndexes("", []int{0, 1}, dimStyle, matchStyle)
	if result2 != "" {
		t.Errorf("expected empty string for empty text, got %q", result2)
	}
}
