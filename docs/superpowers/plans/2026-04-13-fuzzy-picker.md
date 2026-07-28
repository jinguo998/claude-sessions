# Fuzzy Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an fzf-style inline fuzzy session picker (`claude-sessions pick`) with shell hotkey integration.

**Architecture:** New `internal/picker/` package with two files: `fuzzy.go` (matching logic + snippet extraction) and `picker.go` (Bubble Tea inline TUI). The `pick` subcommand in `main.go` loads sessions synchronously, launches a non-alt-screen Bubble Tea program, and on selection does `os.Chdir` + `syscall.Exec` to resume. Shell init is split into zsh/bash with `ctrl+s` keybinding widget.

**Tech Stack:** Go, Bubble Tea (inline mode), `sahilm/fuzzy` (fuzzy matching), lipgloss (styling), `go-runewidth` (CJK width)

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/picker/fuzzy.go` (create) | Fuzzy matching wrapper: `FuzzyFind`, `FuzzyMatchFirstMsg`, `ExtractSnippet` |
| `internal/picker/fuzzy_test.go` (create) | Tests for all matching and snippet functions |
| `internal/picker/picker.go` (create) | `PickerModel` (Bubble Tea): styles, `FormatRow`, `Model`, `Update`, `View` |
| `internal/picker/picker_test.go` (create) | Tests for row formatting |
| `cmd/claude-sessions/main.go` (modify) | `pick` subcommand + split `shellInit` into zsh/bash with keybinding |

---

### Task 1: Fuzzy Matching Core

**Files:**
- Create: `internal/picker/fuzzy.go`
- Create: `internal/picker/fuzzy_test.go`
- Modify: `go.mod` (add `sahilm/fuzzy`)

- [ ] **Step 1: Add the fuzzy dependency**

```bash
cd ~/claude-sessions && go get github.com/sahilm/fuzzy
```

- [ ] **Step 2: Write failing tests for FuzzyFind, FuzzyMatchFirstMsg, ExtractSnippet**

Create `internal/picker/fuzzy_test.go`:

```go
package picker

import (
	"strings"
	"testing"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func TestFuzzyFindMatchesSessions(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", FirstMsg: "install fzf and ripgrep", SearchText: "install fzf and ripgrep"},
		{ID: "2", FirstMsg: "fix the login bug", SearchText: "fix the login bug"},
		{ID: "3", FirstMsg: "add rate limiting", SearchText: "add rate limiting also discussed fzf integration"},
	}

	results := FuzzyFind("fzf", sessions)
	if len(results) < 1 {
		t.Fatal("expected at least 1 match for 'fzf'")
	}

	// Session 1 has "fzf" directly in FirstMsg/SearchText — should be top match
	if results[0].Session.ID != "1" {
		t.Errorf("expected first match to be session 1, got %s", results[0].Session.ID)
	}

	// All results should have MatchedIndexes
	for i, r := range results {
		if len(r.MatchedIndexes) == 0 {
			t.Errorf("result %d has no MatchedIndexes", i)
		}
	}
}

func TestFuzzyFindEmptyQuery(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", FirstMsg: "hello"},
		{ID: "2", FirstMsg: "world"},
	}
	results := FuzzyFind("", sessions)
	if len(results) != 2 {
		t.Fatalf("expected all %d sessions for empty query, got %d", len(sessions), len(results))
	}
}

func TestFuzzyFindNoMatch(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", SearchText: "hello world"},
	}
	results := FuzzyFind("zzzzz", sessions)
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestFuzzyFindNonContiguous(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", SearchText: "file zero search"},
	}
	// "fzs" should match f-ile z-ero s-earch (non-contiguous)
	results := FuzzyFind("fzs", sessions)
	if len(results) != 1 {
		t.Fatalf("expected 1 non-contiguous match, got %d", len(results))
	}
}

