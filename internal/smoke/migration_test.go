package smoke

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	apparchive "github.com/jinguo998/claude-sessions/internal/app/archive"
	"github.com/jinguo998/claude-sessions/internal/app/model"
	apppreview "github.com/jinguo998/claude-sessions/internal/app/preview"
	appresume "github.com/jinguo998/claude-sessions/internal/app/resume"
	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
	"github.com/jinguo998/claude-sessions/internal/source/claude"
	"github.com/jinguo998/claude-sessions/internal/source/codex"
	"github.com/jinguo998/claude-sessions/internal/source/opencode"
	storagetrash "github.com/jinguo998/claude-sessions/internal/storage/trash"
	"github.com/jinguo998/claude-sessions/internal/ui/picker"
)

func TestPreviewVerboseSmoke(t *testing.T) {
	adapter := claude.NewAdapter()
	service := apppreview.NewService([]source.PreviewParser{adapter})
	sess := domain.Session{
		ID:       "preview",
		Source:   domain.SourceClaude,
		FilePath: fixture(t, "verbose_sample.jsonl"),
	}

	summary, err := service.Load(context.Background(), sess, false)
	if err != nil {
		t.Fatalf("summary preview error = %v", err)
	}
	verbose, err := service.Load(context.Background(), sess, true)
	if err != nil {
		t.Fatalf("verbose preview error = %v", err)
	}

	summaryText := normalizeTurns(summary)
	verboseText := normalizeTurns(verbose)
	if strings.Contains(summaryText, "AgentTeamConfig") {
		t.Fatalf("summary preview includes verbose-only tool_result content: %q", summaryText)
	}
	if !strings.Contains(verboseText, "AgentTeamConfig") {
		t.Fatalf("verbose preview missing tool_result content: %q", verboseText)
	}
}

func TestPickerSearchOrderSmoke(t *testing.T) {
	sessions := []model.Session{
		{ID: "newer", SearchText: "codex billing", LastTime: parseTime(t, "2026-04-14T12:00:00Z")},
		{ID: "older", SearchText: "codex billing", LastTime: parseTime(t, "2026-04-13T12:00:00Z")},
		{ID: "skip", SearchText: "claude billing", LastTime: parseTime(t, "2026-04-15T12:00:00Z")},
	}

	results := picker.FindSessions("codex billing", sessions)
	got := make([]string, 0, len(results))
	for _, result := range results {
		got = append(got, result.Session.ID)
	}
	if !reflect.DeepEqual(got, []string{"newer", "older"}) {
		t.Fatalf("picker result order = %#v", got)
	}
}

func TestResumePlanSmoke(t *testing.T) {
	claudeAdapter := claude.NewAdapter()
	codexAdapter := codex.NewAdapter()
	openCodeAdapter := opencode.NewAdapter()
	service := appresume.NewService([]source.ResumePlanner{claudeAdapter, codexAdapter, openCodeAdapter})

	claudePlan, err := service.Plan(context.Background(), domain.ResumeTarget{
		Session:        domain.Session{ID: "claude-session", Source: domain.SourceClaude, ProjectPath: "/repo"},
		Action:         domain.ResumeActionFork,
		PermissionMode: domain.PermissionModeFast,
	})
	if err != nil {
		t.Fatalf("Claude plan error = %v", err)
	}
	if !reflect.DeepEqual(claudePlan.Args, []string{"claude", "--resume", "claude-session", "--fork-session", "--dangerously-skip-permissions"}) {
		t.Fatalf("Claude plan args = %#v", claudePlan.Args)
	}

	codexPlan, err := service.Plan(context.Background(), domain.ResumeTarget{
		Session: domain.Session{ID: "codex-session", Source: domain.SourceCodex, ProjectPath: "/repo"},
		Action:  domain.ResumeActionFork,
	})
	if err != nil {
		t.Fatalf("Codex plan error = %v", err)
	}
	if !reflect.DeepEqual(codexPlan.Args, []string{"codex", "fork", "codex-session"}) {
		t.Fatalf("Codex plan args = %#v", codexPlan.Args)
	}

	openCodePlan, err := service.Plan(context.Background(), domain.ResumeTarget{
		Session: domain.Session{ID: "opencode-session", Source: domain.SourceOpenCode, ProjectPath: "/repo"},
		Action:  domain.ResumeActionFork,
	})
	if err != nil {
		t.Fatalf("OpenCode plan error = %v", err)
	}
	if !reflect.DeepEqual(openCodePlan.Args, []string{"opencode", "--session", "opencode-session", "--fork"}) {
		t.Fatalf("OpenCode plan args = %#v", openCodePlan.Args)
	}
}

func TestArchiveRestoreMetadataSmoke(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	sessionFile := filepath.Join(project, "session.jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sideDir := strings.TrimSuffix(sessionFile, ".jsonl")
	if err := os.Mkdir(sideDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	adapter := claude.NewAdapter()
	service := apparchive.NewService([]source.ArchiveSpecifier{adapter}, storagetrash.NewAt(home))
	sess := domain.Session{
		ID:          "archive-smoke",
		Source:      domain.SourceClaude,
		ProjectPath: project,
		FilePath:    sessionFile,
		Title:       "Archive smoke",
	}

	if _, err := service.Archive(context.Background(), sess); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	items, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("archive items = %d, want 1", len(items))
	}
	if items[0].Metadata.Title != "Archive smoke" || items[0].Metadata.Source != domain.SourceClaude {
		t.Fatalf("archive metadata = %#v", items[0].Metadata)
	}
	if err := service.Restore(context.Background(), items[0]); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, err := os.Stat(sessionFile); err != nil {
		t.Fatalf("restored session stat error = %v", err)
	}
	if _, err := os.Stat(sideDir); err != nil {
		t.Fatalf("restored side dir stat error = %v", err)
	}
}

func fixture(t *testing.T, parts ...string) string {
	t.Helper()
	root := filepath.Join("..", "..", "testdata")
	all := append([]string{root}, parts...)
	return filepath.Join(all...)
}

func normalizeTurns(turns []domain.ConversationTurn) string {
	var parts []string
	for _, turn := range turns {
		parts = append(parts, turn.Role+":"+strings.Join(strings.Fields(turn.Text), " "))
	}
	return strings.Join(parts, "\n")
}

func parseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}
