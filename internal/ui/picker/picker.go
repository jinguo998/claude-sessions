package picker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func adaptive(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

var (
	dimStyle      = plainStyle.Foreground(adaptive("242", "241"))
	matchStyle    = plainStyle.Reverse(true).Bold(true)
	plainStyle    = lipgloss.NewStyle()
	selectedStyle = plainStyle.Foreground(adaptive("255", "255")).Background(adaptive("57", "57"))
	countStyle    = plainStyle.Foreground(adaptive("242", "241"))
)

const (
	colProject      = 15
	maxSnippetWidth = 40
)

func relativeTime(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func displayWidth(s string) int {
	return runewidth.StringWidth(s)
}

func truncateToWidth(s string, maxWidth int) string {
	if displayWidth(s) <= maxWidth {
		return s
	}
	ellipsis := "..."
	budget := maxWidth - displayWidth(ellipsis)
	if budget <= 0 {
		return ellipsis[:maxWidth]
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > budget {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + ellipsis
}

func padToWidth(s string, targetWidth int) string {
	w := displayWidth(s)
	if w >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-w)
}

func highlightByIndexes(text string, indexes []int, baseSty, matchSty lipgloss.Style) string {
	if text == "" {
		return ""
	}

	matchSet := make(map[int]bool, len(indexes))
	for _, idx := range indexes {
		matchSet[idx] = true
	}

	runes := []rune(text)
	var b strings.Builder

	i := 0
	for i < len(runes) {
		isMatch := matchSet[i]
		j := i + 1
		for j < len(runes) && matchSet[j] == isMatch {
			j++
		}
		chunk := string(runes[i:j])
		if isMatch {
			b.WriteString(matchSty.Render(chunk))
		} else {
			b.WriteString(baseSty.Render(chunk))
		}
		i = j
	}

	return b.String()
}

// FormatRow formats a single picker row. tokens are the pre-lowercased query
// words (split once by the caller, not per row).
func FormatRow(match MatchResult, tokens []string, width int, selected bool, sources ...session.SourceRegistry) string {
	s := match.Session
	registry := session.SourceRegistry{}
	if len(sources) > 0 {
		registry = sources[0]
	}
	info := registry.Info(s.Source)

	// --- badge ---
	badgeChar := info.Badge
	badgeSty := plainStyle.Foreground(adaptive(info.LightColor, info.DarkColor)).Bold(true)

	// --- time (10 cols) ---
	timeStr := padToWidth(truncateToWidth(relativeTime(s.LastTime), 10), 10)

	// --- project (colProject cols) ---
	projStr := padToWidth(truncateToWidth(s.ProjectShortName(), colProject), colProject)

	// --- message part ---
	// Fixed overhead: prefix(2) + badge(1) + sp(2) + time(10) + sp(2) + project(15) + sp(2)
	const overhead = 2 + 1 + 2 + 10 + 2 + colProject + 2
	msgWidth := max(width-overhead, 0)

	firstMsgTrunc := truncateToWidth(s.FirstMsg, msgWidth)

	// Selected row: full-width background, no keyword highlights (like fzf).
	if selected {
		plain := "> " + badgeChar + "  " + timeStr + "  " + projStr + "  " + firstMsgTrunc
		return selectedStyle.Render(padToWidth(plain, width))
	}

	var msgPart string
	if len(tokens) == 0 {
		msgPart = firstMsgTrunc
	} else {
		firstMsgIndexes := HighlightIndexes(tokens, firstMsgTrunc)
		if len(firstMsgIndexes) > 0 {
			msgPart = highlightByIndexes(firstMsgTrunc, firstMsgIndexes, plainStyle, matchStyle)
		} else {
			// Tokens not in FirstMsg; show FirstMsg + context snippet.
			firstMsgWidth := displayWidth(firstMsgTrunc)
			remaining := msgWidth - firstMsgWidth - 2
			searchText := match.SearchText
			if searchText == "" {
				searchText = s.SearchText
			}
			if remaining > 10 && len(match.MatchedIndexes) > 0 && searchText != "" {
				snippet, snippetIdxs := ExtractSnippet(searchText, match.MatchedIndexes, min(remaining, maxSnippetWidth))
				snippetHighlighted := highlightByIndexes(snippet, snippetIdxs, dimStyle, matchStyle)
				msgPart = firstMsgTrunc + "  " + snippetHighlighted
			} else {
				msgPart = firstMsgTrunc
			}
		}
	}

	return "  " +
		badgeSty.Render(badgeChar) +
		"  " +
		dimStyle.Render(timeStr) +
		"  " +
		dimStyle.Render(projStr) +
		"  " +
		msgPart
}

// Result is returned by the picker when the user selects or cancels.
type Result struct {
	Session   *session.Session
	Cancelled bool
}

// Model is the Bubble Tea model for the inline session picker.
type Model struct {
	input      textinput.Model
	sessions   []session.Session // all sessions, sorted by LastTime descending
	matches    []MatchResult
	cursor     int
	listOffset int
	width      int
	height     int // half of terminal height (picker area)
	fullHeight int // full terminal height (for mouse offset calculation)
	result     Result
	quitting   bool
	sources    session.SourceRegistry
}

func NewModel(sessions []session.Session, sources ...session.SourceRegistry) Model {
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
	registry := session.SourceRegistry{}
	if len(sources) > 0 {
		registry = sources[0]
	}

	return Model{
		input:    ti,
		sessions: sorted,
		matches:  matches,
		result:   Result{Cancelled: true},
		sources:  registry,
	}
}

// PickResult returns the result from the most recent picker interaction.
func (m Model) PickResult() Result {
	return m.result
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.fullHeight = msg.Height
		m.height = max(msg.Height/2, 3)
		m.input.Width = msg.Width - 20

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if len(m.matches) > 0 {
				s := m.matches[m.cursor].Session
				m.result = Result{Session: &s, Cancelled: false}
			}
			m.quitting = true
			return m, tea.Quit

		case "esc", "ctrl+c":
			m.result = Result{Cancelled: true}
			m.quitting = true
			return m, tea.Quit

		// fzf-style: up moves selection away from input (higher index),
		// down moves toward input (lower index).
		case "up", "ctrl+p":
			if m.cursor < len(m.matches)-1 {
				m.cursor++
				maxVisible := m.height - 1
				if m.cursor >= m.listOffset+maxVisible {
					m.listOffset = m.cursor - maxVisible + 1
				}
			}

		case "down", "ctrl+n":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.listOffset {
					m.listOffset = m.cursor
				}
			}

		default:
			// Forward key to text input and re-filter if query changed.
			prevQuery := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			newQuery := m.input.Value()
			if newQuery != prevQuery {
				if newQuery == "" {
					m.matches = make([]MatchResult, len(m.sessions))
					for i, s := range m.sessions {
						m.matches[i] = MatchResult{Session: s}
					}
				} else {
					m.matches = FindSessions(newQuery, m.sessions)
				}
				m.cursor = 0
				m.listOffset = 0
			}
			return m, cmd
		}

	case tea.MouseMsg:
		maxVisible := m.height - 1
		// fzf layout: rows are rendered reversed. The last result row
		// (closest to input) is visual line maxVisible-1 from top (0-indexed).
		// Visual line i corresponds to match index: listOffset + (maxVisible - 1 - i).
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			// Scroll up = show higher-indexed matches (further from input)
			if m.cursor < len(m.matches)-1 {
				m.cursor++
				if m.cursor >= m.listOffset+maxVisible {
					m.listOffset = m.cursor - maxVisible + 1
				}
			}
		case tea.MouseButtonWheelDown:
			// Scroll down = show lower-indexed matches (closer to input)
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.listOffset {
					m.listOffset = m.cursor
				}
			}
		case tea.MouseButtonLeft:
			// In inline mode, the picker renders at the bottom of the terminal.
			// Count from bottom: row 0 = input line, row 1 = first result, etc.
			rowFromBottom := m.fullHeight - 1 - msg.Y
			if rowFromBottom >= 1 && rowFromBottom <= maxVisible {
				idx := m.listOffset + (rowFromBottom - 1)
				if idx >= 0 && idx < len(m.matches) {
					if idx == m.cursor {
						s := m.matches[m.cursor].Session
						m.result = Result{Session: &s, Cancelled: false}
						m.quitting = true
						return m, tea.Quit
					}
					m.cursor = idx
				}
			}
		}
		return m, nil

	default:
		// Forward other messages to the text input.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting || m.width == 0 {
		return ""
	}

	maxVisible := m.height - 1
	query := m.input.Value()
	tokens := strings.Fields(strings.ToLower(query))

	lines := make([]string, 0, maxVisible+1)

	for i := maxVisible - 1; i >= 0; i-- {
		idx := m.listOffset + i
		if idx < len(m.matches) {
			row := FormatRow(m.matches[idx], tokens, m.width, idx == m.cursor, m.sources)
			lines = append(lines, row)
		} else {
			lines = append(lines, "")
		}
	}

	// Bottom line: input + right-aligned count.
	inputView := m.input.View()
	countStr := fmt.Sprintf("%d/%d", len(m.matches), len(m.sessions))
	countRendered := countStyle.Render(countStr)
	inputW := lipgloss.Width(inputView)
	countW := lipgloss.Width(countRendered)
	padding := max(m.width-inputW-countW, 1)
	lines = append(lines, inputView+strings.Repeat(" ", padding)+countRendered)

	return strings.Join(lines, "\n")
}
