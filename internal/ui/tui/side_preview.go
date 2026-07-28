package tui

import (
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
	apppreview "github.com/jinguo998/claude-sessions/internal/app/preview"
)

var latestSidePreviewToken atomic.Uint64
var sidePreviewFullRenderSem = make(chan struct{}, 1)

// SidePreviewModel is an extracted Bubble Tea sub-component for the
// right-hand side preview panel shown in split (wide) mode.
type SidePreviewModel struct {
	sessionID    string
	requestToken uint64
	content      string   // rendered preview content
	lines        []string // content split by newline (cached)
	scroll       int      // scroll offset
	height       int
	loading      bool
	loadingMore  bool
	complete     bool
	filePath     string
	source       session.Source
	session      session.Session
	width        int
	// Cached session metadata for View rendering.
	title string // first message of the loaded session
	path  string // project path of the loaded session
}

// NewSidePreviewModel returns a zero-value SidePreviewModel.
func NewSidePreviewModel() SidePreviewModel {
	return SidePreviewModel{}
}

// Update processes incoming messages for the side preview.
func (s SidePreviewModel) Update(msg tea.Msg) (SidePreviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SidePreviewLoadedMsg:
		start := time.Now()
		accepted := msg.SessionID == s.sessionID && msg.Token == s.requestToken
		if accepted {
			s.content = msg.Content
			s.lines = strings.Split(msg.Content, "\n")
			s.loading = false
			s.loadingMore = false
			s.complete = msg.Complete
			if msg.PreserveTailLines > 0 {
				s.scroll = len(s.lines) - msg.PreserveTailLines
			} else {
				// Scroll to bottom (most recent messages)
				s.scroll = len(s.lines)
			}
			s = s.clampScroll()
		}
		traceEvent("side_preview_update", map[string]any{
			"accepted":           accepted,
			"msg_session_id":     msg.SessionID,
			"current_session_id": s.sessionID,
			"msg_token":          msg.Token,
			"current_token":      s.requestToken,
			"loading":            s.loading,
			"loading_more":       s.loadingMore,
			"complete":           s.complete,
			"content_bytes":      len(s.content),
			"content_lines":      len(s.lines),
			"update_ms":          traceDurationMS(time.Since(start)),
		})
	}
	return s, nil
}

// View renders the side preview panel content (title, path, and scrollable preview).
// The caller is responsible for combining this into the split layout.
func (s SidePreviewModel) View(rightWidth int) string {
	var traceStart time.Time
	if traceEnabled() {
		traceStart = time.Now()
	}
	if s.sessionID == "" {
		out := emptyStyle.Render("Select a session to preview")
		if !traceStart.IsZero() {
			traceEvent("side_preview_view", map[string]any{
				"empty":       true,
				"right_width": rightWidth,
				"bytes":       len(out),
				"lines":       strings.Count(out, "\n") + 1,
				"view_ms":     traceDurationMS(time.Since(traceStart)),
			})
		}
		return out
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render(" " + truncateToWidth(s.title, rightWidth-4) + " "))
	b.WriteString("\n")
	b.WriteString(pathStyle.Render(" " + truncateToWidth(s.path, rightWidth-2)))
	b.WriteString("\n")
	b.WriteString(statsMutedStyle.Render(" " + truncateToWidth(s.statusText(), rightWidth-2)))
	b.WriteString("\n\n")

	// Render preview content with scroll support
	if len(s.lines) > 0 {
		maxLines := s.height - splitPreviewPadding
		if maxLines < 1 {
			maxLines = 1
		}
		start := s.scroll
		if start > len(s.lines)-maxLines {
			start = len(s.lines) - maxLines
		}
		if start < 0 {
			start = 0
		}
		shown := 0
		for i := start; i < len(s.lines) && shown < maxLines; i++ {
			b.WriteString(s.lines[i])
			b.WriteString("\n")
			shown++
		}
	}

	out := b.String()
	if !traceStart.IsZero() {
		traceEvent("side_preview_view", map[string]any{
			"empty":        false,
			"session_id":   s.sessionID,
			"token":        s.requestToken,
			"loading":      s.loading,
			"loading_more": s.loadingMore,
			"complete":     s.complete,
			"right_width":  rightWidth,
			"bytes":        len(out),
			"lines":        strings.Count(out, "\n") + 1,
			"content_len":  len(s.content),
			"line_count":   len(s.lines),
			"scroll":       s.scroll,
			"height":       s.height,
			"view_ms":      traceDurationMS(time.Since(traceStart)),
		})
	}
	return out
}

