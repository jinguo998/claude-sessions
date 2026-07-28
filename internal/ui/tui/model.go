package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	apparchive "github.com/jinguo998/claude-sessions/internal/app/archive"
	session "github.com/jinguo998/claude-sessions/internal/app/model"
	"github.com/jinguo998/claude-sessions/internal/app/ports"
	apppreview "github.com/jinguo998/claude-sessions/internal/app/preview"
	appscan "github.com/jinguo998/claude-sessions/internal/app/scan"
)

type viewState int

const (
	viewList viewState = iota
	viewPreview
	viewConfirmDelete
	viewContextMenu
	viewProjectPicker
	viewHelp
	viewStats
	viewTrash
	viewConfirmTrashDelete
	viewConfirmTrashEmpty
)

// Result holds the data main.go needs after the TUI exits.
type Result struct {
	Dir            string
	ID             string
	Fork           bool
	PermissionMode session.PermissionMode
	CdOnly         bool // just cd to project dir, don't resume
	Source         session.Source
}

type Services struct {
	Scan      *appscan.Repository
	Preview   *apppreview.Service
	Archive   *apparchive.Service
	Clipboard ports.Clipboard
	Editor    ports.Editor
	Sources   session.SourceRegistry
}

// Model is the Bubble Tea model for the session browser.
type Model struct {
	list            ListModel
	view            viewState
	previewReturn   viewState
	width           int
	height          int
	stats           StatsModel
	preview         PreviewModel
	result          *Result
	deleteIdx       int
	trashDeleteItem apparchive.Item
	sidePreview     SidePreviewModel
	contextMenu     ContextMenuModel
	projectPicker   ProjectPickerModel
	trash           TrashModel
	flash           string // transient status message shown briefly at the bottom
	mouseEnabled    bool
	scanSeq         uint64
	previewSeq      uint64
	sidePreviewSeq  uint64
	sidePreviewReq  uint64
	services        Services
}

func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Result returns the resume information after the TUI exits.
func (m Model) Result() Result {
	if m.result != nil {
		return *m.result
	}
	return Result{}
}

// InitialModel creates the initial Model for the Bubble Tea program.
func InitialModel(services ...Services) Model {
	svc := Services{}
	if len(services) > 0 {
		svc = services[0]
	}
	sources := svc.Sources
	return Model{
		list:         NewListModel(sources),
		stats:        NewStatsModel(sources),
		preview:      NewPreviewModel(sources),
		sidePreview:  NewSidePreviewModel(),
		contextMenu:  NewContextMenuModel(sources),
		trash:        NewTrashModel(sources),
		mouseEnabled: true,
		scanSeq:      1,
		services:     svc,
	}
}

func (m Model) Init() tea.Cmd {
	return scanSessionsCmd(m.scanSeq, m.services.Scan)
}

func scanSessionsCmd(token uint64, scanner *appscan.Repository) tea.Cmd {
	return func() tea.Msg {
		if scanner == nil {
			return SessionsLoadedMsg{Token: token}
		}
		result := scanner.Scan(context.Background())
		return SessionsLoadedMsg{Token: token, Sessions: result.Sessions, Warnings: result.Warnings, Err: result.Err}
	}
}

