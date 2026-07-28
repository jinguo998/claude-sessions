package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

// projectItem represents one entry in the project picker.
type projectItem struct {
	dir   string // raw encoded dir ("" for All)
	label string // display name
	count int    // number of sessions
}

// ProjectPickerModel is an overlay for selecting a project filter.
type ProjectPickerModel struct {
	items   []projectItem
	cursor  int
	x, y    int
	current string // currently active filter (to highlight)
}

// NewProjectPickerModel returns a zero-value picker.
func NewProjectPickerModel() ProjectPickerModel {
	return ProjectPickerModel{}
}

// Open builds the project list from sessions and prepares the picker for display.
func (pp ProjectPickerModel) Open(sessions []session.Session, currentFilter string, x, y int) ProjectPickerModel {
	// Count sessions per project path (merges sources for same directory)
	counts := map[string]int{}
	for _, s := range sessions {
		counts[s.ProjectPath]++
	}

	// Build sorted unique project list
	var items []projectItem
	items = append(items, projectItem{dir: "", label: "All", count: len(sessions)})

	seen := map[string]bool{}
	for _, s := range sessions {
		if seen[s.ProjectPath] {
			continue
		}
		seen[s.ProjectPath] = true
		items = append(items, projectItem{
			dir:   s.ProjectPath,
			label: s.ProjectShortName(),
			count: counts[s.ProjectPath],
		})
	}

	// Set cursor to current filter
	cursor := 0
	for i, item := range items {
		if item.dir == currentFilter {
			cursor = i
			break
		}
	}

	pp.items = items
	pp.cursor = cursor
	pp.x = x
	pp.y = y
	pp.current = currentFilter
	return pp
}

// Update handles keyboard and mouse input for the project picker.
func (pp ProjectPickerModel) Update(msg tea.Msg) (ProjectPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if pp.cursor > 0 {
				pp.cursor--
			}
			return pp, nil
		case "down", "j":
			if pp.cursor < len(pp.items)-1 {
				pp.cursor++
			}
			return pp, nil
		case "enter":
			item := pp.items[pp.cursor]
			return pp, func() tea.Msg {
				return ProjectSelectedMsg{ProjectDir: item.dir}
			}
		case "esc", "q", "f":
			return pp, func() tea.Msg { return ProjectPickerCloseMsg{} }
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Menu layout: border(1) + title(1) + divider(1) + items + border(1)
			itemTop := pp.y + 1 + 2 // border + title + divider
			itemBot := itemTop + len(pp.items)
			if msg.Y >= itemTop && msg.Y < itemBot && msg.X >= pp.x && msg.X < pp.x+pp.viewWidth()+4 {
				pp.cursor = msg.Y - itemTop
				item := pp.items[pp.cursor]
				return pp, func() tea.Msg {
					return ProjectSelectedMsg{ProjectDir: item.dir}
				}
			}
			// Click outside → close
			return pp, func() tea.Msg { return ProjectPickerCloseMsg{} }
		}
	}
	return pp, nil
}

func (pp ProjectPickerModel) viewWidth() int {
	maxW := 20 // minimum width
	for _, item := range pp.items {
		label := fmt.Sprintf(" %s (%d) ", item.label, item.count)
		if w := displayWidth(label) + 2; w > maxW {
			maxW = w
		}
	}
	if maxW > 40 {
		maxW = 40
	}
	return maxW
}

// View renders the bordered picker box.
func (pp ProjectPickerModel) View() string {
	w := pp.viewWidth()
	var content strings.Builder

	content.WriteString(menuTitleStyle.Render("Filter by Project"))
	content.WriteString("\n")
	content.WriteString(menuDividerStyle.Render(strings.Repeat("─", w)))
	content.WriteString("\n")

	itemFmt := fmt.Sprintf(" %%-%ds", w-1)
	for i, item := range pp.items {
		label := fmt.Sprintf("%s (%d)", item.label, item.count)
		line := fmt.Sprintf(itemFmt, label)
		if i == pp.cursor {
			content.WriteString(menuSelectedStyle.Render(">" + line))
		} else if item.dir == pp.current {
			// Highlight currently active filter
			content.WriteString(detailLabelStyle.Render(" " + line))
		} else {
			content.WriteString(menuItemStyle.Render(" " + line))
		}
		content.WriteString("\n")
	}

	return menuStyle.Render(strings.TrimRight(content.String(), "\n"))
}

// OverlayOn renders the picker on top of a base screen string.
func (pp ProjectPickerModel) OverlayOn(base string) string {
	rendered := pp.View()
	menuLines := strings.Split(rendered, "\n")
	baseLines := strings.Split(base, "\n")

	y := pp.y
	if y+len(menuLines) > len(baseLines) {
		y = len(baseLines) - len(menuLines)
	}
	if y < 0 {
		y = 0
	}

	x := pp.x
	padding := strings.Repeat(" ", x)

	for i, mLine := range menuLines {
		row := y + i
		if row >= 0 && row < len(baseLines) {
			newLine := padding + mLine
			baseW := lipgloss.Width(baseLines[row])
			newW := lipgloss.Width(newLine)
			if newW < baseW {
				newLine += strings.Repeat(" ", baseW-newW)
			}
			baseLines[row] = newLine
		}
	}

	return strings.Join(baseLines, "\n")
}
