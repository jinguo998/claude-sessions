package query

import (
	"strings"
	"testing"

	"github.com/jinguo998/claude-sessions/internal/app/model"
	"github.com/jinguo998/claude-sessions/internal/domain"
)

func TestIndexBuildsSearchCorpusOutsideDomain(t *testing.T) {
	svc := NewService()
	sessions := []model.Session{{
		ID:          "s1",
		Source:      domain.SourceCodex,
		ProjectPath: "/workspace/acme",
		Title:       "Payment refactor",
		Client:      "vscode",
		Origin:      "cli",
		Labels:      []string{"billing"},
		Attributes:  map[string]string{"branch": "feature/payments"},
		FirstMsg:    "fix checkout",
		SearchText:  "raw parsed tool text",
	}}

	indexed := svc.Index(sessions)
	if len(indexed) != 1 {
		t.Fatalf("indexed sessions = %d, want 1", len(indexed))
	}
	corpus := indexed[0].SearchText
	for _, needle := range []string{
		"payment refactor",
		"vscode",
		"cli",
		"billing",
		"feature/payments",
		"raw parsed tool text",
	} {
		if !strings.Contains(corpus, needle) {
			t.Fatalf("corpus %q missing %q", corpus, needle)
		}
	}

	domainSession := sessions[0].Domain()
	if domainSession.Title != "Payment refactor" || domainSession.Client != "vscode" || domainSession.Origin != "cli" {
		t.Fatalf("domain normalized fields not preserved: %#v", domainSession)
	}
}

func TestFilterUsesQueryProjectAndSource(t *testing.T) {
	svc := NewService()
	sessions := []model.Session{
		{ID: "claude", Source: domain.SourceClaude, ProjectPath: "/a", FirstMsg: "billing bug"},
		{ID: "codex", Source: domain.SourceCodex, ProjectPath: "/a", Title: "billing plan"},
		{ID: "other", Source: domain.SourceCodex, ProjectPath: "/b", Title: "billing plan"},
	}

	got := svc.Filter(sessions, Filter{
		Query:       "billing",
		ProjectPath: "/a",
		Source:      domain.SourceCodex,
	})
	if len(got) != 1 || got[0].ID != "codex" {
		t.Fatalf("Filter() = %#v, want codex only", got)
	}
}