const (
	splitMinWidth            = 120 // minimum terminal width for side-by-side layout
	sidePreviewDebounceDelay = 80 * time.Millisecond
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case SessionsLoadedMsg:
		if msg.Token != 0 && msg.Token != m.scanSeq {
			traceEvent("sessions_loaded_stale", map[string]any{
				"msg_token":     msg.Token,
				"current_token": m.scanSeq,
				"session_count": len(msg.Sessions),
			})
			return m, nil
		}
		m.list = m.list.SetSessions(msg.Sessions)
		if msg.Err != nil {
			m.flash = "Scan failed: " + msg.Err.Error()
		} else if len(msg.Warnings) > 0 && len(msg.Sessions) == 0 {
			m.flash = "Scan warnings: " + msg.Warnings[0].Message
		}
		var cmd tea.Cmd
		if m.wideMode() {
			cmd = m.loadSidePreviewNow()
		}
		return m, cmd

	case SidePreviewLoadMsg:
		if msg.Token != m.sidePreviewSeq || !m.wideMode() {
			traceEvent("side_preview_debounce_drop", map[string]any{
				"msg_token":     msg.Token,
				"current_token": m.sidePreviewSeq,
				"wide_mode":     m.wideMode(),
			})
			return m, nil
		}
		traceEvent("side_preview_debounce_fire", map[string]any{
			"token": msg.Token,
		})
		return m, m.loadSidePreviewIfNeeded()

	case SidePreviewLoadedMsg:
		var cmd tea.Cmd
		m.sidePreview, cmd = m.sidePreview.Update(msg)
		return m, cmd

	case PreviewLoadedMsg:
		if msg.Token != m.previewSeq || msg.SessionID != m.preview.session.ID {
			return m, nil
		}
		if msg.OK {
			m.preview = m.preview.ApplyLoaded(msg.Result, msg.PreserveMsgIndex, msg.ScrollBottom)
		} else {
			m.preview = m.preview.SetNoContent()
		}
		return m, nil

	case OpenContextMenuMsg:
		m.contextMenu = m.contextMenu.Open(msg.Session, msg.X, msg.Y)
		m.view = viewContextMenu
		return m, nil

	case MenuActionMsg:
		return m.handleMenuAction(msg)

	case MenuCloseMsg:
		m.view = viewList
		return m, nil

	case OpenProjectPickerMsg:
		m.projectPicker = m.projectPicker.Open(m.list.sessions, m.list.filterProj, 2, 2)
		m.view = viewProjectPicker
		return m, nil

	case ProjectSelectedMsg:
		m.list.filterProj = msg.ProjectDir
		m.list.applyFilter()
		m.list.updateListOffset()
		m.view = viewList
		var cmd tea.Cmd
		if m.wideMode() {
			cmd = m.loadSidePreviewNow()
		}
		return m, cmd

	case ProjectPickerCloseMsg:
		m.view = viewList
		return m, nil

	case OpenHelpMsg:
		m.view = viewHelp
		return m, nil

	case OpenStatsMsg:
		m.view = viewStats
		m.stats = m.stats.SetSize(m.width, m.height).Open(m.list.Filtered(), len(m.list.sessions), m.statsScopeSummary())
		return m, nil

	case StatsCloseMsg:
		m.view = viewList
		return m, nil

	case StatsPreviewMsg:
		return m, m.openPreview(msg.Session, viewStats)

	case StatsProjectFilterMsg:
		if msg.ProjectPath != "" {
			m.list.filterProj = msg.ProjectPath
			m.list.applyFilter()
			m.list.updateListOffset()
			m.stats = m.stats.SetSessions(m.list.Filtered(), len(m.list.sessions), m.statsScopeSummary())
			m.flash = "Filtered: " + session.Session{ProjectPath: msg.ProjectPath}.ProjectShortName()
			var cmd tea.Cmd = tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
			if m.wideMode() {
				cmd = tea.Batch(cmd, m.loadSidePreviewNow())
			}
			return m, cmd
		}
		return m, nil

	case StatsListFocusMsg:
		if m.list.focusSessionID(msg.Session.ID) {
			m.view = viewList
			m.flash = "Focused: " + truncateID(msg.Session.ID)
			clearCmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
			if m.wideMode() {
				return m, tea.Batch(clearCmd, m.loadSidePreviewNow())
			}
			return m, clearCmd
		}
		return m, nil

	case OpenTrashMsg:
		m.view = viewTrash
		m.trash = NewTrashModel(m.services.Sources).SetSize(m.width, m.height)
		return m, loadTrashCmd(m.services.Archive)

	case TrashLoadedMsg:
		var err error
		if msg.Err != "" {
			err = fmt.Errorf("%s", msg.Err)
		}
		m.trash = m.trash.SetSize(m.width, m.height).SetItems(msg.Items, err)
		return m, nil

	case TrashCloseMsg:
		m.view = viewList
		return m, nil

	case TrashRestoreMsg:
		if m.services.Archive == nil {
			m.flash = "Restore failed: archive service unavailable"
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
		}
		if err := m.services.Archive.Restore(context.Background(), msg.Item); err != nil {
			m.flash = "Restore failed: " + err.Error()
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
		}
		m.flash = "Restored: " + truncateID(msg.Item.Metadata.ID)
		return m, tea.Batch(
			m.startScan(),
			loadTrashCmd(m.services.Archive),
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} }),
		)

	case TrashPreviewMsg:
		return m, m.openPreview(archivedItemSession(msg.Item), viewTrash)

	case TrashDeleteMsg:
		m.trashDeleteItem = msg.Item
		m.view = viewConfirmTrashDelete
		return m, nil

	case TrashEmptyMsg:
		m.view = viewConfirmTrashEmpty
		return m, nil

	case HelpCloseMsg:
		m.view = viewList
		return m, nil

	case RefreshMsg:
		return m, m.startScan()

	case CopyIDMsg:
		id := msg.ID
		short := id
		if len(id) > 8 {
			short = id[:8]
		}
		m.flash = "Copied: " + short
		return m, tea.Batch(
			func() tea.Msg {
				if m.services.Clipboard != nil {
					_ = m.services.Clipboard.Copy(id)
				}
				return nil
			},
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} }),
		)

	case FlashMsg:
		m.flash = msg.Text
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })

	case FlashClearMsg:
		m.flash = ""
		return m, nil

	case OpenEditorMsg:
		if m.services.Editor == nil {
			return m, nil
		}
		return m, func() tea.Msg {
			_ = m.services.Editor.Open(msg.FilePath)
			return nil
		}

	case tea.WindowSizeMsg:
		wasWide := m.wideMode()
		m.width = msg.Width
		m.height = msg.Height
		m.list = m.list.SetSize(msg.Width, msg.Height).SetCompact(m.wideMode())
		m.stats = m.stats.SetSize(msg.Width, msg.Height)
		m.preview = m.preview.SetSize(msg.Width, msg.Height)
		m.trash = m.trash.SetSize(msg.Width, msg.Height)
		m.sidePreview = m.sidePreview.SetSize(msg.Height)
		isWide := m.wideMode()
		if !isWide && wasWide {
			m.invalidateSidePreviewLoads("resize_narrow")
			m.sidePreview = NewSidePreviewModel().SetSize(msg.Height)
			return m, nil
		}
		if isWide && !wasWide && m.list.loaded {
			return m, m.loadSidePreviewNow()
		}
		return m, nil

	case PreviewCloseMsg:
		m.view = m.previewReturn
		if m.view == viewPreview {
			m.view = viewList
		}
		m.previewReturn = viewList
		return m, nil

	// Messages emitted by ListModel
	case SessionSelectedMsg:
		m.result = &Result{Dir: msg.Session.ProjectPath, ID: msg.Session.ID, PermissionMode: normalizePermissionMode(msg.PermissionMode), Source: msg.Session.Source}
		return m, tea.Quit

	case SessionForkMsg:
		m.result = &Result{Dir: msg.Session.ProjectPath, ID: msg.Session.ID, Fork: true, PermissionMode: normalizePermissionMode(msg.PermissionMode), Source: msg.Session.Source}
		return m, tea.Quit

	case SessionPreviewMsg:
		return m, m.openPreview(msg.Session, viewList)

	case PreviewReloadMsg:
		m.preview.verbose = msg.Verbose
		return m, m.reloadPreview()

	case SessionDeleteMsg:
		m.deleteIdx = msg.Index
		m.view = viewConfirmDelete
		return m, nil

	case SessionArchiveUnsupportedMsg:
		label := m.services.Sources.Info(msg.Session.Source).Label
		m.flash = "Archive unsupported for " + label
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })

	case FilterChangedMsg:
		var cmd tea.Cmd
		if m.wideMode() {
			if msg.Debounce {
				cmd = m.debounceSidePreviewLoad()
			} else {
				cmd = m.loadSidePreviewNow()
			}
		}
		return m, cmd

	case tea.KeyMsg:
		if msg.String() == "M" {
			return m.toggleMouseMode()
		}
		switch m.view {
		case viewPreview:
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		case viewContextMenu:
			var cmd tea.Cmd
			m.contextMenu, cmd = m.contextMenu.Update(msg)
			return m, cmd
		case viewProjectPicker:
			var cmd tea.Cmd
			m.projectPicker, cmd = m.projectPicker.Update(msg)
			return m, cmd
		case viewConfirmDelete:
			return m.updateDeleteConfirm(msg)
		case viewConfirmTrashDelete:
			return m.updateTrashDeleteConfirm(msg)
		case viewConfirmTrashEmpty:
			return m.updateTrashEmptyConfirm(msg)
		case viewHelp:
			m.view = viewList // any key closes help
			return m, nil
		case viewStats:
			m.stats = m.stats.SetSessions(m.list.Filtered(), len(m.list.sessions), m.statsScopeSummary())
			var cmd tea.Cmd
			m.stats, cmd = m.stats.Update(msg)
			return m, cmd
		case viewTrash:
			var cmd tea.Cmd
			m.trash, cmd = m.trash.Update(msg)
			return m, cmd
		default: // viewList (including searching)
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

	case tea.MouseMsg:
		switch m.view {
		case viewContextMenu:
			var cmd tea.Cmd
			m.contextMenu, cmd = m.contextMenu.Update(msg)
			return m, cmd
		case viewProjectPicker:
			var cmd tea.Cmd
			m.projectPicker, cmd = m.projectPicker.Update(msg)
			return m, cmd
		case viewPreview:
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		case viewTrash:
			var cmd tea.Cmd
			m.trash, cmd = m.trash.Update(msg)
			return m, cmd
		case viewHelp:
			return m, nil
		case viewStats:
			m.stats = m.stats.SetSessions(m.list.Filtered(), len(m.list.sessions), m.statsScopeSummary())
			var cmd tea.Cmd
			m.stats, cmd = m.stats.Update(msg)
			return m, cmd
		case viewConfirmDelete:
			return m, nil
		case viewConfirmTrashDelete, viewConfirmTrashEmpty:
			return m, nil
		default: // viewList
			return m.handleMouse(msg)
		}
	}
	return m, nil
}