func TestFuzzyMatchFirstMsg(t *testing.T) {
	indexes := FuzzyMatchFirstMsg("fzf", "install fzf and ripgrep")
	if indexes == nil {
		t.Fatal("expected match in FirstMsg")
	}
	if len(indexes) != 3 {
		t.Fatalf("expected 3 matched indexes for 'fzf', got %d", len(indexes))
	}
}

func TestFuzzyMatchFirstMsgNoMatch(t *testing.T) {
	indexes := FuzzyMatchFirstMsg("xyz", "install fzf and ripgrep")
	if indexes != nil {
		t.Fatalf("expected no match, got %v", indexes)
	}
}

func TestFuzzyMatchFirstMsgEmpty(t *testing.T) {
	if FuzzyMatchFirstMsg("", "hello") != nil {
		t.Error("empty query should return nil")
	}
	if FuzzyMatchFirstMsg("hello", "") != nil {
		t.Error("empty firstMsg should return nil")
	}
}

func TestExtractSnippet(t *testing.T) {
	text := "this is a long search text with fzf mentioned somewhere in the middle of it all"
	// "fzf" starts at rune index 31
	indexes := []int{31, 32, 33}
	snippet, adjusted := ExtractSnippet(text, indexes, 30)
	if snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
	if !strings.Contains(snippet, "fzf") {
		t.Errorf("snippet should contain 'fzf', got: %s", snippet)
	}
	if len(adjusted) == 0 {
		t.Fatal("expected adjusted indexes")
	}
}

func TestExtractSnippetShortText(t *testing.T) {
	text := "short text"
	indexes := []int{0, 1}
	snippet, adjusted := ExtractSnippet(text, indexes, 30)
	if snippet != text {
		t.Errorf("expected full text for short input, got: %s", snippet)
	}
	if len(adjusted) != 2 || adjusted[0] != 0 || adjusted[1] != 1 {
		t.Errorf("expected unchanged indexes for short text, got: %v", adjusted)
	}
}

func TestExtractSnippetEmpty(t *testing.T) {
	snippet, adjusted := ExtractSnippet("", []int{0}, 30)
	if snippet != "" {
		t.Errorf("expected empty snippet for empty text, got: %s", snippet)
	}
	if adjusted != nil {
		t.Errorf("expected nil adjusted for empty text, got: %v", adjusted)
	}

	snippet2, adjusted2 := ExtractSnippet("hello", nil, 30)
	if snippet2 != "" {
		t.Errorf("expected empty snippet for nil indexes, got: %s", snippet2)
	}
	if adjusted2 != nil {
		t.Errorf("expected nil adjusted for nil indexes, got: %v", adjusted2)
	}
}

