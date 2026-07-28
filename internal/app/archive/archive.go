package archive

import (
	"context"
	"fmt"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
	"github.com/jinguo998/claude-sessions/internal/storage"
)

type Item = storage.ArchivedSession
type Metadata = storage.ArchivedSessionMetadata

type Service struct {
	specifiers map[domain.Source]source.ArchiveSpecifier
	trash      storage.TrashStore
}

func NewService(specifiers []source.ArchiveSpecifier, trash storage.TrashStore) *Service {
	m := make(map[domain.Source]source.ArchiveSpecifier, len(specifiers))
	for _, specifier := range specifiers {
		m[specifier.Source()] = specifier
	}
	return &Service{specifiers: m, trash: trash}
}

func (s *Service) Archive(ctx context.Context, sess domain.Session) (string, error) {
	if s.trash == nil {
		return "", fmt.Errorf("trash store unavailable")
	}
	specifier, ok := s.specifiers[sess.Source]
	if !ok {
		return "", fmt.Errorf("%s archive: %w", sess.Source, source.ErrUnsupportedCapability)
	}
	spec, err := specifier.ArchiveSpec(ctx, sess)
	if err != nil {
		return "", err
	}
	return s.trash.Archive(sess, spec.SideDir)
}

func (s *Service) List(ctx context.Context) ([]Item, error) {
	_ = ctx
	if s.trash == nil {
		return nil, fmt.Errorf("trash store unavailable")
	}
	return s.trash.List()
}

func (s *Service) Restore(ctx context.Context, item Item) error {
	_ = ctx
	if s.trash == nil {
		return fmt.Errorf("trash store unavailable")
	}
	return s.trash.Restore(item)
}

func (s *Service) Delete(ctx context.Context, item Item) error {
	_ = ctx
	if s.trash == nil {
		return fmt.Errorf("trash store unavailable")
	}
	return s.trash.Delete(item)
}

func (s *Service) DeleteAll(ctx context.Context, items []Item) error {
	_ = ctx
	if s.trash == nil {
		return fmt.Errorf("trash store unavailable")
	}
	return s.trash.DeleteAll(items)
}
