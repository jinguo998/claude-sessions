package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	splitBorderCols       = 3  // columns for the split border (space + pipe + space)
	splitPreviewPadding   = 6  // lines subtracted from height for side preview
	sidePreviewScrollStep = 12 // lines to scroll per right-pane wheel tick
)

func (m Model) View() string {
	if m.result != nil {
		return ""
	}

	var base string
	switch m.view {
	case viewPreview:
		return m.preview.View()
	case viewConfirmDelete:
		base = m.renderListOrSplit() + "\n" + m.renderDeleteConfirm()
	case viewConfirmTrashDelete:
		base = m.trash.View() + "\n" + m.renderTrashDeleteConfirm()
	case viewConfirmTrashEmpty:
		base = m.trash.View() + "\n" + m.renderTrashEmptyConfirm()
	case viewContextMenu:
		base = m.contextMenu.OverlayOn(m.renderListOrSplit())
	case viewProjectPicker:
		base = m.projectPicker.OverlayOn(m.renderListOrSplit())
	case viewHelp:
		base = m.renderHelp()
	case viewStats:
		base = m.renderStats()
	case viewTrash:
		base = m.trash.View()
	default:
		base = m.renderListOrSplit()
	}
	if m.flash != "" {
		return m.overlayFlash(base)
	}
	return base
}

// overlayFlash replaces the last non-empty line of base with a flash notification.
func (m Model) overlayFlash(base string) string {
	lines := strings.Split(base, "\n")
	flashLine := flashStyle.Render(" " + m.flash + " ")
	// Replace the last line with the flash message
	if len(lines) > 0 {
		lines[len(lines)-1] = flashLine
	}
	return strings.Join(lines, "\n")
}

// renderListOrSplit returns either the full-width list or the split view,
// depending on the terminal width.
func (m Model) renderListOrSplit() string {
	if m.wideMode() {
		return m.renderSplitView()
	}
	return m.list.View()
}

func (m Model) renderSplitView() string {
	var traceStart time.Time
	if traceEnabled() {
		traceStart = time.Now()
	}
	leftWidth := m.width/2 - 1
	rightWidth := m.width - leftWidth - splitBorderCols

	// Left panel: compact session list
	leftStart := time.Now()
	leftContent := m.list.CompactView(leftWidth)
	leftDuration := time.Since(leftStart)

	// Right panel: preview (rendered by SidePreviewModel)
	rightStart := time.Now()
	rightContent := m.sidePreview.View(rightWidth)
	rightDuration := time.Since(rightStart)

	// Combine with border
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	border := borderStyle.Render("\u2502")

	var out strings.Builder
	for i := 0; i < maxLines; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lw := lipgloss.Width(l)
		if lw < leftWidth {
			l += strings.Repeat(" ", leftWidth-lw)
		}
		out.WriteString(l)
		out.WriteString(" ")
		out.WriteString(border)
		out.WriteString(" ")
		out.WriteString(r)
		out.WriteString("\n")
	}

	result := out.String()
	if !traceStart.IsZero() {
		traceEvent("split_view_render", map[string]any{
			"width":            m.width,
			"height":           m.height,
			"left_width":       leftWidth,
			"right_width":      rightWidth,
			"selected_session": m.sidePreview.sessionID,
			"preview_token":    m.sidePreview.requestToken,
			"preview_loading":  m.sidePreview.loading,
			"left_ms":          traceDurationMS(leftDuration),
			"right_ms":         traceDurationMS(rightDuration),
			"total_ms":         traceDurationMS(time.Since(traceStart)),
			"left_bytes":       len(leftContent),
			"right_bytes":      len(rightContent),
			"output_bytes":     len(result),
			"output_lines":     strings.Count(result, "\n") + 1,
		})
	}
	return result
}