func TestExtractSnippetEllipsis(t *testing.T) {
	text := "aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll"
	// Match "gggg" starting at rune 30
	indexes := []int{30, 31, 32, 33}
	snippet, _ := ExtractSnippet(text, indexes, 20)
	if !strings.HasPrefix(snippet, "...") {
		t.Errorf("expected leading ellipsis, got: %s", snippet)
	}
	if !strings.HasSuffix(snippet, "...") {
		t.Errorf("expected trailing ellipsis, got: %s", snippet)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd ~/claude-sessions && go test -v ./internal/picker/
```

Expected: compilation error — package and functions don't exist yet.

- [ ] **Step 4: Implement fuzzy matching functions**

Create `internal/picker/fuzzy.go`:

```go
package picker

import (
	"strings"

	"github.com/sahilm/fuzzy"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

// sessionSource implements fuzzy.Source for matching against SearchText.
type sessionSource []session.Session

func (s sessionSource) String(i int) string { return s[i].SearchText }
func (s sessionSource) Len() int            { return len(s) }

// MatchResult holds a matched session with highlight info.
type MatchResult struct {
	Session        session.Session
	Score          int
	MatchedIndexes []int // character positions in SearchText
}

// FuzzyFind performs fuzzy matching against all sessions' SearchText.
// Returns matched sessions sorted by score (best first).
// If query is empty, returns all sessions in their original order.
func FuzzyFind(query string, sessions []session.Session) []MatchResult {
	if query == "" {
		results := make([]MatchResult, len(sessions))
		for i, s := range sessions {
			results[i] = MatchResult{Session: s}
		}
		return results
	}

	matches := fuzzy.FindFrom(strings.ToLower(query), sessionSource(sessions))
	results := make([]MatchResult, len(matches))
	for i, m := range matches {
		results[i] = MatchResult{
			Session:        sessions[m.Index],
			Score:          m.Score,
			MatchedIndexes: m.MatchedIndexes,
		}
	}
	return results
}

// FuzzyMatchFirstMsg checks if the query fuzzy-matches the session's FirstMsg.
// Returns matched character indexes in the lowercased FirstMsg, or nil if no match.
func FuzzyMatchFirstMsg(query, firstMsg string) []int {
	if query == "" || firstMsg == "" {
		return nil
	}
	matches := fuzzy.Find(strings.ToLower(query), []string{strings.ToLower(firstMsg)})
	if len(matches) == 0 {
		return nil
	}
	return matches[0].MatchedIndexes
}

// ExtractSnippet extracts a context snippet from searchText around the matched characters.
// Returns the snippet string and the adjusted matched indexes within the snippet.
// searchText is expected to be lowercased (from session.SearchText).
func ExtractSnippet(searchText string, matchedIndexes []int, maxWidth int) (string, []int) {
	if len(matchedIndexes) == 0 || searchText == "" {
		return "", nil
	}

	runes := []rune(searchText)
	if len(runes) <= maxWidth {
		return searchText, matchedIndexes
	}

	// Center around first match, biased forward (1/3 before, 2/3 after)
	firstIdx := matchedIndexes[0]
	start := firstIdx - maxWidth/3
	if start < 0 {
		start = 0
	}
	end := start + maxWidth
	if end > len(runes) {
		end = len(runes)
		start = end - maxWidth
		if start < 0 {
			start = 0
		}
	}

	snippet := string(runes[start:end])

	// Map matched indexes relative to snippet start, filtering out-of-range
	var adjusted []int
	for _, idx := range matchedIndexes {
		adj := idx - start
		if adj >= 0 && adj < end-start {
			adjusted = append(adjusted, adj)
		}
	}

	prefix := ""
	if start > 0 {
		prefix = "..."
	}
	suffix := ""
	if end < len(runes) {
		suffix = "..."
	}

	// Shift adjusted indexes by prefix length
	if prefix != "" {
		prefixLen := len([]rune(prefix))
		for i := range adjusted {
			adjusted[i] += prefixLen
		}
	}

	return prefix + snippet + suffix, adjusted
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd ~/claude-sessions && go test -v ./internal/picker/
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
cd ~/claude-sessions && git add internal/picker/fuzzy.go internal/picker/fuzzy_test.go go.mod go.sum
git commit -m "feat(picker): add fuzzy matching core with tests"
```

---

### Task 2: Row Formatting and Styles

**Files:**
- Create: `internal/picker/picker.go` (styles + formatting functions only, no Model yet)
- Create: `internal/picker/picker_test.go`

- [ ] **Step 1: Write failing tests for FormatRow and highlightByIndexes**

Create `internal/picker/picker_test.go`:

```go
package picker

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func TestFormatRowClaude(t *testing.T) {
	sess := session.Session{
		ID:          "abc12345",
		FirstMsg:    "install fzf and ripgrep",
		ProjectPath: "/Users/test/my-project",
		LastTime:    time.Now().Add(-3 * time.Hour),
		Source:      session.SourceClaude,
		SearchText:  "install fzf and ripgrep",
	}
	match := MatchResult{Session: sess, MatchedIndexes: []int{8, 9, 10}}
	row := FormatRow(match, "fzf", 80, false)
	if row == "" {
		t.Fatal("expected non-empty row")
	}
	plain := stripAnsi(row)
	if !strings.Contains(plain, "C") {
		t.Error("expected Claude badge 'C' in row")
	}
	if !strings.Contains(plain, "my-project") {
		t.Error("expected project name in row")
	}
	if !strings.Contains(plain, "install fzf") {
		t.Error("expected FirstMsg in row")
	}
}

func TestFormatRowCodex(t *testing.T) {
	sess := session.Session{
		ID:          "xyz12345",
		FirstMsg:    "fix the login bug",
		ProjectPath: "/Users/test/api-server",
		LastTime:    time.Now().Add(-24 * time.Hour),
		Source:      session.SourceCodex,
		SearchText:  "fix the login bug",
	}
	match := MatchResult{Session: sess}
	row := FormatRow(match, "", 80, false)
	plain := stripAnsi(row)
	if !strings.Contains(plain, "X") {
		t.Error("expected Codex badge 'X' in row")
	}
}

func TestFormatRowSelected(t *testing.T) {
	sess := session.Session{
		ID:          "abc12345",
		FirstMsg:    "hello world",
		ProjectPath: "/Users/test/proj",
		LastTime:    time.Now(),
		Source:      session.SourceClaude,
		SearchText:  "hello world",
	}
	match := MatchResult{Session: sess}
	selected := FormatRow(match, "", 80, true)
	notSelected := FormatRow(match, "", 80, false)
	if selected == notSelected {
		t.Error("selected and non-selected rows should differ")
	}
	plain := stripAnsi(selected)
	if !strings.HasPrefix(plain, "> ") {
		t.Errorf("selected row should start with '> ', got: %q", plain[:10])
	}
}

func TestFormatRowContextSnippet(t *testing.T) {
	sess := session.Session{
		ID:          "abc12345",
		FirstMsg:    "setup dotfiles for new mac",
		ProjectPath: "/Users/test/dotfiles",
		LastTime:    time.Now(),
		Source:      session.SourceClaude,
		SearchText:  "setup dotfiles for new mac then later discussed installing fzf and ripgrep tools",
	}
	// "fzf" matches in SearchText at positions 57,58,59 but NOT in FirstMsg
	match := MatchResult{Session: sess, MatchedIndexes: []int{57, 58, 59}}
	row := FormatRow(match, "fzf", 120, false)
	plain := stripAnsi(row)
	// Should contain the snippet indicator since "fzf" is not in FirstMsg
	if !strings.Contains(plain, "fzf") {
		t.Errorf("expected snippet with 'fzf' in row, got: %s", plain)
	}
}

func TestHighlightByIndexes(t *testing.T) {
	base := lipgloss.NewStyle()
	hl := lipgloss.NewStyle().Bold(true)
	result := highlightByIndexes("hello", []int{1, 2}, base, hl)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	// The result should contain the original text chars
	plain := stripAnsi(result)
	if plain != "hello" {
		t.Errorf("expected plain text 'hello', got: %q", plain)
	}
}

func TestHighlightByIndexesEmpty(t *testing.T) {
	base := lipgloss.NewStyle()
	hl := lipgloss.NewStyle().Bold(true)
	result := highlightByIndexes("hello", nil, base, hl)
	plain := stripAnsi(result)
	if plain != "hello" {
		t.Errorf("expected plain text 'hello', got: %q", plain)
	}
}

// stripAnsi removes ANSI escape sequences for testing plain text content.
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ~/claude-sessions && go test -v ./internal/picker/
```

Expected: compilation error — `FormatRow`, `highlightByIndexes` not defined.

- [ ] **Step 3: Implement styles, helpers, FormatRow, and highlightByIndexes**

Create `internal/picker/picker.go` (formatting only — Model comes in Task 3):

```go
package picker

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

// adaptive returns a color that works on both light and dark backgrounds.
func adaptive(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

var (
	promptStyle      = lipgloss.NewStyle().Bold(true)
	dimStyle         = lipgloss.NewStyle().Foreground(adaptive("242", "241"))
	badgeClaudeStyle = lipgloss.NewStyle().Foreground(adaptive("27", "75")).Bold(true)
	badgeCodexStyle  = lipgloss.NewStyle().Foreground(adaptive("22", "40")).Bold(true)
	matchStyle       = lipgloss.NewStyle().Foreground(adaptive("0", "0")).Background(adaptive("178", "178")).Bold(true)
	selectedStyle    = lipgloss.NewStyle().Reverse(true).Bold(true)
	countStyle       = lipgloss.NewStyle().Foreground(adaptive("242", "241"))
)

const (
	colProject     = 15 // display width for project column
	maxSnippetWidth = 40 // max width for context snippet
)

// relativeTime formats a time as a human-readable relative string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// displayWidth returns the visual width of a string, counting CJK/fullwidth chars as 2.
func displayWidth(s string) int {
	return runewidth.StringWidth(s)
}

// truncateToWidth truncates a string to fit within maxWidth display columns.
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return "..."[:maxWidth]
	}
	w := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth-3 && i < len(s)-1 {
			return s[:i] + "..."
		}
		w += rw
	}
	return s
}

// padToWidth pads a string with spaces to reach exactly targetWidth display columns.
func padToWidth(s string, targetWidth int) string {
	w := displayWidth(s)
	if w >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-w)
}

// highlightByIndexes applies matchSty to characters at the given rune indexes,
// and baseSty to all other characters. Consecutive same-type characters are grouped.
func highlightByIndexes(text string, indexes []int, baseSty, matchSty lipgloss.Style) string {
	if len(indexes) == 0 {
		return baseSty.Render(text)
	}

	indexSet := make(map[int]bool, len(indexes))
	for _, idx := range indexes {
		indexSet[idx] = true
	}

	runes := []rune(text)
	var b strings.Builder
	inMatch := false
	start := 0

	for i := 0; i <= len(runes); i++ {
		isMatch := i < len(runes) && indexSet[i]
		if i == len(runes) || isMatch != inMatch {
			if i > start {
				segment := string(runes[start:i])
				if inMatch {
					b.WriteString(matchSty.Render(segment))
				} else {
					b.WriteString(baseSty.Render(segment))
				}
			}
			start = i
			inMatch = isMatch
		}
	}

	return b.String()
}

// FormatRow renders a single result row with badge, time, project, message, and optional snippet.
// If selected is true, the entire row is rendered in reverse video (no inline colors).
func FormatRow(match MatchResult, query string, width int, selected bool) string {
	s := match.Session

	// Build plain text parts
	badge := "C"
	if s.Source == session.SourceCodex {
		badge = "X"
	}

	timeStr := relativeTime(s.LastTime)
	timeField := padToWidth(timeStr, 10)

	projName := s.ProjectShortName()
	if displayWidth(projName) > colProject {
		projName = truncateToWidth(projName, colProject)
	}
	projField := padToWidth(projName, colProject)

	prefix := "  "
	if selected {
		prefix = "> "
	}

	// Calculate remaining width for message + snippet
	// prefix(2) + badge(1) + sp(2) + time(10) + sp(2) + project(15) + sp(2) = 34
	metaWidth := 2 + 1 + 2 + 10 + 2 + colProject + 2
	remainingWidth := width - metaWidth
	if remainingWidth < 10 {
		remainingWidth = 10
	}

	// Selected row: plain text, uniform reverse style
	if selected {
		msg := s.FirstMsg
		if displayWidth(msg) > remainingWidth {
			msg = truncateToWidth(msg, remainingWidth)
		}
		plainLine := prefix + badge + "  " + timeField + "  " + projField + "  " + msg
		return selectedStyle.Render(padToWidth(plainLine, width))
	}

	// Non-selected row: styled components
	badgeRendered := badgeClaudeStyle.Render(badge)
	if s.Source == session.SourceCodex {
		badgeRendered = badgeCodexStyle.Render(badge)
	}
	timeRendered := dimStyle.Render(timeField)
	projRendered := dimStyle.Render(projField)

	// Determine message display: highlight in FirstMsg or show snippet
	var msgDisplay string
	firstMsgIndexes := FuzzyMatchFirstMsg(query, s.FirstMsg)

	if firstMsgIndexes != nil || query == "" {
		// FirstMsg matches or no query — highlight directly in FirstMsg
		msg := s.FirstMsg
		if displayWidth(msg) > remainingWidth {
			msg = truncateToWidth(msg, remainingWidth)
		}
		if query != "" && firstMsgIndexes != nil {
			msgDisplay = highlightByIndexes(msg, firstMsgIndexes, lipgloss.NewStyle(), matchStyle)
		} else {
			msgDisplay = msg
		}
	} else {
		// FirstMsg doesn't match — show FirstMsg + context snippet
		snippetW := maxSnippetWidth
		msgW := remainingWidth - snippetW - 3 // 3 for "  " separator + "…" not needed, just spacing
		if msgW < 15 {
			msgW = 15
			snippetW = remainingWidth - msgW - 2
		}
		if snippetW < 10 {
			snippetW = 0
		}

		msg := s.FirstMsg
		if displayWidth(msg) > msgW {
			msg = truncateToWidth(msg, msgW)
		}

		if snippetW > 0 && len(match.MatchedIndexes) > 0 {
			snippet, snippetIndexes := ExtractSnippet(s.SearchText, match.MatchedIndexes, snippetW-6) // 6 for "...匹配: " overhead — simplified
			snippetDisplay := highlightByIndexes(snippet, snippetIndexes, dimStyle, matchStyle)
			msgDisplay = msg + "  " + snippetDisplay
		} else {
			msgDisplay = msg
		}
	}

	return prefix + badgeRendered + "  " + timeRendered + "  " + projRendered + "  " + msgDisplay
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ~/claude-sessions && go test -v ./internal/picker/
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add internal/picker/picker.go internal/picker/picker_test.go
git commit -m "feat(picker): add row formatting with fuzzy highlight and context snippets"
```

---

### Task 3: PickerModel (Bubble Tea Interactive Component)

**Files:**
- Modify: `internal/picker/picker.go` (append Model, NewModel, Init, Update, View, PickResult)

- [ ] **Step 1: Add the PickerModel, Result, NewModel, Init**

Append to `internal/picker/picker.go`:

```go
// Result holds the selected session info after the picker exits.
type Result struct {
	Session   *session.Session
	Cancelled bool
}

// Model is the Bubble Tea model for the inline fuzzy picker.
type Model struct {
	input      textinput.Model
	sessions   []session.Session // all sessions, sorted by LastTime
	matches    []MatchResult
	cursor     int
	listOffset int
	width      int
	height     int // half of terminal height
	result     Result
}

// NewModel creates a picker Model pre-loaded with sessions.
// Sessions are sorted by LastTime descending (most recent first).
func NewModel(sessions []session.Session) Model {
	sorted := make([]session.Session, len(sessions))
	copy(sorted, sessions)
	sort.Sort(session.SortByLastTime(sorted))

	matches := make([]MatchResult, len(sorted))
	for i, s := range sorted {
		matches[i] = MatchResult{Session: s}
	}

	ti := textinput.New()
	ti.Placeholder = "Search sessions..."
	ti.Focus()
	ti.Prompt = "> "
	ti.CharLimit = 100

	return Model{
		input:    ti,
		sessions: sorted,
		matches:  matches,
		result:   Result{Cancelled: true},
	}
}

// PickResult returns the result after the program exits.
func (m Model) PickResult() Result {
	return m.result
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}
```

Add these imports to the file's import block:

```go
	"sort"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
```

- [ ] **Step 2: Implement Update**

Append to `internal/picker/picker.go`:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height / 2
		if m.height < 3 {
			m.height = 3
		}
		m.input.Width = msg.Width - 20 // leave room for count display
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if len(m.matches) > 0 && m.cursor < len(m.matches) {
				s := m.matches[m.cursor].Session
				m.result = Result{Session: &s}
			}
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.result = Result{Cancelled: true}
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.listOffset {
					m.listOffset = m.cursor
				}
			}
			return m, nil
		case "down", "ctrl+n":
			maxVisible := m.height - 1
			if m.cursor < len(m.matches)-1 {
				m.cursor++
				if m.cursor >= m.listOffset+maxVisible {
					m.listOffset = m.cursor - maxVisible + 1
				}
			}
			return m, nil
		}
	}

	// Forward to text input
	prevQuery := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	newQuery := m.input.Value()

	// Re-filter if query changed
	if newQuery != prevQuery {
		if newQuery == "" {
			m.matches = make([]MatchResult, len(m.sessions))
			for i, s := range m.sessions {
				m.matches[i] = MatchResult{Session: s}
			}
		} else {
			m.matches = FuzzyFind(newQuery, m.sessions)
		}
		m.cursor = 0
		m.listOffset = 0
	}

	return m, cmd
}
```

- [ ] **Step 3: Implement View**

Append to `internal/picker/picker.go`:

```go
func (m Model) View() string {
	if m.width == 0 {
		return "" // not yet sized
	}

	var lines []string

	// Line 1: input + match count (right-aligned)
	inputView := m.input.View()
	count := countStyle.Render(fmt.Sprintf("%d/%d", len(m.matches), len(m.sessions)))
	inputW := lipgloss.Width(inputView)
	countW := lipgloss.Width(count)
	gap := m.width - inputW - countW
	if gap < 1 {
		gap = 1
	}
	lines = append(lines, inputView+strings.Repeat(" ", gap)+count)

	// Result rows
	maxVisible := m.height - 1
	if maxVisible < 1 {
		maxVisible = 1
	}

	query := m.input.Value()
	for i := 0; i < maxVisible; i++ {
		idx := m.listOffset + i
		if idx < len(m.matches) {
			lines = append(lines, FormatRow(m.matches[idx], query, m.width, idx == m.cursor))
		} else {
			lines = append(lines, strings.Repeat(" ", m.width))
		}
	}

	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Verify compilation**

```bash
cd ~/claude-sessions && go build ./internal/picker/
```

Expected: compiles without error.

- [ ] **Step 5: Run all tests**

```bash
cd ~/claude-sessions && go test ./...
```

Expected: all tests PASS (existing + new picker tests).

- [ ] **Step 6: Commit**

```bash
cd ~/claude-sessions && git add internal/picker/picker.go
git commit -m "feat(picker): add Bubble Tea PickerModel with inline fuzzy search UI"
```

---

### Task 4: Add `pick` Subcommand to main.go

**Files:**
- Modify: `cmd/claude-sessions/main.go:26-41` (add `pick` subcommand before existing TUI launch)

- [ ] **Step 1: Add the pick subcommand**

In `cmd/claude-sessions/main.go`, add a new import:

```go
	"github.com/jinguo998/claude-sessions/internal/picker"
	"github.com/jinguo998/claude-sessions/internal/scanner"
```

After the `init` subcommand block (after line 41: `return`), add a new block before the existing TUI code:

```go
	// Handle "pick" subcommand — fzf-style fuzzy picker
	if len(os.Args) > 1 && os.Args[1] == "pick" {
		sessions, err := scanner.ScanAllSessions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Fprintln(os.Stderr, "no sessions found")
			return
		}

		m := picker.NewModel(sessions)
		p := tea.NewProgram(m, tea.WithMouseCellMotion())

		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		result := finalModel.(picker.Model).PickResult()
		if result.Cancelled || result.Session == nil {
			return
		}

		sess := *result.Session
		fmt.Fprintf(os.Stderr, "\033[2m→ cd %s\033[0m\n", sess.ProjectPath)
		if err := os.Chdir(sess.ProjectPath); err != nil {
			fmt.Fprintf(os.Stderr, "cd failed: %v\n", err)
			os.Exit(1)
		}
		os.WriteFile("/tmp/claude-sessions-cd", []byte(sess.ProjectPath), 0644)

		shortID := sess.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		var binName string
		var args []string
		if sess.Source == session.SourceCodex {
			binName = "codex"
			args = []string{"codex", "resume", sess.ID}
			fmt.Fprintf(os.Stderr, "\033[2m→ Resuming Codex session %s...\033[0m\n", shortID)
		} else {
			binName = "claude"
			args = []string{"claude", "--resume", sess.ID, "--dangerously-skip-permissions"}
			fmt.Fprintf(os.Stderr, "\033[2m→ Resuming session %s...\033[0m\n", shortID)
		}

		binPath, err := exec.LookPath(binName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s not found in PATH: %v\n", binName, err)
			os.Exit(1)
		}
		if err := syscall.Exec(binPath, args, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "exec failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
```

Also add `scanner` to the imports:

```go
	"github.com/jinguo998/claude-sessions/internal/scanner"
```

- [ ] **Step 2: Verify compilation**

```bash
cd ~/claude-sessions && go build -o claude-sessions ./cmd/claude-sessions/
```

Expected: compiles without error.

- [ ] **Step 3: Run all tests**

```bash
cd ~/claude-sessions && go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Manual test**

```bash
cd ~/claude-sessions && ./claude-sessions pick
```

Expected: inline picker appears in bottom half of terminal. Type to filter. `↑/↓` to navigate. `Esc` to cancel.

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add cmd/claude-sessions/main.go
git commit -m "feat: add 'pick' subcommand for fzf-style fuzzy session picker"
```

---

### Task 5: Shell Init with Fuzzy Picker Keybinding

**Files:**
- Modify: `cmd/claude-sessions/main.go:16-24` (replace `shellInit` with zsh/bash variants)

- [ ] **Step 1: Replace shellInit with zsh and bash versions**

In `cmd/claude-sessions/main.go`, replace the existing `shellInit` constant (lines 16-24):

```go
// shellInit works for both zsh and bash.
const shellInit = `# claude-sessions shell integration
cs() {
    command claude-sessions "$@"
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}
`
```

With two separate constants:

```go
const zshInit = `# claude-sessions shell integration
cs() {
    command claude-sessions "$@"
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}

# Fuzzy session picker (ctrl+s)
_claude_sessions_pick() {
    claude-sessions pick
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
    zle reset-prompt
}
zle -N _claude_sessions_pick
bindkey '^s' _claude_sessions_pick
`

const bashInit = `# claude-sessions shell integration
cs() {
    command claude-sessions "$@"
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}

# Fuzzy session picker (ctrl+s)
_claude_sessions_pick() {
    claude-sessions pick
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}
bind -x '"\C-s": _claude_sessions_pick'
`
```

- [ ] **Step 2: Update the init subcommand handler**

Replace the init switch case (around lines 33-38):

```go
		switch shell {
		case "zsh", "bash":
			fmt.Print(shellInit)
```

With:

```go
		switch shell {
		case "zsh":
			fmt.Print(zshInit)
		case "bash":
			fmt.Print(bashInit)
```

- [ ] **Step 3: Run tests**

```bash
cd ~/claude-sessions && go test ./...
```

Expected: all tests PASS. If `main_test.go` has tests for the init output, they may need updating — check and fix if needed.

- [ ] **Step 4: Verify init output**

```bash
cd ~/claude-sessions && go run ./cmd/claude-sessions/ init zsh
```

Expected: outputs zsh version with `cs()` function AND `_claude_sessions_pick` widget + `bindkey '^s'`.

```bash
cd ~/claude-sessions && go run ./cmd/claude-sessions/ init bash
```

Expected: outputs bash version with `cs()` function AND `_claude_sessions_pick` function + `bind -x`.

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add cmd/claude-sessions/main.go
git commit -m "feat: add ctrl+s shell keybinding for fuzzy picker in zsh/bash init"
```
