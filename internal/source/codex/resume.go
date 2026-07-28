package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
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
	args := []string{"codex", "resume", sess.ID}
	message := fmt.Sprintf("Resuming Codex session %s...", shortID)
	if target.Action == domain.ResumeActionFork {
		args = []string{"codex", "fork", sess.ID}
		message = fmt.Sprintf("Forking Codex session %s...", shortID)
	}
	return domain.ResumePlan{
		WorkingDir: sess.ProjectPath,
		Executable: "codex",
		Args:       args,
		HandoffDir: sess.ProjectPath,
		Message:    message,
	}, nil
}

func (a Adapter) ArchiveSpec(ctx context.Context, sess domain.Session) (source.ArchiveSpec, error) {
	_ = ctx
	return source.ArchiveSpec{SideDir: strings.TrimSuffix(sess.FilePath, ".jsonl")}, nil
}
