package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

// MenuItem represents a single entry in the context menu.
type MenuItem struct {
	Label  string
	Action string
}

// ContextMenuModel is an independent Bubble Tea sub-component for a context menu overlay.
type ContextMenuModel struct {
	items   []MenuItem
	cursor  int
	x, y    int
	session session.Session
	sources session.SourceRegistry
}

// Default menu items and dimensions (moved from view.go).
var baseMenuItems = []MenuItem{
	{Label: "Resume", Action: ActionResumeFast},
	{Label: "Fork", Action: ActionFork},
	{Label: "cd to project", Action: ActionCd},
	{Label: "Preview", Action: ActionPreview},
	{Label: "Archive", Action: ActionDelete},
}
var safeResumeMenuItem = MenuItem{Label: "Resume (safe)", Action: ActionResumeSafe}

const menuWidth = 17 // visible width of each menu line (matches longest label + padding)

func menuItemsForSession(sources session.SourceRegistry, sess session.Session) []MenuItem {
	items := make([]MenuItem, 0, len(baseMenuItems)+1)
	items = append(items, baseMenuItems[0])
	if sources.SupportsSafeResumeAction(sess.Source) {
		items = append(items, safeResumeMenuItem)
	}
	for _, item := range baseMenuItems[1:] {
		if item.Action == ActionDelete && !sources.SupportsArchive(sess.Source) {
			continue
		}
		items = append(items, item)
	}
	return items
}

// NewContextMenuModel creates a ContextMenuModel initialised with the default menu items.
func NewContextMenuModel(sources ...session.SourceRegistry) ContextMenuModel {
	registry := session.SourceRegistry{}
	if len(sources) > 0 {
		registry = sources[0]
	}
	defaultSource := session.Source("")
	if all := registry.All(); len(all) > 0 {
		defaultSource = all[0].Source
	}
	return ContextMenuModel{
		items:   menuItemsForSession(registry, session.Session{Source: defaultSource}),
		sources: registry,
	}
}

// Open prepares the menu for display at the given screen position for the given session.
func (c ContextMenuModel) Open(sess session.Session, x, y int) ContextMenuModel {
	c.session = sess
	c.items = menuItemsForSession(c.sources, sess)
	c.x = x
	c.y = y
	c.cursor = 0
	return c
}

// Update handles keyboard and mouse input for the context menu.
// It returns Cmds that emit MenuActionMsg or MenuCloseMsg to the parent.
func (c ContextMenuModel) Update(msg tea.Msg) (ContextMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if c.cursor > 0 {
				c.cursor--
			}
			return c, nil
		case "down", "j":
			if c.cursor < len(c.items)-1 {
				c.cursor++
			}
			return c, nil
		case "enter":
			item := c.items[c.cursor]
			sess := c.session
			return c, func() tea.Msg {
				return MenuActionMsg{Action: item.Action, Session: sess}
			}
		case "esc", "q":
			return c, func() tea.Msg { return MenuCloseMsg{} }
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Check if click is inside the menu item area
			// Menu layout: border(1) + title(1) + divider(1) + items + border(1)
			menuItemTop := c.y + 1 + 2 // +1 border, +2 title+divider
			menuItemBot := menuItemTop + len(c.items)
			menuLeft := c.x
			menuRight := menuLeft + menuWidth + 4
			if msg.Y >= menuItemTop && msg.Y < menuItemBot && msg.X >= menuLeft && msg.X < menuRight {
				c.cursor = msg.Y - menuItemTop
				item := c.items[c.cursor]
				sess := c.session
				return c, func() tea.Msg {
					return MenuActionMsg{Action: item.Action, Session: sess}
				}
			}
			// Click outside menu -> close
			return c, func() tea.Msg { return MenuCloseMsg{} }
		}
	}
	return c, nil
}

// View renders the bordered context menu box with a session title header.
func (c ContextMenuModel) View() string {
	var menuContent strings.Builder

	// Session title header
	title := c.session.FirstMsg
	if title == "" {
		title = truncateID(c.session.ID)
	}
	maxTitleW := menuWidth - 2
	if displayWidth(title) > maxTitleW {
		title = truncateToWidth(title, maxTitleW)
	}
	menuContent.WriteString(menuTitleStyle.Render(title))
	menuContent.WriteString("\n")
	menuContent.WriteString(menuDividerStyle.Render(strings.Repeat("─", menuWidth)))
	menuContent.WriteString("\n")

	itemFmt := fmt.Sprintf(" %%-%ds ", menuWidth-4) // pad to menuWidth minus borders
	for i, item := range c.items {
		line := fmt.Sprintf(itemFmt, item.Label)
		if i == c.cursor {
			menuContent.WriteString(menuSelectedStyle.Render(">" + line))
		} else {
			menuContent.WriteString(menuItemStyle.Render(" " + line))
		}
		menuContent.WriteString("\n")
	}
	return menuStyle.Render(strings.TrimRight(menuContent.String(), "\n"))
}

// OverlayOn renders the context menu on top of a base screen string by
// replacing whole lines at position (x, y). Each replaced line is padded
// to match the original line's visual width so no black gaps appear.
func (c ContextMenuModel) OverlayOn(base string) string {
	rendered := c.View()
	menuLines := strings.Split(rendered, "\n")

	baseLines := strings.Split(base, "\n")

	// Clamp menu position
	y := c.y
	if y+len(menuLines) > len(baseLines) {
		y = len(baseLines) - len(menuLines)
	}
	if y < 0 {
		y = 0
	}

	x := c.x
	padding := strings.Repeat(" ", x)

	// Replace lines where the menu appears, padding to original width
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
