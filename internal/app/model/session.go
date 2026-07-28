package model

import (
	"fmt"
	"sort"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

type Source = domain.Source
type TokenUsage = domain.TokenUsage

const (
	SourceClaude   = domain.SourceClaude
	SourceCodex    = domain.SourceCodex
	SourceOpenCode = domain.SourceOpenCode
)

// Session is the application-facing session view model. It embeds the pure
// domain session and carries derived search corpus outside the domain model.
type Session struct {
	ID          string
	Source      Source
	ProjectDir  string
	ProjectPath string
	Title       string
	Client      string
	Origin      string
	Labels      []string
	Attributes  map[string]string
	FirstMsg    string
	LastMsg     string
	StartTime   time.Time
	LastTime    time.Time
	MsgCount    int
	ToolCount   int
	FileSize    int64
	FilePath    string
	Model       string
	TokenUsage  TokenUsage
	SearchText  string
}

func FromDomain(sess domain.Session, searchText string) Session {
	return Session{
		ID:          sess.ID,
		Source:      sess.Source,
		ProjectDir:  sess.ProjectDir,
		ProjectPath: sess.ProjectPath,
		Title:       sess.Title,
		Client:      sess.Client,
		Origin:      sess.Origin,
		Labels:      sess.Labels,
		Attributes:  sess.Attributes,
		FirstMsg:    sess.FirstMsg,
		LastMsg:     sess.LastMsg,
		StartTime:   sess.StartTime,
		LastTime:    sess.LastTime,
		MsgCount:    sess.MsgCount,
		ToolCount:   sess.ToolCount,
		FileSize:    sess.FileSize,
		FilePath:    sess.FilePath,
		Model:       sess.Model,
		TokenUsage:  sess.TokenUsage,
		SearchText:  searchText,
	}
}

func (s Session) Domain() domain.Session {
	return domain.Session{
		ID:          s.ID,
		Source:      s.Source,
		ProjectDir:  s.ProjectDir,
		ProjectPath: s.ProjectPath,
		Title:       s.Title,
		Client:      s.Client,
		Origin:      s.Origin,
		Labels:      s.Labels,
		Attributes:  s.Attributes,
		FirstMsg:    s.FirstMsg,
		LastMsg:     s.LastMsg,
		StartTime:   s.StartTime,
		LastTime:    s.LastTime,
		MsgCount:    s.MsgCount,
		ToolCount:   s.ToolCount,
		FileSize:    s.FileSize,
		FilePath:    s.FilePath,
		Model:       s.Model,
		TokenUsage:  s.TokenUsage,
	}
}

func ToDomainSessions(sessions []Session) []domain.Session {
	out := make([]domain.Session, len(sessions))
	for i, sess := range sessions {
		out[i] = sess.Domain()
	}
	return out
}

func (s Session) Duration() time.Duration {
	return s.Domain().Duration()
}

func (s Session) FormatDuration() string {
	d := s.Duration()
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

func (s Session) FormatSize() string {
	switch {
	case s.FileSize < 1024:
		return fmt.Sprintf("%dB", s.FileSize)
	case s.FileSize < 1024*1024:
		return fmt.Sprintf("%.0fKB", float64(s.FileSize)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(s.FileSize)/(1024*1024))
	}
}

func (s Session) TotalTokens() int {
	return s.TokenUsage.Total()
}

func (s Session) ProjectShortName() string {
	return s.Domain().ProjectShortName()
}

type SortByLastTime []Session

func (s SortByLastTime) Len() int           { return len(s) }
func (s SortByLastTime) Less(i, j int) bool { return s[i].LastTime.After(s[j].LastTime) }
func (s SortByLastTime) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type SortByMsgCount []Session

func (s SortByMsgCount) Len() int           { return len(s) }
func (s SortByMsgCount) Less(i, j int) bool { return s[i].MsgCount > s[j].MsgCount }
func (s SortByMsgCount) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type SortByToolCount []Session

func (s SortByToolCount) Len() int           { return len(s) }
func (s SortByToolCount) Less(i, j int) bool { return s[i].ToolCount > s[j].ToolCount }
func (s SortByToolCount) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type SortByTotalTokens []Session

func (s SortByTotalTokens) Len() int { return len(s) }
func (s SortByTotalTokens) Less(i, j int) bool {
	return s[i].TotalTokens() > s[j].TotalTokens()
}
func (s SortByTotalTokens) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func SortRecent(sessions []Session) {
	sort.Sort(SortByLastTime(sessions))
}
