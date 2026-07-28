package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
	appquery "github.com/jinguo998/claude-sessions/internal/app/query"
)

// sortMode controls the ordering of sessions in the list.
type sortMode int

const (
	sortRecent sortMode = iota
	sortProject
	sortMsgCount
	sortToolCount
	sortTokens
	sortModeCount // sentinel for cycling
)

func (s sortMode) String() string {
	switch s {
	case sortRecent:
		return "Recent"
	case sortProject:
		return "Project"
	case sortMsgCount:
		return "Messages"
	case sortToolCount:
		return "Tools"
	case sortTokens:
		return "Tokens"
	default:
		return "Recent"
	}
}

type sourceFilter int

const (
	sourceAll sourceFilter = 0
)

// Raw ANSI escape codes for row styling. Using raw codes instead of
// lipgloss Render so they don't emit \033[0m resets that kill row backgrounds.
const (
	ansiReset   = "\033[0m"
	ansiFgReset = "\033[22;39m" // un-bold + default fg (keeps bg intact)
)

// ansiSelectedBg returns the selected row style, adapting to terminal theme.
func ansiSelectedBg() string {
	return "\033[1;38;5;255;48;5;57m" // bold white on the shared purple accent
}

func ansiBadgeSource(info session.SourceInfo) string {
	color := info.LightColor
	if lipgloss.HasDarkBackground() {
		color = info.DarkColor
	}
	return fmt.Sprintf("\033[1;38;5;%sm", color)
}

func sourceBadgeStyle(info session.SourceInfo) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: info.LightColor, Dark: info.DarkColor}).
		Bold(true)
}

const (
	colWidthProject      = 20                     // display width for project column in full list
	colWidthProjectSplit = 16                     // display width for project column in split view
	colWidthMsg          = 42                     // display width for message column in full list
	splitMsgOffset       = 40                     // badge(1) + time(11) + proj(20) + spacing(8)
	maxLastMsgWidth      = 80                     // max width for last-message detail line
	maxSnippetWidth      = 40                     // max width for search match snippet
	doubleClickThreshold = 400 * time.Millisecond // max interval between clicks for double-click
	scrollStep           = 5                      // lines to scroll per wheel tick
	listPaddingBase      = 5                      // title(1) + blank(1) + blank-before-help(1) + help(1) + legend(1)
	searchCharLimit      = 100                    // max characters in search input
)

// ListModel is an independent Bubble Tea sub-component for the session list,
// including search, sort, filter, mouse and keyboard navigation.
type ListModel struct {
	sessions      []session.Session
	filtered      []session.Session
	cursor        int
	listOffset    int
	searchInput   textinput.Model
	searching     bool
	searchQuery   string
	sortMode      sortMode
	filterProj    string // "" means all
	sourceFilter  sourceFilter
	sources       session.SourceRegistry
	width         int
	height        int
	lastClickTime time.Time
	lastClickIdx  int
	loaded        bool
	compact       bool // true when in split/compact mode
}

// NewListModel creates an initialised ListModel.
func NewListModel(sources ...session.SourceRegistry) ListModel {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = searchCharLimit
	registry := session.SourceRegistry{}
	if len(sources) > 0 {
		registry = sources[0]
	}
	return ListModel{
		searchInput: ti,
		sources:     registry,
	}
}

// SetSessions replaces the session data and re-applies filter/sort.
func (l ListModel) SetSessions(sessions []session.Session) ListModel {
	l.sessions = sessions
	l.loaded = true
	l.applyFilter()
	return l
}

// SetSize updates the stored width/height.
func (l ListModel) SetSize(w, h int) ListModel {
	l.width = w
	l.height = h
	return l
}

// SetCompact sets whether the list is in compact/split mode (affects scroll calculations).
func (l ListModel) SetCompact(compact bool) ListModel {
	l.compact = compact
	return l
}

// SelectedSession returns the currently highlighted session, if any.
func (l ListModel) SelectedSession() (session.Session, bool) {
	if l.cursor >= 0 && l.cursor < len(l.filtered) {
		return l.filtered[l.cursor], true
	}
	return session.Session{}, false
}

