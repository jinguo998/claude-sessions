package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	apparchive "github.com/jinguo998/claude-sessions/internal/app/archive"
	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

type TrashModel struct {
	items         []apparchive.Item
	cursor        int
	width         int
	height        int
	loading       bool
	err           string
	sources       session.SourceRegistry
	lastClickTime time.Time
	lastClickIdx  int
}

const trashRowStart = 2

func NewTrashModel(sources ...session.SourceRegistry) TrashModel {
	registry := session.SourceRegistry{}
	if len(sources) > 0 {
		registry = sources[0]
	}
	return TrashModel{loading: true, sources: registry, lastClickIdx: -1}
}

func (t TrashModel) SetSize(w, h int) TrashModel {
	t.width = w
	t.height = h
	return t
}

func (t TrashModel) SetItems(items []apparchive.Item, err error) TrashModel {
	t.items = items
	t.err = ""
	t.loading = false
	if err != nil {
		t.err = err.Error()
	}
	if t.cursor >= len(t.items) {
		if len(t.items) == 0 {
			t.cursor = 0
		} else {
			t.cursor = len(t.items) - 1
		}
	}
	return t
}

func (t TrashModel) Selected() (apparchive.Item, bool) {
	if t.cursor >= 0 && t.cursor < len(t.items) {
		return t.items[t.cursor], true
	}
	return apparchive.Item{}, false
}

func (t TrashModel) Items() []apparchive.Item {
	return append([]apparchive.Item(nil), t.items...)
}

func (t TrashModel) Update(msg tea.Msg) (TrashModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
			}
			return t, nil
		case "down", "j":
			if t.cursor < len(t.items)-1 {
				t.cursor++
			}
			return t, nil
		case "enter", "r":
			if item, ok := t.Selected(); ok {
				return t, func() tea.Msg { return TrashRestoreMsg{Item: item} }
			}
			return t, nil
		case "p":
			if item, ok := t.Selected(); ok {
				return t, func() tea.Msg { return TrashPreviewMsg{Item: item} }
			}
			return t, nil
		case "x":
			if item, ok := t.Selected(); ok {
				return t, func() tea.Msg { return TrashDeleteMsg{Item: item} }
			}
			return t, nil
		case "D":
			if len(t.items) > 0 {
				return t, func() tea.Msg { return TrashEmptyMsg{} }
			}
			return t, nil
		case "esc", "q", "T":
			return t, func() tea.Msg { return TrashCloseMsg{} }
		}
	case tea.MouseMsg:
		return t.updateMouse(msg)
	}
	return t, nil
}

func (t TrashModel) updateMouse(msg tea.MouseMsg) (TrashModel, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if t.cursor > 0 {
			t.cursor--
		}
		return t, nil
	case tea.MouseButtonWheelDown:
		if t.cursor < len(t.items)-1 {
			t.cursor++
		}
		return t, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return t, nil
	}
	idx := msg.Y - trashRowStart
	if idx < 0 || idx >= len(t.items) {
		return t, nil
	}

	now := time.Now()
	if idx == t.lastClickIdx && now.Sub(t.lastClickTime) < doubleClickThreshold {
		t.cursor = idx
		t.lastClickIdx = -1
		return t, func() tea.Msg { return TrashPreviewMsg{Item: t.items[idx]} }
	}

	t.cursor = idx
	t.lastClickIdx = idx
	t.lastClickTime = now
	return t, nil
}

func (t TrashModel) View() string {
	titleText := "  Trash"
	title := titleBarStyle.Render(titleText + strings.Repeat(" ", max(t.width-lipglossWidth(titleText)-4, 1)))

	if t.loading {
		return title + "\n\n" + loadingStyle.Render("Loading archived sessions...")
	}
	if t.err != "" {
		return title + "\n\n" + confirmStyle.Render("  "+t.err) + "\n\n" + dimStyle.Render("  Press q to close")
	}
	if len(t.items) == 0 {
		return title + "\n\n" + emptyStyle.Render("No archived sessions") + "\n\n" + dimStyle.Render("  Press q to close")
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")

	for i, item := range t.items {
		label := truncateToWidth(itemLabel(item), max(t.width-34, 16))
		line := fmt.Sprintf(" %s  %-18s  %s",
			archiveSourceBadge(t.sources, item.Metadata.Source),
			padToWidth(itemArchivedAt(item), 18),
			label,
		)
		if i == t.cursor {
			b.WriteString(selectedTrashStyle.Render("> " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}

	if item, ok := t.Selected(); ok {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(strings.Repeat("─", max(30, min(t.width-4, 80)))))
		b.WriteString("\n")
		b.WriteString(" " + detailLabelStyle.Render("Restore to: ") + detailValueStyle.Render(item.Metadata.OriginalFilePath))
		b.WriteString("\n")
		if item.Metadata.ProjectPath != "" {
			b.WriteString(" " + detailLabelStyle.Render("Project: ") + detailValueStyle.Render(item.Metadata.ProjectPath))
			b.WriteString("\n")
		}
		if item.Metadata.Title != "" {
			b.WriteString(" " + detailLabelStyle.Render("Title: ") + detailValueStyle.Render(item.Metadata.Title))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(renderHelpBar([][2]string{
		{"j/k", "Move"}, {"p", "Preview"}, {"Enter/r", "Restore"}, {"x", "Delete"}, {"D", "Empty"}, {"q", "Back"},
	}))
	return b.String()
}

func itemLabel(item apparchive.Item) string {
	switch {
	case item.Metadata.Title != "":
		return item.Metadata.Title
	case item.Metadata.OriginalFilePath != "":
		return filepath.Base(item.Metadata.OriginalFilePath)
	default:
		return item.Metadata.ID
	}
}

func itemArchivedAt(item apparchive.Item) string {
	if item.Metadata.ArchivedAt == "" {
		return "unknown time"
	}
	ts, err := time.Parse(time.RFC3339, item.Metadata.ArchivedAt)
	if err != nil {
		return item.Metadata.ArchivedAt
	}
	return ts.Local().Format("01-02 15:04")
}

func archiveSourceBadge(sources session.SourceRegistry, src session.Source) string {
	info := sources.Info(src)
	return sourceBadgeStyle(info).Render(info.Badge)
}

func lipglossWidth(s string) int {
	return displayWidth(s)
}

func loadTrashCmd(service *apparchive.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return TrashLoadedMsg{}
		}
		items, err := service.List(context.Background())
		if err != nil {
			return TrashLoadedMsg{Err: err.Error()}
		}
		return TrashLoadedMsg{Items: items}
	}
}