// LoadSession sets loading state and returns an async Cmd that parses the
// session file and sends a SidePreviewLoadedMsg when done.
func (s SidePreviewModel) LoadSession(loader *apppreview.Service, sess session.Session, width int, token uint64) (SidePreviewModel, tea.Cmd) {
	latestSidePreviewToken.Store(token)
	traceEvent("side_preview_load_start", map[string]any{
		"session_id":   sess.ID,
		"token":        token,
		"file_path":    sess.FilePath,
		"project_path": sess.ProjectPath,
		"source":       string(sess.Source),
		"width":        width,
		"first_msg":    sess.FirstMsg,
	})
	s.loading = true
	s.loadingMore = false
	s.complete = false
	s.sessionID = sess.ID
	s.requestToken = token
	s.scroll = 0
	s.content = dimStyle.Render("Loading...")
	s.lines = strings.Split(s.content, "\n")
	s.title = sess.FirstMsg
	s.path = sess.ProjectPath
	s.filePath = sess.FilePath
	s.source = sess.Source
	s.session = sess
	s.width = width

	id := sess.ID
	filePath := sess.FilePath
	previewWidth := width
	source := sess.Source

	cmd := func() tea.Msg {
		start := time.Now()
		traceEvent("side_preview_cmd_start", map[string]any{
			"session_id": id,
			"token":      token,
			"file_path":  filePath,
			"source":     string(source),
			"width":      previewWidth,
		})
		if !sidePreviewTokenIsCurrent(token) {
			return staleSidePreviewLoadedMsg(id, token, start, "before_render")
		}

		if result, ok := LoadSidePreviewContent(loader, sess, previewWidth); ok {
			if !sidePreviewTokenIsCurrent(token) {
				return staleSidePreviewLoadedMsg(id, token, start, "after_render")
			}
			traceEvent("side_preview_cmd_done", map[string]any{
				"session_id":    id,
				"token":         token,
				"ok":            true,
				"duration_ms":   traceDurationMS(time.Since(start)),
				"content_bytes": len(result.content),
				"content_lines": strings.Count(result.content, "\n") + 1,
			})
			return SidePreviewLoadedMsg{Token: token, SessionID: id, Content: result.content}
		}
		if !sidePreviewTokenIsCurrent(token) {
			return staleSidePreviewLoadedMsg(id, token, start, "after_empty_render")
		}
		traceEvent("side_preview_cmd_done", map[string]any{
			"session_id":  id,
			"token":       token,
			"ok":          false,
			"duration_ms": traceDurationMS(time.Since(start)),
		})
		return SidePreviewLoadedMsg{Token: token, SessionID: id, Content: "(no previewable content)"}
	}
	return s, cmd
}

