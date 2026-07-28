package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
	apppreview "github.com/jinguo998/claude-sessions/internal/app/preview"
	"github.com/jinguo998/claude-sessions/internal/domain"
)

var (
	markdownStyle        = "dark"
	markdownColorProfile = termenv.ANSI256
)

func ConfigureTerminalTheme(style string) {
	if style != "" {
		markdownStyle = style
	}
	markdownColorProfile = termenv.EnvColorProfile()
	lipgloss.SetHasDarkBackground(markdownStyle != "light")
	lipgloss.SetColorProfile(markdownColorProfile)
}

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
	if maxWidth <= 0 {
		return ""
	}
	if displayWidth(s) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
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

// wrapText wraps text to the given width. If width <= 0, returns text unchanged.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var result strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if displayWidth(line) <= width {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}
		// Wrap long line
		runes := []rune(line)
		w := 0
		start := 0
		for i, r := range runes {
			rw := runewidth.RuneWidth(r)
			if w+rw > width {
				result.WriteString(string(runes[start:i]))
				result.WriteString("\n")
				start = i
				w = 0
			}
			w += rw
		}
		if start < len(runes) {
			result.WriteString(string(runes[start:]))
			result.WriteString("\n")
		}
	}
	return strings.TrimRight(result.String(), "\n")
}

// highlightQuery highlights all occurrences of query in text (case-insensitive).
// baseStyle is applied to non-matching segments, matchStyle to matching segments.
func highlightQuery(text, query string, baseStyle, matchStyle lipgloss.Style) string {
	if query == "" {
		return text
	}
	lower := strings.ToLower(text)
	q := strings.ToLower(query)
	var b strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], q)
		if idx < 0 {
			b.WriteString(baseStyle.Render(text[pos:]))
			break
		}
		if idx > 0 {
			b.WriteString(baseStyle.Render(text[pos : pos+idx]))
		}
		b.WriteString(matchStyle.Render(text[pos+idx : pos+idx+len(q)]))
		pos += idx + len(q)
	}
	return b.String()
}

// findMatchSnippet finds the query in text and returns a surrounding snippet.
// The snippet is centered on the match and limited to maxLen runes.
// text is expected to be lowercased; the returned snippet preserves that case.
func findMatchSnippet(text, query string, maxLen int) string {
	idx := strings.Index(text, query)
	if idx < 0 {
		return ""
	}

	// Convert byte index to rune context
	runes := []rune(text)
	// Find rune index of match
	runeIdx := len([]rune(text[:idx]))

	// Center the snippet around the match
	snippetStart := runeIdx - maxLen/4
	if snippetStart < 0 {
		snippetStart = 0
	}
	snippetEnd := snippetStart + maxLen
	if snippetEnd > len(runes) {
		snippetEnd = len(runes)
		snippetStart = snippetEnd - maxLen
		if snippetStart < 0 {
			snippetStart = 0
		}
	}

	snippet := string(runes[snippetStart:snippetEnd])
	snippet = strings.Join(strings.Fields(snippet), " ")

	prefix := ""
	if snippetStart > 0 {
		prefix = "..."
	}
	suffix := ""
	if snippetEnd < len(runes) {
		suffix = "..."
	}
	return prefix + snippet + suffix
}

// LoadPreviewContent parses a session file and returns the formatted preview content.
// Returns the content string and true on success, or ("", false) on failure.
// If verbose is true, tool results and full tool inputs are included.
// Markdown rendering is enabled by default; pass a second false option for raw text.
func LoadPreviewContent(loader *apppreview.Service, sess session.Session, width int, options ...bool) (previewResult, bool) {
	full := len(options) > 0 && options[0]
	renderMarkdown := len(options) < 2 || options[1]
	if loader == nil {
		return previewResult{}, false
	}
	msgs, err := loader.Load(context.Background(), sess.Domain(), full)
	if err != nil || len(msgs) == 0 {
		return previewResult{}, false
	}
	return formatPreviewWithColors(msgs, width, full, renderMarkdown), true
}

const (
	sidePreviewMessageLimit = 32
	sidePreviewTailBytes    = 2 * 1024 * 1024
	sidePreviewMaxTextRunes = 4000
)

// LoadSidePreviewContent renders only recent messages for the split-view preview.
func LoadSidePreviewContent(loader *apppreview.Service, sess session.Session, width int) (previewResult, bool) {
	totalStart := time.Now()
	parseStart := time.Now()
	var msgs []domain.ConversationTurn
	var err error
	if loader != nil {
		msgs, err = loader.LoadTail(context.Background(), sess.Domain(), false, sidePreviewMessageLimit, sidePreviewTailBytes)
	}
	parseDuration := time.Since(parseStart)
	if err != nil || len(msgs) == 0 {
		fields := map[string]any{
			"file_path":  sess.FilePath,
			"source":     string(sess.Source),
			"width":      width,
			"ok":         false,
			"msg_count":  len(msgs),
			"parse_ms":   traceDurationMS(parseDuration),
			"total_ms":   traceDurationMS(time.Since(totalStart)),
			"error":      "",
			"tail_bytes": sidePreviewTailBytes,
		}
		if err != nil {
			fields["error"] = err.Error()
		}
		traceEvent("side_preview_content", fields)
		return previewResult{}, false
	}
	msgs = compactSidePreviewMessages(msgs)
	renderStart := time.Now()
	result := formatPreviewWithColors(msgs, width, false, true)
	renderDuration := time.Since(renderStart)
	traceEvent("side_preview_content", map[string]any{
		"file_path":     sess.FilePath,
		"source":        string(sess.Source),
		"width":         width,
		"ok":            true,
		"msg_count":     len(msgs),
		"parse_ms":      traceDurationMS(parseDuration),
		"render_ms":     traceDurationMS(renderDuration),
		"total_ms":      traceDurationMS(time.Since(totalStart)),
		"content_bytes": len(result.content),
		"content_lines": strings.Count(result.content, "\n") + 1,
		"tail_bytes":    sidePreviewTailBytes,
	})
	return result, true
}

