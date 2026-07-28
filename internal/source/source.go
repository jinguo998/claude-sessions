package source

import (
	"context"
	"errors"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

var ErrUnsupportedCapability = errors.New("unsupported source capability")

type Candidate struct {
	Source      domain.Source
	Path        string
	ProjectDir  string
	FileSize    int64
	ModTime     time.Time
	MetadataKey string
	Attributes  map[string]string
}

type ScannedSession struct {
	Session     domain.Session
	SearchParts []string
}

type PreviewOptions struct {
	Verbose bool
}

type TailOptions struct {
	Verbose     bool
	MaxMessages int
	MaxBytes    int64
}

type ArchiveSpec struct {
	SideDir string
}

type Scanner interface {
	Source() domain.Source
	Discover(context.Context) ([]Candidate, error)
	ScanFile(context.Context, Candidate) (ScannedSession, error)
}

type PreviewParser interface {
	Source() domain.Source
	ParsePreview(context.Context, domain.Session, PreviewOptions) ([]domain.ConversationTurn, error)
	ParsePreviewTail(context.Context, domain.Session, TailOptions) ([]domain.ConversationTurn, error)
}

type ResumePlanner interface {
	Source() domain.Source
	PlanResume(context.Context, domain.ResumeTarget) (domain.ResumePlan, error)
}

type ArchiveSpecifier interface {
	Source() domain.Source
	ArchiveSpec(context.Context, domain.Session) (ArchiveSpec, error)
}