// LoadFullSession keeps the current tail preview visible while asynchronously
// replacing it with a full-session side preview.
func (s SidePreviewModel) LoadFullSession(loader *apppreview.Service, width int, token uint64) (SidePreviewModel, tea.Cmd) {
	if !s.CanLoadMore() {
		return s, nil
	}
	latestSidePreviewToken.Store(token)
	s.requestToken = token
	s.loadingMore = true
	s.width = width

	id := s.sessionID
	filePath := s.filePath
	source := s.source
	sess := s.session
	tailLines := len(s.lines)

	traceEvent("side_preview_full_load_start", map[string]any{
		"session_id": id,
		"token":      token,
		"file_path":  filePath,
		"source":     string(source),
		"width":      width,
		"tail_lines": tailLines,
	})

	cmd := func() tea.Msg {
		start := time.Now()
		if !sidePreviewTokenIsCurrent(token) {
			return staleSidePreviewLoadedMsg(id, token, start, "before_full_render")
		}

		sidePreviewFullRenderSem <- struct{}{}
		defer func() { <-sidePreviewFullRenderSem }()

		if !sidePreviewTokenIsCurrent(token) {
			return staleSidePreviewLoadedMsg(id, token, start, "after_full_wait")
		}

		if result, ok := LoadFullSidePreviewContent(loader, sess, width); ok {
			if !sidePreviewTokenIsCurrent(token) {
				return staleSidePreviewLoadedMsg(id, token, start, "after_full_render")
			}
			traceEvent("side_preview_full_load_done", map[string]any{
				"session_id":    id,
				"token":         token,
				"ok":            true,
				"duration_ms":   traceDurationMS(time.Since(start)),
				"content_bytes": len(result.content),
				"content_lines": strings.Count(result.content, "\n") + 1,
			})
			return SidePreviewLoadedMsg{
				Token:             token,
				SessionID:         id,
				Content:           result.content,
				Complete:          true,
				PreserveTailLines: tailLines,
			}
		}

		if !sidePreviewTokenIsCurrent(token) {
			return staleSidePreviewLoadedMsg(id, token, start, "after_full_empty_render")
		}
		traceEvent("side_preview_full_load_done", map[string]any{
			"session_id":  id,
			"token":       token,
			"ok":          false,
			"duration_ms": traceDurationMS(time.Since(start)),
		})
		return SidePreviewLoadedMsg{
			Token:             token,
			SessionID:         id,
			Content:           s.content,
			Complete:          false,
			PreserveTailLines: tailLines,
		}
	}
	return s, cmd
}

func sidePreviewTokenIsCurrent(token uint64) bool {
	return token == latestSidePreviewToken.Load()
}

func staleSidePreviewLoadedMsg(id string, token uint64, start time.Time, stage string) tea.Msg {
	traceEvent("side_preview_cmd_stale", map[string]any{
		"session_id":  id,
		"token":       token,
		"latest":      latestSidePreviewToken.Load(),
		"stage":       stage,
		"duration_ms": traceDurationMS(time.Since(start)),
	})
	return SidePreviewLoadedMsg{Token: token, SessionID: id, Content: ""}
}

// SetSize updates the dimensions available for rendering.
func (s SidePreviewModel) SetSize(h int) SidePreviewModel {
	s.height = h
	return s.clampScroll()
}

// ScrollUp scrolls the preview up by n lines.
func (s SidePreviewModel) ScrollUp(n int) SidePreviewModel {
	s.scroll -= n
	return s.clampScroll()
}

// ScrollDown scrolls the preview down by n lines.
func (s SidePreviewModel) ScrollDown(n int) SidePreviewModel {
	s.scroll += n
	return s.clampScroll()
}

func (s SidePreviewModel) clampScroll() SidePreviewModel {
	maxScroll := s.maxScroll()
	if s.scroll > maxScroll {
		s.scroll = maxScroll
	}
	if s.scroll < 0 {
		s.scroll = 0
	}
	return s
}

func (s SidePreviewModel) maxScroll() int {
	maxLines := s.height - splitPreviewPadding
	if maxLines < 1 {
		maxLines = 1
	}
	maxScroll := len(s.lines) - maxLines
	if maxScroll < 0 {
		return 0
	}
	return maxScroll
}

func (s SidePreviewModel) AtTop() bool {
	return s.scroll <= 0
}

func (s SidePreviewModel) CanLoadMore() bool {
	return s.sessionID != "" && s.filePath != "" && !s.complete && !s.loading && !s.loadingMore
}

func (s SidePreviewModel) statusText() string {
	switch {
	case s.loading:
		return "Loading preview..."
	case s.loadingMore:
		return "Loading full history..."
	case s.complete:
		return "Full history  |  M select text"
	default:
		return "Tail preview  |  scroll up for full history  |  M select text"
	}
}

// NeedsReload returns true if the given session ID differs from the current
// panel. While a session is already loading, duplicate reloads for that same
// session only add competing background work and delay the newest selection.
func (s SidePreviewModel) NeedsReload(sessionID string) bool {
	return sessionID != s.sessionID
}
