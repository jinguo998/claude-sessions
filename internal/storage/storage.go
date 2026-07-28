package storage

import (
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

type CachedSession struct {
	Session     domain.Session
	SearchText  string
	MetadataKey string
}

type MetadataCache interface {
	Get(path string, size int64, modTime time.Time) (CachedSession, bool)
	Put(path string, size int64, modTime time.Time, sess CachedSession)
	Save()
}

type ArchivedSessionMetadata struct {
	ID               string        `json:"id"`
	Source           domain.Source `json:"source"`
	ArchivedAt       string        `json:"archived_at"`
	OriginalFilePath string        `json:"original_file_path"`
	OriginalSideDir  string        `json:"original_side_dir,omitempty"`
	ProjectPath      string        `json:"project_path,omitempty"`
	Title            string        `json:"title,omitempty"`
	LegacyThreadName string        `json:"thread_name,omitempty"`
}

type ArchivedSession struct {
	ArchiveDir  string
	Metadata    ArchivedSessionMetadata
	SessionFile string
	SideDir     string
}

type TrashStore interface {
	Archive(domain.Session, string) (string, error)
	List() ([]ArchivedSession, error)
	Restore(ArchivedSession) error
	Delete(ArchivedSession) error
	DeleteAll([]ArchivedSession) error
}