// Cursor returns the current cursor index.
func (l ListModel) Cursor() int {
	return l.cursor
}

// Filtered returns the current filtered session list.
func (l ListModel) Filtered() []session.Session {
	return l.filtered
}

// Update handles key and mouse messages for the list.
func (l ListModel) Update(msg tea.Msg) (ListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if l.searching {
			return l.updateSearching(msg)
		}
		return l.updateList(msg)
	case tea.MouseMsg:
		return l.handleMouse(msg)
	}
	return l, nil
}

func (l ListModel) updateSearching(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		l.searching = false
		l.searchQuery = ""
		l.searchInput.SetValue("")
		l.applyFilter()
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{} }
	case "enter":
		l.searching = false
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg { return SessionPreviewMsg{Session: sess} }
		}
		return l, nil
	case "up", "ctrl+k":
		if l.cursor > 0 {
			l.cursor--
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	case "down", "ctrl+j":
		if l.cursor < len(l.filtered)-1 {
			l.cursor++
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	default:
		prevQuery := l.searchQuery
		var cmd tea.Cmd
		l.searchInput, cmd = l.searchInput.Update(msg)
		l.searchQuery = l.searchInput.Value()
		l.applyFilter()
		l.updateListOffset()
		if l.searchQuery != prevQuery {
			return l, tea.Batch(cmd, func() tea.Msg { return FilterChangedMsg{Debounce: true} })
		}
		return l, cmd
	}
}

func (l ListModel) updateList(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch msg.String() {
	case "q":
		return l, tea.Quit
	case "up", "k":
		if l.cursor > 0 {
			l.cursor--
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	case "down", "j":
		if l.cursor < len(l.filtered)-1 {
			l.cursor++
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	case "enter", "p":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg { return SessionPreviewMsg{Session: sess} }
		}
		return l, nil
	case "r":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg { return defaultResumeSelection(l.sources, sess) }
		}
		return l, nil
	case "R":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg { return safeResumeSelection(sess) }
		}
		return l, nil
	case "n":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg { return forkSelection(l.sources, sess) }
		}
		return l, nil
	case "d":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			if !l.sources.SupportsArchive(sess.Source) {
				return l, func() tea.Msg { return SessionArchiveUnsupportedMsg{Session: sess} }
			}
			idx := l.cursor
			return l, func() tea.Msg { return SessionDeleteMsg{Index: idx} }
		}
		return l, nil
	case "/":
		l.searching = true
		l.searchInput.Focus()
		return l, textinput.Blink
	case "s":
		l.sortMode = (l.sortMode + 1) % sortModeCount
		l.applySort()
		return l, nil
	case "f":
		return l, func() tea.Msg { return OpenProjectPickerMsg{} }
	case "F":
		l.sourceFilter = (l.sourceFilter + 1) % sourceFilter(len(l.sources.All())+1)
		l.applyFilter()
		return l, func() tea.Msg { return FilterChangedMsg{} }
	case "m":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg {
				return OpenContextMenuMsg{Session: sess, X: 4, Y: l.cursor - l.listOffset + 3}
			}
		}
		return l, nil
	case "G":
		if len(l.filtered) > 0 {
			l.cursor = len(l.filtered) - 1
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	case "g":
		l.cursor = 0
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	case "ctrl+d":
		page := l.visibleRows() / 2
		l.cursor += page
		if l.cursor >= len(l.filtered) {
			l.cursor = len(l.filtered) - 1
		}
		if l.cursor < 0 {
			l.cursor = 0
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	case "ctrl+u":
		page := l.visibleRows() / 2
		l.cursor -= page
		if l.cursor < 0 {
			l.cursor = 0
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	case "y":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg { return CopyIDMsg{ID: sess.ID} }
		}
		return l, nil
	case "O":
		if len(l.filtered) > 0 {
			sess := l.filtered[l.cursor]
			return l, func() tea.Msg { return OpenEditorMsg{FilePath: sess.FilePath} }
		}
		return l, nil
	case "ctrl+r":
		return l, func() tea.Msg { return RefreshMsg{} }
	case "?":
		return l, func() tea.Msg { return OpenHelpMsg{} }
	case "t":
		return l, func() tea.Msg { return OpenStatsMsg{} }
	case "T":
		return l, func() tea.Msg { return OpenTrashMsg{} }
	case "esc":
		return l, tea.Quit
	}
	return l, nil
}