func (m Model) toggleMouseMode() (Model, tea.Cmd) {
	clearCmd := tea.Tick(3*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
	if m.mouseEnabled {
		m.mouseEnabled = false
		m.flash = "Mouse off: drag to select text; press M to restore"
		return m, tea.Batch(tea.DisableMouse, clearCmd)
	}
	m.mouseEnabled = true
	m.flash = "Mouse on: click and scroll enabled"
	return m, tea.Batch(tea.EnableMouseCellMotion, clearCmd)
}

// handleMouse routes mouse events in list view, splitting between
// left panel (list) and right panel (side preview) in wide mode.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Scroll wheel on right panel in wide mode → side preview
	if m.wideMode() && msg.X > m.width/2 {
		if msg.Button == tea.MouseButtonWheelUp {
			beforeScroll := m.sidePreview.scroll
			m.sidePreview = m.sidePreview.ScrollUp(sidePreviewScrollStep)
			if m.sidePreview.AtTop() && beforeScroll <= sidePreviewScrollStep {
				return m, m.loadFullSidePreviewNow()
			}
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.sidePreview = m.sidePreview.ScrollDown(sidePreviewScrollStep)
			return m, nil
		}
		// Ignore other clicks on right panel
		return m, nil
	}

	// Everything else goes to the list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.deleteIdx >= 0 && m.deleteIdx < len(m.list.Filtered()) {
			sess := m.list.Filtered()[m.deleteIdx]
			if m.services.Archive == nil {
				m.view = viewList
				m.flash = "Archive failed: archive service unavailable"
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
			}
			_, err := m.services.Archive.Archive(context.Background(), sess.Domain())
			m.view = viewList
			if err != nil {
				m.flash = "Archive failed: " + err.Error()
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
			}
			short := sess.ID
			if len(short) > 8 {
				short = short[:8]
			}
			m.flash = "Archived: " + short
			return m, tea.Batch(
				m.startScan(),
				tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} }),
			)
		}
		m.view = viewList
		return m, nil
	default:
		m.view = viewList
		return m, nil
	}
}

