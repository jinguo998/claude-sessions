package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

// previewSearch manages in-preview search state.
type previewSearch struct {
	active  bool            // search mode is on (input visible)
	input   textinput.Model // search text input
	query   string          // confirmed search query (after Enter)
	hits    []int           // line indices of matches (in the raw content lines)
	current int             // current hit index
	content string          // original styled content (as set on the viewport)
	lines   []string        // content split by newline (cached)
}

// newPreviewSearch creates an initialized previewSearch.
func newPreviewSearch() previewSearch {
	ti := textinput.New()
	ti.Placeholder = "Search in preview..."
	ti.CharLimit = 200
	return previewSearch{input: ti}
}

// Open activates the search bar and focuses the input.
func (ps *previewSearch) Open() {
	ps.active = true
	ps.input.Focus()
	ps.input.SetValue("")
}

// Close deactivates the search bar and clears query/hits.
func (ps *previewSearch) Close() {
	ps.active = false
	ps.input.Blur()
	ps.query = ""
	ps.hits = nil
	ps.current = 0
}

// SetContent stores the rendered content and splits it into lines.
// This should be called whenever the preview viewport content is set.
func (ps *previewSearch) SetContent(content string) {
	ps.content = content
	ps.lines = strings.Split(content, "\n")
	// Clear previous search results since content changed
	ps.hits = nil
	ps.current = 0
	ps.query = ""
}

// Execute runs the search: finds all line indices containing the query
// (case-insensitive), sets hits, and resets current to 0.
func (ps *previewSearch) Execute() {
	ps.query = ps.input.Value()
	ps.hits = nil
	ps.current = 0

	if ps.query == "" {
		return
	}

	q := strings.ToLower(ps.query)
	for i, line := range ps.lines {
		if strings.Contains(strings.ToLower(line), q) {
			ps.hits = append(ps.hits, i)
		}
	}
}

// Next advances to the next hit (wrapping around) and returns the target
// line index. Returns -1 if there are no hits.
func (ps *previewSearch) Next() int {
	if len(ps.hits) == 0 {
		return -1
	}
	ps.current = (ps.current + 1) % len(ps.hits)
	return ps.hits[ps.current]
}

// Prev goes to the previous hit (wrapping around) and returns the target
// line index. Returns -1 if there are no hits.
func (ps *previewSearch) Prev() int {
	if len(ps.hits) == 0 {
		return -1
	}
	ps.current = (ps.current - 1 + len(ps.hits)) % len(ps.hits)
	return ps.hits[ps.current]
}

// HighlightContent re-renders the stored content with all occurrences of
// the current query highlighted using the highlight style. It performs
// case-insensitive replacement on the already-styled content.
func (ps *previewSearch) HighlightContent() string {
	if ps.query == "" || ps.content == "" {
		return ps.content
	}

	q := ps.query
	lower := strings.ToLower(ps.content)
	qLower := strings.ToLower(q)

	var b strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], qLower)
		if idx < 0 {
			b.WriteString(ps.content[pos:])
			break
		}
		// Write everything before the match
		b.WriteString(ps.content[pos : pos+idx])
		// Write the highlighted match (preserving original case)
		matched := ps.content[pos+idx : pos+idx+len(q)]
		b.WriteString(highlightStyle.Render(matched))
		pos += idx + len(q)
	}
	return b.String()
}

// StatusText returns a string like "3/12" showing the current match position
// and total matches, or "" if there is no active query.
func (ps *previewSearch) StatusText() string {
	if ps.query == "" {
		return ""
	}
	if len(ps.hits) == 0 {
		return "0/0"
	}
	return strconv.Itoa(ps.current+1) + "/" + strconv.Itoa(len(ps.hits))
}
