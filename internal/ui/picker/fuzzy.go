package picker

import (
	"strings"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
	appquery "github.com/jinguo998/claude-sessions/internal/app/query"
)

// MatchResult holds a matched session with the rune positions of matched
// characters in SearchText (used for snippet extraction).
type MatchResult struct {
	Session        session.Session
	SearchText     string
	MatchedIndexes []int
}

// FindSessions performs multi-token substring matching on sessions.
// The query is split by spaces; ALL tokens must appear as substrings in
// SearchText (AND logic). Empty query returns all sessions.
func FindSessions(query string, sessions []session.Session) []MatchResult {
	tokens := appquery.Tokens(query)
	matches := appquery.NewService().Search(sessions, appquery.Filter{Query: query})
	if len(tokens) == 0 {
		results := make([]MatchResult, len(sessions))
		for i, s := range sessions {
			results[i] = MatchResult{Session: s}
		}
		return results
	}

	results := make([]MatchResult, 0, len(matches))
	for _, match := range matches {
		results = append(results, MatchResult{
			Session:        match.Session,
			SearchText:     match.SearchText,
			MatchedIndexes: appquery.MatchedRuneIndexes(tokens, match.SearchText),
		})
	}
	return results
}

// HighlightIndexes finds tokens in text (case-insensitive) and returns the
// rune indexes of matched characters for display highlighting.
func HighlightIndexes(tokens []string, text string) []int {
	if len(tokens) == 0 || text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	return appquery.MatchedRuneIndexes(tokens, lower)
}

// ExtractSnippet extracts a context snippet from searchText around the first
// matched character. Returns the snippet and adjusted rune indexes within it.
func ExtractSnippet(searchText string, matchedIndexes []int, maxWidth int) (string, []int) {
	if len(matchedIndexes) == 0 || searchText == "" {
		return "", nil
	}

	runes := []rune(searchText)
	if len(runes) <= maxWidth {
		return searchText, matchedIndexes
	}

	firstIdx := matchedIndexes[0]
	start := max(firstIdx-maxWidth/3, 0)
	end := start + maxWidth
	if end > len(runes) {
		end = len(runes)
		start = max(end-maxWidth, 0)
	}

	snippet := string(runes[start:end])

	var adjusted []int
	for _, idx := range matchedIndexes {
		adj := idx - start
		if adj >= 0 && adj < end-start {
			adjusted = append(adjusted, adj)
		}
	}

	if start > 0 {
		for i := range adjusted {
			adjusted[i] += 3 // len("...")
		}
		snippet = "..." + snippet
	}
	if end < len(runes) {
		snippet += "..."
	}

	return snippet, adjusted
}