func (m Model) updateTrashDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.services.Archive == nil {
			m.view = viewTrash
			m.flash = "Delete failed: archive service unavailable"
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
		}
		if err := m.services.Archive.Delete(context.Background(), m.trashDeleteItem); err != nil {
			m.view = viewTrash
			m.flash = "Delete failed: " + err.Error()
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
		}
		m.view = viewTrash
		m.flash = "Deleted: " + truncateID(m.trashDeleteItem.Metadata.ID)
		return m, tea.Batch(
			loadTrashCmd(m.services.Archive),
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} }),
		)
	default:
		m.view = viewTrash
		return m, nil
	}
}

func (m Model) updateTrashEmptyConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		items := m.trash.Items()
		if m.services.Archive == nil {
			m.view = viewTrash
			m.flash = "Empty trash failed: archive service unavailable"
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
		}
		if err := m.services.Archive.DeleteAll(context.Background(), items); err != nil {
			m.view = viewTrash
			m.flash = "Empty trash failed: " + err.Error()
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
		}
		m.view = viewTrash
		m.flash = "Trash emptied"
		return m, tea.Batch(
			loadTrashCmd(m.services.Archive),
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} }),
		)
	default:
		m.view = viewTrash
		return m, nil
	}
}

func (m Model) handleMenuAction(msg MenuActionMsg) (tea.Model, tea.Cmd) {
	sess := msg.Session
	switch msg.Action {
	case ActionResumeSafe:
		m.result = &Result{Dir: sess.ProjectPath, ID: sess.ID, PermissionMode: session.PermissionModeSafe, Source: sess.Source}
		return m, tea.Quit
	case ActionResumeFast:
		m.result = &Result{Dir: sess.ProjectPath, ID: sess.ID, PermissionMode: m.services.Sources.DefaultPermissionMode(sess.Source), Source: sess.Source}
		return m, tea.Quit
	case ActionFork:
		m.result = &Result{Dir: sess.ProjectPath, ID: sess.ID, Fork: true, PermissionMode: m.services.Sources.DefaultPermissionMode(sess.Source), Source: sess.Source}
		return m, tea.Quit
	case ActionCd:
		m.result = &Result{Dir: sess.ProjectPath, ID: sess.ID, CdOnly: true, PermissionMode: session.PermissionModeSafe, Source: sess.Source}
		return m, tea.Quit
	case ActionPreview:
		return m, m.openPreview(sess, viewList)
	case ActionDelete:
		if !m.services.Sources.SupportsArchive(sess.Source) {
			label := m.services.Sources.Info(sess.Source).Label
			m.flash = "Archive unsupported for " + label
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return FlashClearMsg{} })
		}
		m.deleteIdx = m.list.Cursor()
		m.view = viewConfirmDelete
		return m, nil
	}
	m.view = viewList
	return m, nil
}