// LoadFullSidePreviewContent renders the whole session for an on-demand side
// preview expansion after the user scrolls beyond the initial tail.
func LoadFullSidePreviewContent(loader *apppreview.Service, sess session.Session, width int) (previewResult, bool) {
	totalStart := time.Now()
	result, ok := LoadPreviewContent(loader, sess, width, false, true)
	traceEvent("side_preview_full_content", map[string]any{
		"file_path":     sess.FilePath,
		"source":        string(sess.Source),
		"width":         width,
		"ok":            ok,
		"total_ms":      traceDurationMS(time.Since(totalStart)),
		"content_bytes": len(result.content),
		"content_lines": strings.Count(result.content, "\n") + 1,
	})
	return result, ok
}

func compactSidePreviewMessages(msgs []domain.ConversationTurn) []domain.ConversationTurn {
	if sidePreviewMaxTextRunes <= 0 {
		return msgs
	}
	compact := make([]domain.ConversationTurn, len(msgs))
	for i, msg := range msgs {
		compact[i] = msg
		compact[i].Text = truncateRunes(msg.Text, sidePreviewMaxTextRunes)
	}
	return compact
}

func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return strings.TrimRight(string(runes[:maxRunes]), " \n\t") + "\n... (truncated for side preview)"
}

// previewResult holds rendered content and message line positions.
type previewResult struct {
	content  string
	msgLines []int // line number where each message (user/assistant) starts
}

// formatPreviewWithColors renders messages with colored role labels.
// If width > 0, long lines are wrapped to fit.
// In verbose mode, tool content is fully wrapped instead of truncated to one line.
// Returns content and a map of message index → start line number.
func formatPreviewWithColors(msgs []domain.ConversationTurn, width int, verbose bool, markdown ...bool) previewResult {
	var b strings.Builder
	var msgLines []int
	lineCount := 0
	renderMarkdown := len(markdown) == 0 || markdown[0]
	var markdownRenderer *glamour.TermRenderer
	if renderMarkdown {
		markdownRenderer = newMarkdownRenderer(width)
	}
	if markdownRenderer != nil {
		defer markdownRenderer.Close()
	}

	countLines := func(s string) int {
		return strings.Count(s, "\n")
	}

	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			if b.Len() > 0 {
				b.WriteString("\n")
				lineCount++
			}
			msgLines = append(msgLines, lineCount)
			b.WriteString(userStyle.Render(" User "))
			b.WriteString("\n")
			lineCount++
			text := renderMarkdownMessage(markdownRenderer, msg.Text, width, userTextStyle)
			b.WriteString(text)
			b.WriteString("\n\n")
			lineCount += countLines(text) + 2
		case "tool":
			if verbose {
				for i, line := range strings.Split(msg.Text, "\n") {
					prefix := "│ "
					if i > 0 {
						prefix = "  "
					}
					wrapped := wrapText(prefix+line, width)
					b.WriteString(toolStyle.Render(wrapped))
					b.WriteString("\n")
					lineCount += countLines(wrapped) + 1
				}
			} else {
				toolText := "│ " + strings.SplitN(msg.Text, "\n", 2)[0]
				if width > 0 {
					toolText = truncateToWidth(toolText, width)
				}
				b.WriteString(toolStyle.Render(toolText))
				b.WriteString("\n")
				lineCount++
			}
		case "tool_result":
			resultText := wrapText(msg.Text, width-2)
			for _, line := range strings.Split(resultText, "\n") {
				b.WriteString(toolResultStyle.Render("  " + line))
				b.WriteString("\n")
				lineCount++
			}
		case "approval":
			approvalText := "  " + strings.SplitN(msg.Text, "\n", 2)[0]
			if width > 0 {
				approvalText = truncateToWidth(approvalText, width)
			}
			b.WriteString(approvalStyle.Render(approvalText))
			b.WriteString("\n")
			lineCount++
		case "assistant":
			if b.Len() > 0 {
				b.WriteString("\n")
				lineCount++
			}
			msgLines = append(msgLines, lineCount)
			b.WriteString(assistantStyle.Render(" Assistant "))
			b.WriteString("\n")
			lineCount++
			text := renderMarkdownMessage(markdownRenderer, msg.Text, width, assistantTextStyle)
			b.WriteString(text)
			b.WriteString("\n\n")
			lineCount += countLines(text) + 2
		}
	}
	return previewResult{content: b.String(), msgLines: msgLines}
}

func newMarkdownRenderer(width int) *glamour.TermRenderer {
	if width <= 0 {
		width = 80
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(markdownStyle),
		glamour.WithColorProfile(markdownColorProfile),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return nil
	}
	return renderer
}

func renderMarkdownMessage(renderer *glamour.TermRenderer, text string, width int, fallbackStyle lipgloss.Style) string {
	if renderer == nil || strings.TrimSpace(text) == "" {
		return fallbackStyle.Render(wrapText(text, width))
	}
	rendered, err := renderer.Render(text)
	if err != nil {
		return fallbackStyle.Render(wrapText(text, width))
	}
	rendered = strings.Trim(rendered, "\n")
	if rendered == "" {
		return fallbackStyle.Render(wrapText(text, width))
	}
	return rendered
}