// detailLineCount returns how many lines the detail panel will use for the selected session.
func (l ListModel) detailLineCount() int {
	if l.cursor >= len(l.filtered) {
		return 0
	}
	sess := l.filtered[l.cursor]
	if l.compact {
		lines := 2 // leading-blank + metadata
		if sess.LastMsg != "" && sess.LastMsg != sess.FirstMsg {
			lines++
		}
		return lines
	}
	// Full mode: blank + separator + metadata + path + ID = 5
	lines := 5
	if sess.Title != "" {
		lines++
	}
	if sess.LastMsg != "" && sess.LastMsg != sess.FirstMsg {
		lines++
	}
	return lines
}

// listPadding returns total non-list lines (title + search + detail + help + legend).
func (l ListModel) listPadding() int {
	p := listPaddingBase
	if l.searching {
		p++
	}
	return p + l.detailLineCount()
}

// visibleRows returns the number of rows visible in the list.
func (l ListModel) visibleRows() int {
	h := l.height - l.listPadding()
	if h < 1 {
		h = 1
	}
	return h
}

func (l ListModel) handleMouse(msg tea.MouseMsg) (ListModel, tea.Cmd) {
	// Left click: select or double-click resume
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		// In split mode, ignore clicks on the right panel — parent handles this
		// via the wideMode check before routing to us. But we still guard
		// against clicks beyond our width.
		clickedRow := msg.Y - l.headerLines()
		if clickedRow >= 0 {
			idx := l.listOffset + clickedRow
			if idx >= 0 && idx < len(l.filtered) {
				now := time.Now()
				if idx == l.lastClickIdx && now.Sub(l.lastClickTime) < doubleClickThreshold {
					sess := l.filtered[idx]
					return l, func() tea.Msg { return SessionPreviewMsg{Session: sess} }
				}
				l.cursor = idx
				l.lastClickTime = now
				l.lastClickIdx = idx
				l.updateListOffset()
				return l, func() tea.Msg { return FilterChangedMsg{} }
			}
		}
	}

	// Right-click: context menu
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonRight {
		clickedRow := msg.Y - l.headerLines()
		if clickedRow >= 0 {
			idx := l.listOffset + clickedRow
			if idx >= 0 && idx < len(l.filtered) {
				l.cursor = idx
				l.updateListOffset()
				sess := l.filtered[idx]
				x, y := msg.X, msg.Y
				return l, func() tea.Msg {
					return OpenContextMenuMsg{Session: sess, X: x, Y: y}
				}
			}
		}
	}

	// Scroll wheel on left panel
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		prevCursor := l.cursor
		if msg.Button == tea.MouseButtonWheelUp {
			l.cursor -= scrollStep
			if l.cursor < 0 {
				l.cursor = 0
			}
		} else {
			l.cursor += scrollStep
			if l.cursor >= len(l.filtered) {
				l.cursor = len(l.filtered) - 1
			}
			if l.cursor < 0 {
				l.cursor = 0
			}
		}
		if l.cursor == prevCursor {
			return l, nil
		}
		l.updateListOffset()
		return l, func() tea.Msg { return FilterChangedMsg{Debounce: true} }
	}

	return l, nil
}

// --- View methods ---

// sessionRowsConfig holds parameters for renderSessionRows.
type sessionRowsConfig struct {
	listHeight int    // max visible rows
	projWidth  int    // display width for project column
	maxMsgW    int    // display width for message column
	compact    bool   // if true, omit msgCount and relTime columns
	query      string // lowercased search query
}

// sessionRowsResult holds the output of renderSessionRows.
type sessionRowsResult struct {
	rows   string // rendered session rows
	detail string // rendered detail panel for the selected session
}