func normalizePermissionMode(mode session.PermissionMode) session.PermissionMode {
	if mode == "" {
		return session.PermissionModeSafe
	}
	return mode
}

func (m Model) wideMode() bool {
	return m.width >= splitMinWidth
}

func (m *Model) startScan() tea.Cmd {
	m.scanSeq++
	traceEvent("sessions_scan_start", map[string]any{
		"token": m.scanSeq,
	})
	return scanSessionsCmd(m.scanSeq, m.services.Scan)
}

func (m *Model) openPreview(sess session.Session, returnView viewState) tea.Cmd {
	title := fmt.Sprintf("%s \u2014 %s", sess.FirstMsg, sess.ProjectShortName())
	m.previewSeq++
	token := m.previewSeq
	m.preview = m.preview.SetLoading(title, sess)
	m.previewReturn = returnView
	m.view = viewPreview
	return loadPreviewContentCmd(m.services.Preview, token, sess, m.width-4, m.preview.verbose, m.preview.markdown, 0, true)
}

func (m *Model) reloadPreview() tea.Cmd {
	if m.preview.filePath == "" {
		return nil
	}
	m.previewSeq++
	token := m.previewSeq
	msgIdx := m.preview.CurrentMessageIndex()
	sess := m.preview.session
	m.preview = m.preview.SetLoading(m.preview.title, sess)
	return loadPreviewContentCmd(m.services.Preview, token, sess, m.width-4, m.preview.verbose, m.preview.markdown, msgIdx, false)
}

