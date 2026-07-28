package picker

import (
	"strings"
	"testing"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func TestFindSessionsSingleToken(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", FirstMsg: "install fzf and ripgrep", SearchText: "install fzf and ripgrep"},
		{ID: "2", FirstMsg: "fix the login bug", SearchText: "fix the login bug"},
		{ID: "3", FirstMsg: "add rate limiting", SearchText: "add rate limiting also discussed fzf integration"},
	}

	results := FindSessions("fzf", sessions)
	if len(results) != 2 {
		t.Fatalf("expected 2 matches for 'fzf', got %d", len(results))
	}
	ids := map[string]bool{results[0].Session.ID: true, results[1].Session.ID: true}
	if !ids["1"] || !ids["3"] {
		t.Errorf("expected sessions 1 and 3, got %v", ids)
	}
}

func TestFindSessionsMultiToken(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", SearchText: "add codex session support"},
		{ID: "2", SearchText: "fix login bug in codex"},
		{ID: "3", SearchText: "add rate limiting"},
	}

	results := FindSessions("codex add", sessions)
	if len(results) != 1 {
		t.Fatalf("expected 1 match for 'codex add', got %d", len(results))
	}
	if results[0].Session.ID != "1" {
		t.Errorf("expected session 1, got %s", results[0].Session.ID)
	}
}

func TestFindSessionsEmptyQuery(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", FirstMsg: "hello"},
		{ID: "2", FirstMsg: "world"},
	}
	results := FindSessions("", sessions)
	if len(results) != 2 {
		t.Fatalf("expected all %d sessions for empty query, got %d", len(sessions), len(results))
	}
}

func TestFindSessionsNoMatch(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", SearchText: "hello world"},
	}
	results := FindSessions("zzzzz", sessions)
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestFindSessionsCaseInsensitive(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", SearchText: "install codex cli"},
	}
	results := FindSessions("Codex", sessions)
	if len(results) != 1 {
		t.Fatalf("expected 1 match for 'Codex', got %d", len(results))
	}
}

func TestFindSessionsMatchedIndexes(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", SearchText: "hello fzf world"},
	}
	results := FindSessions("fzf", sessions)
	if len(results) != 1 {
		t.Fatal("expected 1 match")
	}
	idxs := results[0].MatchedIndexes
	if len(idxs) != 3 || idxs[0] != 6 || idxs[1] != 7 || idxs[2] != 8 {
		t.Errorf("expected indexes [6,7,8], got %v", idxs)
	}
}

func TestHighlightIndexes(t *testing.T) {
	indexes := HighlightIndexes([]string{"fzf"}, "install fzf and ripgrep")
	if len(indexes) != 3 {
		t.Fatalf("expected 3 highlight indexes, got %d", len(indexes))
	}
	if indexes[0] != 8 || indexes[1] != 9 || indexes[2] != 10 {
		t.Errorf("expected indexes [8,9,10], got %v", indexes)
	}
}

func TestHighlightIndexesNoMatch(t *testing.T) {
	indexes := HighlightIndexes([]string{"xyz"}, "hello world")
	if len(indexes) != 0 {
		t.Fatalf("expected no indexes, got %v", indexes)
	}
}

func TestHighlightIndexesMultiToken(t *testing.T) {
	indexes := HighlightIndexes([]string{"hello", "world"}, "hello beautiful world")
	if len(indexes) != 10 {
		t.Fatalf("expected 10 indexes, got %d: %v", len(indexes), indexes)
	}
}

func TestExtractSnippet(t *testing.T) {
	text := "this is a long search text with fzf mentioned somewhere in the middle of it all"
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
		t.Errorf("expected unchanged indexes, got: %v", adjusted)
	}
}

func TestExtractSnippetEmpty(t *testing.T) {
	snippet, adjusted := ExtractSnippet("", []int{0}, 30)
	if snippet != "" || adjusted != nil {
		t.Errorf("expected empty for empty text")
	}
	snippet2, adjusted2 := ExtractSnippet("hello", nil, 30)
	if snippet2 != "" || adjusted2 != nil {
		t.Errorf("expected empty for nil indexes")
	}
}

func TestExtractSnippetEllipsis(t *testing.T) {
	text := "aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll"
	indexes := []int{30, 31, 32, 33}
	snippet, _ := ExtractSnippet(text, indexes, 20)
	if !strings.HasPrefix(snippet, "...") {
		t.Errorf("expected leading ellipsis, got: %s", snippet)
	}
	if !strings.HasSuffix(snippet, "...") {
		t.Errorf("expected trailing ellipsis, got: %s", snippet)
	}
}
