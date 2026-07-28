package tui

import (
	apparchive "github.com/jinguo998/claude-sessions/internal/app/archive"
	session "github.com/jinguo998/claude-sessions/internal/app/model"
	appscan "github.com/jinguo998/claude-sessions/internal/app/scan"
)

// Action constants for MenuActionMsg.
const (
	ActionResumeSafe = "resume_safe"
	ActionResumeFast = "resume_fast"
	ActionFork       = "fork"
	ActionCd         = "cd"
	ActionPreview    = "preview"
	ActionDelete     = "delete"
)

// === Child → Parent messages ===

type SessionSelectedMsg struct {
	Session        session.Session
	PermissionMode session.PermissionMode
}
type SessionForkMsg struct {
	Session        session.Session
	PermissionMode session.PermissionMode
}
type SessionPreviewMsg struct{ Session session.Session }
type SessionDeleteMsg struct{ Index int }
type SessionArchiveUnsupportedMsg struct{ Session session.Session }
type FilterChangedMsg struct {
	Debounce bool
}
type OpenContextMenuMsg struct {
	Session session.Session
	X, Y    int
}
type PreviewCloseMsg struct{}
type PreviewReloadMsg struct{ Verbose bool }
type MenuActionMsg struct {
	Action  string
	Session session.Session
}
type MenuCloseMsg struct{}

type OpenProjectPickerMsg struct{}
type ProjectSelectedMsg struct{ ProjectDir string } // "" means All
type ProjectPickerCloseMsg struct{}
type OpenHelpMsg struct{}
type OpenStatsMsg struct{}
type StatsCloseMsg struct{}
type StatsPreviewMsg struct{ Session session.Session }
type StatsProjectFilterMsg struct{ ProjectPath string }
type StatsListFocusMsg struct{ Session session.Session }
type OpenTrashMsg struct{}
type HelpCloseMsg struct{}
type RefreshMsg struct{}
type CopyIDMsg struct{ ID string }
type OpenEditorMsg struct{ FilePath string }
type TrashLoadedMsg struct {
	Items []apparchive.Item
	Err   string
}
type TrashRestoreMsg struct{ Item apparchive.Item }
type TrashPreviewMsg struct{ Item apparchive.Item }
type TrashDeleteMsg struct{ Item apparchive.Item }
type TrashEmptyMsg struct{}
type TrashCloseMsg struct{}

func defaultResumeSelection(sources session.SourceRegistry, sess session.Session) SessionSelectedMsg {
	return SessionSelectedMsg{
		Session:        sess,
		PermissionMode: sources.DefaultPermissionMode(sess.Source),
	}
}

func safeResumeSelection(sess session.Session) SessionSelectedMsg {
	return SessionSelectedMsg{
		Session:        sess,
		PermissionMode: session.PermissionModeSafe,
	}
}

func forkSelection(sources session.SourceRegistry, sess session.Session) SessionForkMsg {
	return SessionForkMsg{
		Session:        sess,
		PermissionMode: sources.DefaultPermissionMode(sess.Source),
	}
}

// === Async messages ===

type SessionsLoadedMsg struct {
	Token    uint64
	Sessions []session.Session
	Warnings []appscan.Warning
	Err      error
}
type PreviewLoadedMsg struct {
	Token            uint64
	SessionID        string
	Result           previewResult
	OK               bool
	PreserveMsgIndex int
	ScrollBottom     bool
}
type SidePreviewLoadMsg struct {
	Token uint64
}
type SidePreviewLoadedMsg struct {
	Token             uint64
	SessionID         string
	Content           string
	Complete          bool
	PreserveTailLines int
}

// FlashMsg briefly displays a status message in the UI.
type FlashMsg struct{ Text string }

// FlashClearMsg clears the flash message.
type FlashClearMsg struct{}