// renderSessionRows renders the session list rows and detail panel shared by
// both the full-width list and the split view.
func (l ListModel) renderSessionRows(cfg sessionRowsConfig) sessionRowsResult {
	var rows strings.Builder
	var detail strings.Builder

	start := l.listOffset
	end := start + cfg.listHeight
	if end > len(l.filtered) {
		end = len(l.filtered)
	}

	snippetWidth := cfg.maxMsgW
	if snippetWidth > maxSnippetWidth {
		snippetWidth = maxSnippetWidth
	}

	visibleIdx := 0
	for i := start; i < end; i++ {
		s := l.filtered[i]

		timeStr := s.LastTime.Local().Format("01-02 15:04")
		proj := truncateToWidth(s.ProjectShortName(), cfg.projWidth)

		displayMsg := s.FirstMsg
		if cfg.query != "" {
			if snippet := findMatchSnippet(s.SearchText, cfg.query, snippetWidth); snippet != "" {
				displayMsg = snippet
			}
		}
		msg := truncateToWidth(displayMsg, cfg.maxMsgW)

		isSelected := i == l.cursor

		// Badge: raw ANSI foreground only (no trailing reset that kills row bg)
		info := l.sources.Info(s.Source)
		rawBadge := ansiBadgeSource(info) + info.Badge + ansiFgReset

		var plainLine string
		if cfg.compact {
			if !isSelected && cfg.query != "" {
				msg = highlightQuery(msg, cfg.query, normalStyle, highlightStyle)
			}
			plainLine = fmt.Sprintf(" %s %s %s",
				timeStr, padToWidth(proj, cfg.projWidth), msg)
		} else {
			msg = padToWidth(msg, cfg.maxMsgW)
			if !isSelected && cfg.query != "" {
				msg = highlightQuery(msg, cfg.query, normalStyle, highlightStyle)
			}
			msgCount := fmt.Sprintf("%4d", s.MsgCount)
			relTime := relativeTime(s.LastTime)
			plainLine = fmt.Sprintf(" %s  %s  %s %s  %s",
				timeStr, padToWidth(proj, cfg.projWidth), msg, msgCount, dimStyle.Render(relTime))
		}

		// Row styling: raw ANSI bg so lipgloss Render resets don't kill it.
		// Selected row uses full raw ANSI. Other rows have no bg wrapping
		// so lipgloss resets from badge/highlight are harmless.
		var line string
		if isSelected {
			line = ansiSelectedBg() + "> " + rawBadge + plainLine + ansiReset
		} else {
			// Use lipgloss badge for non-selected (no bg conflict)
			badge := sourceBadgeStyle(info).Render(info.Badge)
			line = " " + badge + plainLine
		}

		rows.WriteString(line)
		rows.WriteString("\n")
		visibleIdx++
	}

	// Detail panel for selected session
	if l.cursor < len(l.filtered) {
		sess := l.filtered[l.cursor]
		if cfg.compact {
			// Split view: single compact detail line
			var meta []string
			if sess.Model != "" {
				meta = append(meta, sess.Model)
			}
			if sess.Client != "" {
				meta = append(meta, sess.Client)
			}
			meta = append(meta, sess.FormatDuration(), sess.FormatSize(), fmt.Sprintf("%d tools", sess.ToolCount), fmt.Sprintf("%d turns", sess.MsgCount))
			if sess.TokenUsage.Input > 0 {
				meta = append(meta, fmt.Sprintf("%dk tok", (sess.TokenUsage.Input+sess.TokenUsage.Output)/1000))
			}
			detail.WriteString("\n")
			detail.WriteString(dimStyle.Render(" " + strings.Join(meta, " | ")))
			if sess.LastMsg != "" && sess.LastMsg != sess.FirstMsg {
				last := sess.LastMsg
				maxW := cfg.projWidth + cfg.maxMsgW
				if maxW > maxLastMsgWidth {
					maxW = maxLastMsgWidth
				}
				if maxW > 0 {
					last = truncateToWidth(last, maxW)
				}
				detail.WriteString("\n")
				detail.WriteString(" " + detailLabelStyle.Render("Last: ") + dimStyle.Render(last))
			}
		} else {
			// Full list: multi-line detail panel
			detail.WriteString("\n")
			detail.WriteString(dimStyle.Render(strings.Repeat("─", 40)))
			detail.WriteString("\n")

			var details []string
			if sess.Model != "" {
				details = append(details, detailLabelStyle.Render("Model: ")+detailValueStyle.Render(sess.Model))
			}
			details = append(details, detailLabelStyle.Render("Duration: ")+detailValueStyle.Render(sess.FormatDuration()))
			details = append(details, detailLabelStyle.Render("Size: ")+detailValueStyle.Render(sess.FormatSize()))
			details = append(details, detailLabelStyle.Render("Tools: ")+detailValueStyle.Render(fmt.Sprintf("%d", sess.ToolCount)))
			details = append(details, detailLabelStyle.Render("Turns: ")+detailValueStyle.Render(fmt.Sprintf("%d", sess.MsgCount)))
			if sess.Client != "" {
				details = append(details, detailLabelStyle.Render("Editor: ")+detailValueStyle.Render(sess.Client))
			}
			if sess.TokenUsage.Input > 0 || sess.TokenUsage.Output > 0 {
				details = append(details, detailLabelStyle.Render("Tokens: ")+detailValueStyle.Render(
					fmt.Sprintf("%d in / %d out", sess.TokenUsage.Input, sess.TokenUsage.Output)))
			}
			detail.WriteString(" " + strings.Join(details, "  "))
			detail.WriteString("\n")

			detail.WriteString(" " + pathStyle.Render(sess.ProjectPath))
			detail.WriteString("\n")
			detail.WriteString(" " + detailLabelStyle.Render("ID: ") + dimStyle.Render(sess.ID))
			detail.WriteString("\n")

			if sess.Title != "" {
				detail.WriteString(" " + detailLabelStyle.Render("Thread: ") + detailValueStyle.Render(sess.Title))
				detail.WriteString("\n")
			}

			if sess.LastMsg != "" && sess.LastMsg != sess.FirstMsg {
				last := sess.LastMsg
				maxW := l.width - 12
				if maxW > maxLastMsgWidth {
					maxW = maxLastMsgWidth
				}
				if maxW > 0 {
					last = truncateToWidth(last, maxW)
				}
				detail.WriteString(" " + detailLabelStyle.Render("Last: ") + dimStyle.Render(last))
				detail.WriteString("\n")
			}
		}
	}

	return sessionRowsResult{rows: rows.String(), detail: detail.String()}
}

