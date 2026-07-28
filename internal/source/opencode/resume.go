package opencode

import (
	"context"
	"fmt"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

func (a Adapter) PlanResume(ctx context.Context, target domain.ResumeTarget) (domain.ResumePlan, error) {
	_ = ctx
	sess := target.Session
	shortID := sess.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if target.Action == domain.ResumeActionCd {
		return domain.ResumePlan{
			WorkingDir: sess.ProjectPath,
			HandoffDir: sess.ProjectPath,
			CdOnly:     true,
			Message:    fmt.Sprintf("Opening shell in %s", sess.ProjectPath),
		}, nil
	}
	args := []string{"opencode", "--session", sess.ID}
	message := fmt.Sprintf("Resuming OpenCode session %s...", shortID)
	if target.Action == domain.ResumeActionFork {
		args = append(args, "--fork")
		message = fmt.Sprintf("Forking OpenCode session %s...", shortID)
	}
	return domain.ResumePlan{
		WorkingDir: sess.ProjectPath,
		Executable: "opencode",
		Args:       args,
		HandoffDir: sess.ProjectPath,
		Message:    message,
	}, nil
}
