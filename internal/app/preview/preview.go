package preview

import (
	"context"
	"fmt"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type Service struct {
	parsers map[domain.Source]source.PreviewParser
}

func NewService(parsers []source.PreviewParser) *Service {
	m := make(map[domain.Source]source.PreviewParser, len(parsers))
	for _, parser := range parsers {
		m[parser.Source()] = parser
	}
	return &Service{parsers: m}
}

func (s *Service) Load(ctx context.Context, sess domain.Session, verbose bool) ([]domain.ConversationTurn, error) {
	parser, ok := s.parsers[sess.Source]
	if !ok {
		return nil, fmt.Errorf("%s preview: %w", sess.Source, source.ErrUnsupportedCapability)
	}
	return parser.ParsePreview(ctx, sess, source.PreviewOptions{Verbose: verbose})
}

func (s *Service) LoadTail(ctx context.Context, sess domain.Session, verbose bool, maxMessages int, maxBytes int64) ([]domain.ConversationTurn, error) {
	parser, ok := s.parsers[sess.Source]
	if !ok {
		return nil, fmt.Errorf("%s preview tail: %w", sess.Source, source.ErrUnsupportedCapability)
	}
	return parser.ParsePreviewTail(ctx, sess, source.TailOptions{
		Verbose:     verbose,
		MaxMessages: maxMessages,
		MaxBytes:    maxBytes,
	})
}
