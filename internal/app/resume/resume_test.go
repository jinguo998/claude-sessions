package resume

import (
	"context"
	"errors"
	"testing"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type fakePlanner struct {
	source domain.Source
	plan   domain.ResumePlan
	target domain.ResumeTarget
}

func (f *fakePlanner) Source() domain.Source {
	return f.source
}

func (f *fakePlanner) PlanResume(ctx context.Context, target domain.ResumeTarget) (domain.ResumePlan, error) {
	_ = ctx
	f.target = target
	return f.plan, nil
}

func TestPlanDispatchesBySource(t *testing.T) {
	wantPlan := domain.ResumePlan{
		WorkingDir: "/workspace",
		Executable: "claude",
		Args:       []string{"claude", "--resume", "abc"},
	}
	planner := &fakePlanner{source: domain.SourceClaude, plan: wantPlan}
	service := NewService([]source.ResumePlanner{planner})

	target := domain.ResumeTarget{
		Session: domain.Session{ID: "abc", Source: domain.SourceClaude},
		Action:  domain.ResumeActionResume,
	}
	got, err := service.Plan(context.Background(), target)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got.Executable != wantPlan.Executable || got.WorkingDir != wantPlan.WorkingDir {
		t.Fatalf("Plan() = %#v, want %#v", got, wantPlan)
	}
	if planner.target.Session.ID != "abc" || planner.target.Action != domain.ResumeActionResume {
		t.Fatalf("planner target = %#v", planner.target)
	}
}

func TestPlanUnsupportedSourceReturnsCapabilityError(t *testing.T) {
	service := NewService(nil)

	_, err := service.Plan(context.Background(), domain.ResumeTarget{
		Session: domain.Session{Source: domain.Source("unknown")},
	})
	if !errors.Is(err, source.ErrUnsupportedCapability) {
		t.Fatalf("Plan() error = %v, want ErrUnsupportedCapability", err)
	}
}