// renderHelpBar renders key-description pairs with colored styling.
func renderHelpBar(items [][2]string) string {
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			b.WriteString(helpSepStyle.Render("  "))
		}
		b.WriteString(helpKeyStyle.Render(item[0]))
		b.WriteString(helpDescStyle.Render(" " + item[1]))
	}
	return " " + b.String()
}

// renderHelpBarToWidth renders as many key-description pairs as fit.
func renderHelpBarToWidth(items [][2]string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(" ")
	for i, item := range items {
		var candidate strings.Builder
		if i > 0 {
			candidate.WriteString(helpSepStyle.Render("  "))
		}
		candidate.WriteString(helpKeyStyle.Render(item[0]))
		candidate.WriteString(helpDescStyle.Render(" " + item[1]))

		next := b.String() + candidate.String()
		if lipgloss.Width(next) > width {
			break
		}
		b.WriteString(candidate.String())
	}
	return b.String()
}

// renderTitleBar renders a full-width colored title bar with left and right content.
func renderTitleBar(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	return titleBarStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// View renders the full-width list for narrow terminals.
func (l ListModel) View() string {
	var b strings.Builder

	// Title bar
	left := fmt.Sprintf("Sessions (%d)", len(l.filtered))
	right := fmt.Sprintf("Sort: %s  Source: %s  Filter: %s", l.sortMode, l.sourceFilterLabel(), l.filterLabel())
	b.WriteString(renderTitleBar(left, right, l.width))
	b.WriteString("\n")

	// Search bar
	if l.searching {
		b.WriteString(" " + l.searchInput.View())
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Loading state
	if !l.loaded {
		b.WriteString(loadingStyle.Render("Scanning sessions..."))
		return b.String()
	}

	// Empty state
	if len(l.filtered) == 0 {
		if l.searchQuery != "" {
			b.WriteString(emptyStyle.Render(fmt.Sprintf("No sessions matching \"%s\"", l.searchQuery)))
		} else if l.filterProj != "" {
			b.WriteString(emptyStyle.Render("No sessions in this project"))
		} else {
			b.WriteString(emptyStyle.Render("No sessions found"))
		}
		b.WriteString("\n\n")
		help := renderHelpBar([][2]string{
			{"/", "Search"}, {"f", l.filterLabel()}, {"q", "Quit"},
		})
		b.WriteString(help)
		return b.String()
	}

	// Session list
	listHeight := l.height - l.listPadding()
	if listHeight < 1 {
		listHeight = 1
	}

	result := l.renderSessionRows(sessionRowsConfig{
		listHeight: listHeight,
		projWidth:  colWidthProject,
		maxMsgW:    colWidthMsg,
		compact:    false,
		query:      strings.ToLower(l.searchQuery),
	})
	b.WriteString(result.rows)
	b.WriteString(result.detail)

	b.WriteString("\n")
	helpItems := [][2]string{
		{"Enter", "Preview"}, {"r", "Resume"}, {"t", "Stats"}, {"T", "Trash"}, {"/", "Search"},
		{"s", l.sortMode.String()}, {"f", l.filterLabel()}, {"?", "Help"}, {"q", "Quit"},
	}
	if sess, ok := l.SelectedSession(); ok && l.sources.SupportsSafeResumeAction(sess.Source) {
		helpItems = [][2]string{
			{"Enter", "Preview"}, {"r", "Resume"}, {"R", "Safe"}, {"t", "Stats"}, {"T", "Trash"}, {"/", "Search"},
			{"s", l.sortMode.String()}, {"f", l.filterLabel()}, {"?", "Help"}, {"q", "Quit"},
		}
	}
	helpBar := renderHelpBar(helpItems)
	b.WriteString(helpBar)
	b.WriteString("\n")
	b.WriteString(renderSourceLegend(l.sources))

	return b.String()
}

// CompactView renders the left panel for split mode.
func (l ListModel) CompactView(leftWidth int) string {
	var left strings.Builder

	titleLeft := fmt.Sprintf("Sessions (%d)", len(l.filtered))
	titleRight := fmt.Sprintf("Sort: %s  Source: %s  Filter: %s", l.sortMode, l.sourceFilterLabel(), l.filterLabel())
	left.WriteString(renderTitleBar(titleLeft, titleRight, leftWidth))
	left.WriteString("\n")

	if l.searching {
		left.WriteString(" " + l.searchInput.View())
		left.WriteString("\n")
	}
	left.WriteString("\n")

	if !l.loaded {
		left.WriteString(loadingStyle.Render("Scanning..."))
	} else if len(l.filtered) == 0 {
		if l.searchQuery != "" {
			left.WriteString(emptyStyle.Render(fmt.Sprintf("No match: \"%s\"", l.searchQuery)))
		} else {
			left.WriteString(emptyStyle.Render("No sessions found"))
		}
	} else {
		listHeight := l.height - l.listPadding()
		if listHeight < 1 {
			listHeight = 1
		}

		maxMsgW := leftWidth - splitMsgOffset
		if maxMsgW < 10 {
			maxMsgW = 10
		}

		result := l.renderSessionRows(sessionRowsConfig{
			listHeight: listHeight,
			projWidth:  colWidthProjectSplit,
			maxMsgW:    maxMsgW,
			compact:    true,
			query:      strings.ToLower(l.searchQuery),
		})
		left.WriteString(result.rows)
		left.WriteString(result.detail)
	}

	left.WriteString("\n")
	helpItems := [][2]string{
		{"Enter", "Preview"}, {"r", "Resume"}, {"t", "Stats"}, {"/", "Search"},
		{"s", l.sortMode.String()}, {"f", l.filterLabel()}, {"q", "Quit"}, {"T", "Trash"}, {"?", "Help"},
	}
	if sess, ok := l.SelectedSession(); ok && l.sources.SupportsSafeResumeAction(sess.Source) {
		helpItems = [][2]string{
			{"Enter", "Preview"}, {"r", "Resume"}, {"R", "Safe"}, {"t", "Stats"}, {"/", "Search"},
			{"s", l.sortMode.String()}, {"f", l.filterLabel()}, {"q", "Quit"}, {"T", "Trash"}, {"?", "Help"},
		}
	}
	left.WriteString(renderHelpBarToWidth(helpItems, leftWidth-2))
	left.WriteString("\n")
	left.WriteString(renderSourceLegend(l.sources))

	return left.String()
}

// --- Internal methods ---

// filterLabel returns the display label for the current project filter.
func (l ListModel) filterLabel() string {
	if l.filterProj == "" {
		return "All"
	}
	s := session.Session{ProjectPath: l.filterProj}
	return s.ProjectShortName()
}

func (l ListModel) sourceFilterLabel() string {
	if l.sourceFilter == sourceAll {
		return "All"
	}
	all := l.sources.All()
	idx := int(l.sourceFilter) - 1
	if idx < 0 || idx >= len(all) {
		return "All"
	}
	return all[idx].Label
}

func (l ListModel) selectedSourceFilter() session.Source {
	if l.sourceFilter == sourceAll {
		return ""
	}
	all := l.sources.All()
	idx := int(l.sourceFilter) - 1
	if idx < 0 || idx >= len(all) {
		return ""
	}
	return all[idx].Source
}

func renderSourceLegend(sources session.SourceRegistry) string {
	var parts []string
	for _, info := range sources.All() {
		parts = append(parts, sourceBadgeStyle(info).Render(info.Badge)+helpDescStyle.Render(" "+info.Label))
	}
	return " " + strings.Join(parts, helpDescStyle.Render("  "))
}

// headerLines returns the number of header lines above the session list.
func (l ListModel) headerLines() int {
	if l.searching {
		return 3
	}
	return 2
}

func (l *ListModel) applySort() {
	switch l.sortMode {
	case sortRecent:
		sort.Sort(session.SortByLastTime(l.filtered))
	case sortProject:
		sort.SliceStable(l.filtered, func(i, j int) bool {
			left := l.filtered[i].ProjectPath
			if left == "" {
				left = l.filtered[i].ProjectDir
			}
			right := l.filtered[j].ProjectPath
			if right == "" {
				right = l.filtered[j].ProjectDir
			}
			if left == right {
				return l.filtered[i].LastTime.After(l.filtered[j].LastTime)
			}
			return left < right
		})
	case sortMsgCount:
		sort.Sort(session.SortByMsgCount(l.filtered))
	case sortToolCount:
		sort.Sort(session.SortByToolCount(l.filtered))
	case sortTokens:
		sort.Sort(session.SortByTotalTokens(l.filtered))
	}
}

func (l *ListModel) applyFilter() {
	l.filtered = appquery.NewService().Filter(l.sessions, appquery.Filter{
		Query:       l.searchQuery,
		ProjectPath: l.filterProj,
		Source:      l.selectedSourceFilter(),
	})
	l.applySort()
	if l.cursor >= len(l.filtered) {
		if len(l.filtered) > 0 {
			l.cursor = len(l.filtered) - 1
		} else {
			l.cursor = 0
		}
	}
}

func (l *ListModel) updateListOffset() {
	listHeight := l.height - l.listPadding()
	if listHeight < 1 {
		listHeight = 1
	}
	if l.cursor < l.listOffset {
		l.listOffset = l.cursor
	}
	if l.cursor >= l.listOffset+listHeight {
		l.listOffset = l.cursor - listHeight + 1
	}
}

func (l *ListModel) focusSessionID(id string) bool {
	for i, sess := range l.filtered {
		if sess.ID == id {
			l.cursor = i
			l.updateListOffset()
			return true
		}
	}
	return false
}

func uniqueProjects(sessions []session.Session) []string {
	seen := map[string]bool{}
	var result []string
	for _, s := range sessions {
		if !seen[s.ProjectDir] {
			seen[s.ProjectDir] = true
			result = append(result, s.ProjectDir)
		}
	}
	sort.Strings(result)
	return result
}
