package opencode

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type fakeRunner struct {
	dbPath string
	rows   []sessionRow
	texts  map[string][]userTextRow
	parts  map[string][]previewRow
}

func (f fakeRunner) DBPath(context.Context) (string, error) {
	return f.dbPath, nil
}

func (f fakeRunner) Query(_ context.Context, sql string, dest any) error {
	data := any(nil)
	switch {
	case strings.Contains(sql, "from session s"):
		data = f.rows
	case strings.Contains(sql, "json_extract(m.data, '$.role') = 'user'"):
		data = f.texts[extractQuotedSessionID(sql)]
	case strings.Contains(sql, "from message m"):
		data = f.parts[extractQuotedSessionID(sql)]
	default:
		data = []any{}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}

func extractQuotedSessionID(sql string) string {
	const marker = "session_id = '"
	idx := strings.Index(sql, marker)
	if idx < 0 {
		return ""
	}
	rest := sql[idx+len(marker):]
	end := strings.Index(rest, "'")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func testAdapter() Adapter {
	return Adapter{runner: fakeRunner{
		dbPath: "/tmp/opencode.db",
		rows: []sessionRow{
			{
				ID:             "ses_123",
				Directory:      "/repo/demo",
				Title:          "Build demo",
				Agent:          "build",
				Model:          `{"id":"MiniMax-M3","providerID":"minimax"}`,
				TokensInput:    100,
				TokensOutput:   50,
				TimeCreated:    1000,
				TimeUpdated:    2000,
				MessageUpdated: 2500,
				PartUpdated:    3000,
				MessageCount:   4,
				PartCount:      8,
				UserCount:      2,
				ToolCount:      1,
			},
		},
		texts: map[string][]userTextRow{
			"ses_123": {
				{TimeCreated: 1000, Text: "  build   this demo  "},
				{TimeCreated: 2000, Text: "ship it"},
			},
		},
		parts: map[string][]previewRow{
			"ses_123": testPreviewRows(),
		},
	}}
}

func testPreviewRows() []previewRow {
	toolPart := mustPartJSON(partData{
		Type: "tool",
		Tool: "bash",
		State: mustJSON(map[string]any{
			"input":  map[string]any{"command": "go test ./..."},
			"output": "ok",
		}),
	})
	return []previewRow{
		{MessageID: "m1", Role: "user", MessageTime: 1000, PartTime: 1000, PartData: mustPartJSON(partData{Type: "text", Text: "build this demo"})},
		{MessageID: "m2", Role: "assistant", MessageTime: 2000, PartTime: 2000, PartData: mustPartJSON(partData{Type: "text", Text: "Working"})},
		{MessageID: "m2", Role: "assistant", MessageTime: 2000, PartTime: 2100, PartData: toolPart},
	}
}

func mustPartJSON(part partData) json.RawMessage {
	raw, err := json.Marshal(part)
	if err != nil {
		panic(err)
	}
	return raw
}

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestDiscoverBuildsVirtualCandidates(t *testing.T) {
	adapter := testAdapter()

	candidates, err := adapter.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(candidates))
	}
	got := candidates[0]
	if got.Source != domain.SourceOpenCode {
		t.Fatalf("source = %q, want %q", got.Source, domain.SourceOpenCode)
	}
	if got.Path != "/tmp/opencode.db#ses_123" {
		t.Fatalf("path = %q", got.Path)
	}
	if got.ProjectDir != "demo" {
		t.Fatalf("project dir = %q, want demo", got.ProjectDir)
	}
	if got.ModTime != time.UnixMilli(3000) {
		t.Fatalf("mod time = %s, want %s", got.ModTime, time.UnixMilli(3000))
	}
	if !strings.Contains(got.MetadataKey, "Build demo") || !strings.Contains(got.MetadataKey, "MiniMax-M3") {
		t.Fatalf("metadata key missing fingerprint fields: %q", got.MetadataKey)
	}
}

func TestScanFileNormalizesSession(t *testing.T) {
	adapter := testAdapter()
	candidates, err := adapter.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	scanned, err := adapter.ScanFile(context.Background(), candidates[0])
	if err != nil {
		t.Fatalf("ScanFile() error = %v", err)
	}
	sess := scanned.Session
	if sess.ID != "ses_123" || sess.Source != domain.SourceOpenCode {
		t.Fatalf("session identity = %#v", sess)
	}
	if sess.ProjectPath != "/repo/demo" || sess.ProjectDir != "demo" {
		t.Fatalf("project = %q/%q", sess.ProjectPath, sess.ProjectDir)
	}
	if sess.Title != "Build demo" || sess.Client != "opencode" || sess.Origin != "build" {
		t.Fatalf("normalized fields = %#v", sess)
	}
	if sess.Model != "minimax/MiniMax-M3" {
		t.Fatalf("model = %q", sess.Model)
	}
	if sess.FirstMsg != "build this demo" || sess.LastMsg != "ship it" {
		t.Fatalf("messages = %q/%q", sess.FirstMsg, sess.LastMsg)
	}
	if sess.MsgCount != 2 || sess.ToolCount != 1 {
		t.Fatalf("counts = %d/%d", sess.MsgCount, sess.ToolCount)
	}
	if sess.TokenUsage.Input != 100 || sess.TokenUsage.Output != 50 {
		t.Fatalf("token usage = %#v", sess.TokenUsage)
	}
	if !reflect.DeepEqual(scanned.SearchParts, []string{"build this demo", "ship it", "Build demo"}) {
		t.Fatalf("search parts = %#v", scanned.SearchParts)
	}
}

