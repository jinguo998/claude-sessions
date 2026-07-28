package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

const (
	previewPaddingH  = 4  // horizontal padding for full-screen preview
	previewPaddingV  = 6  // vertical padding for full-screen preview
	initialViewportW = 80 // initial viewport width
	initialViewportH = 24 // initial viewport height
)

// PreviewModel is a self-contained Bubble Tea sub-component for the
// full-screen preview pane, including vim-style navigation and in-preview
// search.
type PreviewModel struct {
	viewport viewport.Model
	search   previewSearch
	lastKey  string // for detecting gg sequence
	title    string
	width    int
	height   int
	filePath string // session file path for reloading in verbose mode
	verbose  bool   // true = show full tool content + results
	markdown bool   // true = render message bodies as Markdown
	loading  bool
	session  session.Session // the session being previewed, for resume/fork
	sources  session.SourceRegistry
	msgLines []int // line number where each user/assistant message starts
}

// NewPreviewModel creates an initialised PreviewModel.
func NewPreviewModel(sources ...session.SourceRegistry) PreviewModel {
	vp := viewport.New(initialViewportW, initialViewportH)
	vp.MouseWheelDelta = scrollStep
	registry := session.SourceRegistry{}
	if len(sources) > 0 {
		registry = sources[0]
	}
	return PreviewModel{
		viewport: vp,
		search:   newPreviewSearch(),
		markdown: true,
		sources:  registry,
	}
}

// SetContent sets the preview title & body, stores content in the search
// component, and scrolls to the bottom.
func (p PreviewModel) SetContent(title string, result previewResult, sess session.Session) PreviewModel {
	p.title = title
	p.filePath = sess.FilePath
	p.session = sess
	p.loading = false
	p.msgLines = result.msgLines
	p.viewport.SetContent(result.content)
	p.search.SetContent(result.content)
	p.viewport.GotoBottom()
	return p
}

// SetLoading updates metadata immediately while content is loaded asynchronously.
func (p PreviewModel) SetLoading(title string, sess session.Session) PreviewModel {
	p.title = title
	p.filePath = sess.FilePath
	p.session = sess
	p.loading = true
	p.msgLines = nil
	p.search.Close()
	p.search.SetContent("")
	p.viewport.SetContent(loadingStyle.Render("Loading preview..."))
	p.viewport.GotoTop()
	return p
}

// CurrentMessageIndex returns the message currently nearest the viewport top.
func (p PreviewModel) CurrentMessageIndex() int {
	curOffset := p.viewport.YOffset
	msgIdx := 0
	for i, line := range p.msgLines {
		if line > curOffset {
			break
		}
		msgIdx = i
	}
	return msgIdx
}

// ApplyLoaded replaces loading content with rendered preview content.
func (p PreviewModel) ApplyLoaded(result previewResult, preserveMsgIdx int, scrollBottom bool) PreviewModel {
	p.loading = false
	p.viewport.SetContent(result.content)
	p.search.SetContent(result.content)
	p.msgLines = result.msgLines

	if scrollBottom {
		p.viewport.GotoBottom()
	} else if preserveMsgIdx < len(p.msgLines) {
		p.viewport.SetYOffset(p.msgLines[preserveMsgIdx])
	}
	return p
}

func (p PreviewModel) SetNoContent() PreviewModel {
	p.loading = false
	noContent := dimStyle.Render("(no previewable content)")
	p.viewport.SetContent(noContent)
	p.search.SetContent(noContent)
	p.msgLines = nil
	return p
}

// SetSize updates the stored dimensions and recalculates the viewport size.
func (p PreviewModel) SetSize(w, h int) PreviewModel {
	p.width = w
	p.height = h
	p.viewport.Width = w - previewPaddingH
	p.viewport.Height = h - previewPaddingV
	return p
}

// Update handles all key and mouse messages while the preview is active.
func (p PreviewModel) Update(msg tea.Msg) (PreviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p.updateKey(msg)
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			var cmd tea.Cmd
			p.viewport, cmd = p.viewport.Update(msg)
			return p, cmd
		}
		return p, nil
	}
	return p, nil
}