func loadPreviewContentCmd(loader *apppreview.Service, token uint64, sess session.Session, width int, verbose, markdown bool, msgIdx int, scrollBottom bool) tea.Cmd {
	return func() tea.Msg {
		result, ok := LoadPreviewContent(loader, sess, width, verbose, markdown)
		return PreviewLoadedMsg{
			Token:            token,
			SessionID:        sess.ID,
			Result:           result,
			OK:               ok,
			PreserveMsgIndex: msgIdx,
			ScrollBottom:     scrollBottom,
		}
	}
}

func archivedItemSession(item apparchive.Item) session.Session {
	title := item.Metadata.Title
	if title == "" {
		title = filepath.Base(item.Metadata.OriginalFilePath)
	}
	return session.Session{
		ID:          item.Metadata.ID,
		Source:      item.Metadata.Source,
		FilePath:    item.SessionFile,
		ProjectPath: item.Metadata.ProjectPath,
		FirstMsg:    title,
		Title:       item.Metadata.Title,
	}
}

func (m *Model) loadSidePreviewNow() tea.Cmd {
	m.sidePreviewSeq++
	traceEvent("side_preview_load_now", map[string]any{
		"seq": m.sidePreviewSeq,
	})
	return m.loadSidePreviewIfNeeded()
}

func (m *Model) invalidateSidePreviewLoads(reason string) {
	m.sidePreviewSeq++
	m.sidePreviewReq++
	latestSidePreviewToken.Store(m.sidePreviewReq)
	traceEvent("side_preview_invalidate", map[string]any{
		"reason": reason,
		"seq":    m.sidePreviewSeq,
		"req":    m.sidePreviewReq,
	})
}

func (m *Model) debounceSidePreviewLoad() tea.Cmd {
	m.sidePreviewSeq++
	token := m.sidePreviewSeq
	traceEvent("side_preview_debounce_schedule", map[string]any{
		"token":    token,
		"delay_ms": traceDurationMS(sidePreviewDebounceDelay),
	})
	return tea.Tick(sidePreviewDebounceDelay, func(time.Time) tea.Msg {
		return SidePreviewLoadMsg{Token: token}
	})
}

func (m *Model) loadSidePreviewIfNeeded() tea.Cmd {
	sess, ok := m.list.SelectedSession()
	if !ok {
		m.invalidateSidePreviewLoads("no_selected_session")
		traceEvent("side_preview_load_skip", map[string]any{
			"reason": "no_selected_session",
		})
		m.sidePreview = NewSidePreviewModel()
		return nil
	}
	if !m.sidePreview.NeedsReload(sess.ID) {
		traceEvent("side_preview_load_skip", map[string]any{
			"reason":      "same_session",
			"session_id":  sess.ID,
			"loading":     m.sidePreview.loading,
			"request_tok": m.sidePreview.requestToken,
		})
		return nil
	}
	previewWidth := m.width - 4
	if m.wideMode() {
		previewWidth = m.width/2 - 6
	}
	var cmd tea.Cmd
	m.sidePreviewReq++
	traceEvent("side_preview_load_issue", map[string]any{
		"session_id": sess.ID,
		"token":      m.sidePreviewReq,
		"file_path":  sess.FilePath,
		"source":     string(sess.Source),
		"width":      previewWidth,
	})
	m.sidePreview, cmd = m.sidePreview.LoadSession(m.services.Preview, sess, previewWidth, m.sidePreviewReq)
	m.sidePreview = m.sidePreview.SetSize(m.height)
	return cmd
}

func (m *Model) loadFullSidePreviewNow() tea.Cmd {
	if !m.sidePreview.CanLoadMore() {
		return nil
	}
	previewWidth := m.width - 4
	if m.wideMode() {
		previewWidth = m.width/2 - 6
	}
	m.sidePreviewReq++
	traceEvent("side_preview_full_load_issue", map[string]any{
		"session_id": m.sidePreview.sessionID,
		"token":      m.sidePreviewReq,
		"file_path":  m.sidePreview.filePath,
		"source":     string(m.sidePreview.source),
		"width":      previewWidth,
	})
	var cmd tea.Cmd
	m.sidePreview, cmd = m.sidePreview.LoadFullSession(m.services.Preview, previewWidth, m.sidePreviewReq)
	m.sidePreview = m.sidePreview.SetSize(m.height)
	return cmd
}