func TestPreviewSummaryAndVerbose(t *testing.T) {
	adapter := testAdapter()
	sess := domain.Session{ID: "ses_123", Source: domain.SourceOpenCode, FilePath: "/tmp/opencode.db#ses_123"}

	summary, err := adapter.ParsePreview(context.Background(), sess, source.PreviewOptions{})
	if err != nil {
		t.Fatalf("ParsePreview() error = %v", err)
	}
	if got := turnTexts(summary); !reflect.DeepEqual(got, []string{"user:build this demo", "assistant:Working", "tool:bash go test ./..."}) {
		t.Fatalf("summary turns = %#v", got)
	}

	verbose, err := adapter.ParsePreview(context.Background(), sess, source.PreviewOptions{Verbose: true})
	if err != nil {
		t.Fatalf("ParsePreview(verbose) error = %v", err)
	}
	if got := turnTexts(verbose); !reflect.DeepEqual(got, []string{"user:build this demo", "assistant:Working", "tool:bash go test ./...", "tool_result:ok"}) {
		t.Fatalf("verbose turns = %#v", got)
	}
}

func TestRowsToTurnsAdditionalToolSummaries(t *testing.T) {
	rows := []previewRow{
		{MessageID: "m1", Role: "assistant", MessageTime: 1000, PartTime: 1000, PartData: mustPartJSON(partData{
			Type: "tool",
			Tool: "bash",
			State: mustJSON(map[string]any{
				"input": map[string]any{"command": "go test ./..."},
			}),
		})},
		{MessageID: "m1", Role: "assistant", MessageTime: 1000, PartTime: 1001, PartData: mustPartJSON(partData{
			Type: "tool",
			Tool: "task",
			State: mustJSON(map[string]any{
				"input": map[string]any{"description": "investigate bug", "prompt": "long prompt"},
			}),
		})},
		{MessageID: "m1", Role: "assistant", MessageTime: 1000, PartTime: 1002, PartData: mustPartJSON(partData{
			Type: "tool",
			Tool: "todowrite",
			State: mustJSON(map[string]any{
				"input": map[string]any{"todos": []any{"one", "two"}},
			}),
		})},
		{MessageID: "m1", Role: "assistant", MessageTime: 1000, PartTime: 1003, PartData: mustPartJSON(partData{
			Type: "tool",
			Tool: "skill",
			State: mustJSON(map[string]any{
				"input": map[string]any{"name": "code-review"},
			}),
		})},
		{MessageID: "m1", Role: "assistant", MessageTime: 1000, PartTime: 1004, PartData: mustPartJSON(partData{
			Type: "tool",
			Tool: "question",
			State: mustJSON(map[string]any{
				"input": map[string]any{"questions": []any{
					map[string]any{"question": "Pick one"},
					map[string]any{"question": "Confirm?"},
				}},
			}),
		})},
		{MessageID: "m1", Role: "assistant", MessageTime: 1000, PartTime: 1005, PartData: mustJSON(map[string]any{
			"type":  "patch",
			"files": []any{"a.go", "b.go"},
		})},
	}

	got := turnTexts(rowsToTurns(rows, false))
	want := []string{
		"tool:bash go test ./...",
		"tool:task investigate bug",
		"tool:todowrite 2 todos",
		"tool:skill code-review",
		"tool:question 2 questions",
		"tool:patch 2 files: a.go, b.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("turns = %#v, want %#v", got, want)
	}
}

func TestPreviewAcceptsSQLiteJSONStringPartData(t *testing.T) {
	rawPart := mustPartJSON(partData{Type: "text", Text: "from sqlite json"})
	quoted, err := json.Marshal(string(rawPart))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	turns := rowsToTurns([]previewRow{
		{MessageID: "m1", Role: "user", MessageTime: 1000, PartTime: 1000, PartData: quoted},
	}, false)
	if got := turnTexts(turns); !reflect.DeepEqual(got, []string{"user:from sqlite json"}) {
		t.Fatalf("turns = %#v", got)
	}
}

func TestResumePlans(t *testing.T) {
	adapter := testAdapter()
	sess := domain.Session{ID: "ses_123", Source: domain.SourceOpenCode, ProjectPath: "/repo/demo"}

	resume, err := adapter.PlanResume(context.Background(), domain.ResumeTarget{Session: sess, Action: domain.ResumeActionResume})
	if err != nil {
		t.Fatalf("PlanResume() error = %v", err)
	}
	if !reflect.DeepEqual(resume.Args, []string{"opencode", "--session", "ses_123"}) {
		t.Fatalf("resume args = %#v", resume.Args)
	}

	fork, err := adapter.PlanResume(context.Background(), domain.ResumeTarget{Session: sess, Action: domain.ResumeActionFork})
	if err != nil {
		t.Fatalf("PlanResume(fork) error = %v", err)
	}
	if !reflect.DeepEqual(fork.Args, []string{"opencode", "--session", "ses_123", "--fork"}) {
		t.Fatalf("fork args = %#v", fork.Args)
	}

	cd, err := adapter.PlanResume(context.Background(), domain.ResumeTarget{Session: sess, Action: domain.ResumeActionCd})
	if err != nil {
		t.Fatalf("PlanResume(cd) error = %v", err)
	}
	if !cd.CdOnly || cd.WorkingDir != "/repo/demo" {
		t.Fatalf("cd plan = %#v", cd)
	}
}

func turnTexts(turns []domain.ConversationTurn) []string {
	out := make([]string, 0, len(turns))
	for _, turn := range turns {
		out = append(out, turn.Role+":"+turn.Text)
	}
	return out
}
