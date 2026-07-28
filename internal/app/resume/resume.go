package resume

import (
	"context"
	"fmt"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type Service struct {
	planners map[domain.Source]source.ResumePlanner
}

func NewService(planners []source.ResumePlanner) *Service {
	m := make(map[domain.Source]source.ResumePlanner, len(planners))
	for _, planner := range planners {
		m[planner.Source()] = planner
	}
	return &Service{planners: m}
}

func (s *Service) Plan(ctx context.Context, target domain.ResumeTarget) (domain.ResumePlan, error) {
	planner, ok := s.planners[target.Session.Source]
	if !ok {
		return domain.ResumePlan{}, fmt.Errorf("%s resume: %w", target.Session.Source, source.ErrUnsupportedCapability)
	}
	return planner.PlanResume(ctx, target)
}