func (m Model) renderHelp() string {
	titleText := "  Keyboard Shortcuts"
	title := titleBarStyle.Render(titleText + strings.Repeat(" ", max(m.width-lipgloss.Width(titleText)-4, 1)))

	sections := []struct {
		name  string
		items [][2]string
	}{
		{"Navigation", [][2]string{
			{"j/k ↑↓", "Move cursor"},
			{"g/G", "Jump to top/bottom"},
			{"^D/^U", "Half page down/up"},
			{"Enter/p", "Preview session"},
		}},
		{"Actions", [][2]string{
			{"r", "Resume session"},
			{"R", "Resume safely"},
			{"n", "Fork session"},
			{"d", "Archive session"},
			{"y", "Copy session ID"},
			{"O", "Open JSONL in $EDITOR"},
			{"^R", "Refresh session list"},
		}},
		{"Filter & Sort", [][2]string{
			{"/", "Search sessions"},
			{"s", "Cycle sort mode"},
			{"f", "Filter by project"},
			{"F", "Filter by source"},
		}},
		{"Other", [][2]string{
			{"t", "Stats"},
			{"T", "Trash"},
			{"m", "Context menu"},
			{"M", "Toggle mouse/select"},
			{"?", "This help"},
			{"q/Esc", "Quit"},
		}},
		{"Stats", [][2]string{
			{"[/]", "Cycle time range"},
			{"1/2/3/4", "24h/7d/30d/all"},
			{"j/k", "Move resume queue"},
			{"Enter/p", "Preview selected"},
			{"r/R", "Resume fast/safe"},
			{"f", "Filter to project"},
			{"l", "Focus in list"},
		}},
		{"Trash", [][2]string{
			{"p", "Preview archived session"},
			{"Enter/r", "Restore"},
			{"x", "Permanently delete"},
			{"D", "Empty trash"},
		}},
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")

	for _, sec := range sections {
		b.WriteString("  " + menuTitleStyle.Render(sec.name) + "\n")
		for _, item := range sec.items {
			key := helpKeyStyle.Render(fmt.Sprintf("  %-10s", item[0]))
			b.WriteString(key + helpDescStyle.Render(item[1]) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("  Press any key to close"))
	return b.String()
}

func (m Model) renderStats() string {
	stats := m.stats.SetSize(m.width, m.height).SetSessions(m.list.Filtered(), len(m.list.sessions), m.statsScopeSummary())
	return stats.View()
}

func (m Model) statsScopeSummary() string {
	parts := []string{}
	if m.list.searchQuery != "" {
		parts = append(parts, fmt.Sprintf("query=%q", m.list.searchQuery))
	}
	if m.list.filterProj != "" {
		parts = append(parts, "project="+m.list.filterLabel())
	}
	if m.list.sourceFilter != sourceAll {
		parts = append(parts, "source="+m.list.sourceFilterLabel())
	}
	if len(parts) == 0 {
		return "scope: entire visible workspace"
	}
	return "scope: " + strings.Join(parts, "  |  ")
}

func (m Model) renderDeleteConfirm() string {
	if m.deleteIdx >= 0 && m.deleteIdx < len(m.list.Filtered()) {
		sess := m.list.Filtered()[m.deleteIdx]
		return confirmStyle.Render(fmt.Sprintf(
			" Archive session %s (%s) to trash? [y/N] ",
			truncateID(sess.ID), sess.ProjectShortName()))
	}
	return ""
}

func (m Model) renderTrashDeleteConfirm() string {
	if m.trashDeleteItem.Metadata.ID == "" {
		return ""
	}
	return confirmStyle.Render(fmt.Sprintf(
		" Permanently delete archived session %s? [y/N] ",
		truncateID(m.trashDeleteItem.Metadata.ID)))
}

func (m Model) renderTrashEmptyConfirm() string {
	count := len(m.trash.Items())
	return confirmStyle.Render(fmt.Sprintf(
		" Permanently delete all %d archived sessions? [y/N] ",
		count))
}