func (p PreviewModel) updateKey(msg tea.KeyMsg) (PreviewModel, tea.Cmd) {
	key := msg.String()

	// When search input is active, route keys to the search input
	if p.search.active {
		switch key {
		case "esc":
			p.search.Close()
			p.viewport.SetContent(p.search.content)
			return p, nil
		case "enter":
			p.search.Execute()
			p.search.active = false
			p.search.input.Blur()
			if len(p.search.hits) > 0 {
				p.viewport.SetContent(p.search.HighlightContent())
				p.viewport.SetYOffset(p.search.hits[p.search.current])
			}
			return p, nil
		default:
			var cmd tea.Cmd
			p.search.input, cmd = p.search.input.Update(msg)
			return p, cmd
		}
	}

	// IMPORTANT: record previous key BEFORE overwriting, not via defer.
	prevKey := p.lastKey
	p.lastKey = key

	switch key {
	case "esc":
		if p.search.query != "" {
			p.search.Close()
			p.viewport.SetContent(p.search.content)
			p.lastKey = ""
			return p, nil
		}
		p.lastKey = ""
		return p, func() tea.Msg { return PreviewCloseMsg{} }
	case "q", "p":
		p.search.Close()
		p.lastKey = ""
		return p, func() tea.Msg { return PreviewCloseMsg{} }
	case "r":
		sess := p.session
		p.lastKey = ""
		return p, func() tea.Msg { return defaultResumeSelection(p.sources, sess) }
	case "R":
		sess := p.session
		p.lastKey = ""
		return p, func() tea.Msg { return safeResumeSelection(sess) }
	case "n":
		// When search is active, n = next match; otherwise n = fork
		if p.search.query != "" {
			if line := p.search.Next(); line >= 0 {
				p.viewport.SetContent(p.search.HighlightContent())
				p.viewport.SetYOffset(line)
			}
			return p, nil
		}
		sess := p.session
		p.lastKey = ""
		return p, func() tea.Msg { return forkSelection(p.sources, sess) }
	case "v":
		p.verbose = !p.verbose
		p.lastKey = ""
		return p, func() tea.Msg { return PreviewReloadMsg{Verbose: p.verbose} }
	case "m":
		p.markdown = !p.markdown
		p.lastKey = ""
		return p, func() tea.Msg { return PreviewReloadMsg{Verbose: p.verbose} }
	case "/":
		p.search.Open()
		return p, textinput.Blink
	case "N":
		if p.search.query != "" {
			if line := p.search.Prev(); line >= 0 {
				p.viewport.SetContent(p.search.HighlightContent())
				p.viewport.SetYOffset(line)
			}
		}
		return p, nil
	case "g":
		if prevKey == "g" {
			p.viewport.GotoTop()
			p.lastKey = ""
			return p, nil
		}
		return p, nil
	case "G":
		p.viewport.GotoBottom()
		return p, nil
	case "ctrl+d":
		p.viewport.HalfViewDown()
		return p, nil
	case "ctrl+u":
		p.viewport.HalfViewUp()
		return p, nil
	case "ctrl+f":
		p.viewport.ViewDown()
		return p, nil
	case "ctrl+b":
		p.viewport.ViewUp()
		return p, nil
	default:
		var cmd tea.Cmd
		p.viewport, cmd = p.viewport.Update(msg)
		return p, cmd
	}
}

// View renders the preview: title bar + viewport + search/status/help bar.
func (p PreviewModel) View() string {
	var b strings.Builder

	if p.title != "" {
		modeParts := []string{}
		if p.loading {
			modeParts = append(modeParts, "LOADING")
		}
		if p.verbose {
			modeParts = append(modeParts, "VERBOSE")
		}
		if !p.markdown {
			modeParts = append(modeParts, "RAW")
		}
		mode := ""
		if len(modeParts) > 0 {
			mode = " [" + strings.Join(modeParts, " ") + "]"
		}
		left := fmt.Sprintf("Preview: %s%s", p.title, mode)
		gap := p.width - lipgloss.Width(left) - 4
		if gap < 1 {
			gap = 1
		}
		b.WriteString(titleBarStyle.Render(left + strings.Repeat(" ", gap)))
		b.WriteString("\n\n")
	}

	b.WriteString(p.viewport.View())
	b.WriteString("\n")

	if p.search.active {
		b.WriteString(" /" + p.search.input.View())
		b.WriteString("\n")
	} else if status := p.search.StatusText(); status != "" {
		help := renderHelpBar([][2]string{
			{"/", "Search"}, {"n", "Next"}, {"N", "Prev"},
		})
		b.WriteString(help + "  " + searchStatusStyle.Render(status) + "  ")
		b.WriteString(renderHelpBar([][2]string{
			{"Esc", "Clear"}, {"q", "Back"},
		}))
		b.WriteString("\n")
	} else {
		verboseLabel := "Verbose"
		if p.verbose {
			verboseLabel = "Summary"
		}
		markdownLabel := "Raw"
		if !p.markdown {
			markdownLabel = "Markdown"
		}
		helpItems := [][2]string{
			{"r", "Resume"}, {"n", "Fork"}, {"j/k", "Scroll"}, {"gg", "Top"}, {"G", "Bottom"},
			{"^D/^U", "½Page"}, {"^F/^B", "Page"}, {"/", "Search"},
			{"v", verboseLabel}, {"m", markdownLabel}, {"M", "Select text"}, {"Esc", "Back"},
		}
		if p.sources.SupportsSafeResumeAction(p.session.Source) {
			helpItems = [][2]string{
				{"r", "Resume"}, {"R", "Safe"}, {"n", "Fork"}, {"j/k", "Scroll"}, {"gg", "Top"}, {"G", "Bottom"},
				{"^D/^U", "½Page"}, {"^F/^B", "Page"}, {"/", "Search"},
				{"v", verboseLabel}, {"m", markdownLabel}, {"M", "Select text"}, {"Esc", "Back"},
			}
		}
		help := renderHelpBar(helpItems)
		b.WriteString(help)
		b.WriteString("\n")
	}

	return b.String()
}
