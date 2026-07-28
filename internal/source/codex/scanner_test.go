package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

func TestLoadCodexSessionIndex(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "session_index.jsonl")
	idx, err := LoadCodexSessionIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 2 {
		t.Fatalf("got %d entries, want 2", len(idx))
	}
	if idx["019d0000-0000-0000-0000-000000000001"] != "查看项目结构和测试框架" {
		t.Errorf("thread_name = %q", idx["019d0000-0000-0000-0000-000000000001"])
	}
}

func TestExtractCodexSessionID(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"rollout-2026-03-27T12-15-34-019d2d81-3016-7690-9871-b5220c730345.jsonl", "019d2d81-3016-7690-9871-b5220c730345"},
		{"rollout-2026-04-01T10-00-00-019d0000-0000-0000-0000-000000000001.jsonl", "019d0000-0000-0000-0000-000000000001"},
		{"not-a-codex-file.jsonl", ""},
	}
	for _, tt := range tests {
		got := ExtractCodexSessionID(tt.filename)
		if got != tt.want {
			t.Errorf("ExtractCodexSessionID(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestScanCodexSessionFile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "basic_session.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanCodexSessionFile(path, info.Size(), map[string]string{
		"019d0000-0000-0000-0000-000000000001": "查看项目结构和测试框架",
	})
	if err != nil {
		t.Fatal(err)
	}
	sess := scanned.Session

	if sess.ID != "019d0000-0000-0000-0000-000000000001" {
		t.Errorf("ID = %q", sess.ID)
	}
	if sess.Source != domain.SourceCodex {
		t.Errorf("Source = %q, want codex", sess.Source)
	}
	if sess.ProjectPath != "/Users/test/myproject" {
		t.Errorf("ProjectPath = %q", sess.ProjectPath)
	}
	if sess.ProjectDir != "myproject" {
		t.Errorf("ProjectDir = %q", sess.ProjectDir)
	}
	if sess.Title != "查看项目结构和测试框架" {
		t.Errorf("Title = %q", sess.Title)
	}
	if sess.Origin != "Codex Desktop" {
		t.Errorf("Origin = %q", sess.Origin)
	}
	if sess.Client != "Codex Desktop" {
		t.Errorf("Client = %q, want Codex Desktop for internal source=vscode", sess.Client)
	}
	if sess.MsgCount != 2 {
		t.Errorf("MsgCount = %d, want 2", sess.MsgCount)
	}
	if sess.Model != "gpt-5.4" {
		t.Errorf("Model = %q, want gpt-5.4", sess.Model)
	}
	// function_call + web_search_call + custom_tool_call = 3
	if sess.ToolCount != 3 {
		t.Errorf("ToolCount = %d, want 3", sess.ToolCount)
	}
	if sess.TokenUsage.Input != 12000 {
		t.Errorf("TokenUsage.Input = %d, want 12000", sess.TokenUsage.Input)
	}
	if sess.TokenUsage.Output != 500 {
		t.Errorf("TokenUsage.Output = %d, want 500", sess.TokenUsage.Output)
	}
	if sess.FirstMsg != "查看项目结构" {
		t.Errorf("FirstMsg = %q", sess.FirstMsg)
	}
	if sess.LastMsg != "搜索一下 Go 的 test 框架" {
		t.Errorf("LastMsg = %q", sess.LastMsg)
	}
	expectedStart := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if !sess.StartTime.Equal(expectedStart) {
		t.Errorf("StartTime = %v, want %v", sess.StartTime, expectedStart)
	}
}

func TestScanCodexSessionFile_Empty(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "empty_session.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanCodexSessionFile(path, info.Size(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sess := scanned.Session
	if sess.MsgCount != 0 {
		t.Errorf("MsgCount = %d, want 0", sess.MsgCount)
	}
	if sess.ProjectPath != "/Users/test/empty" {
		t.Errorf("ProjectPath = %q", sess.ProjectPath)
	}
}

func TestCodexClientLabelPrefersOriginator(t *testing.T) {
	tests := []struct {
		name string
		meta CodexSessionMeta
		want string
	}{
		{
			name: "desktop over internal vscode source",
			meta: CodexSessionMeta{Originator: "Codex Desktop", Source: json.RawMessage(`"vscode"`)},
			want: "Codex Desktop",
		},
		{
			name: "claude code over internal vscode source",
			meta: CodexSessionMeta{Originator: "Claude Code", Source: json.RawMessage(`"vscode"`)},
			want: "Claude Code",
		},
		{
			name: "source fallback",
			meta: CodexSessionMeta{Source: json.RawMessage(`"cli"`)},
			want: "cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codexClientLabel(tt.meta); got != tt.want {
				t.Fatalf("codexClientLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScanCodexSessionFileSummarizesApprovalReview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approval.jsonl")
	prompt := "The following is the Codex agent history added since your last approval assessment.\n" +
		">>> TRANSCRIPT DELTA START\n[1] tool exec_command call: {}\n>>> TRANSCRIPT DELTA END\n" +
		">>> APPROVAL REQUEST START\nAssess the exact planned action below.\nPlanned action JSON:\n" +
		"{\"command\":[\"/bin/zsh\",\"-lc\",\"make install\"],\"tool\":\"exec_command\"}\n" +
		">>> APPROVAL REQUEST END\n"
	lines := []map[string]any{
		{
			"timestamp": "2026-06-30T07:52:41.275Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":         "approval-id",
				"cwd":        "/Users/example/claude-sessions",
				"originator": "Codex Desktop",
			},
		},
		{
			"timestamp": "2026-06-30T07:52:42.275Z",
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": prompt,
			},
		},
	}
	var rawLines []string
	for _, line := range lines {
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		rawLines = append(rawLines, string(raw))
	}
	if err := os.WriteFile(path, []byte(strings.Join(rawLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanCodexSessionFile(path, info.Size(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if scanned.Session.FirstMsg != "Approval request: make install" {
		t.Fatalf("FirstMsg = %q", scanned.Session.FirstMsg)
	}
	if scanned.Session.LastMsg != "Approval request: make install" {
		t.Fatalf("LastMsg = %q", scanned.Session.LastMsg)
	}
	if len(scanned.SearchParts) == 0 || scanned.SearchParts[0] != "Approval request: make install" {
		t.Fatalf("SearchParts = %#v", scanned.SearchParts)
	}
}

func TestScanCodexSessionFileHidesGuardianApprovalReview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guardian.jsonl")
	prompt := "The following is the Codex agent history added since your last approval assessment.\n" +
		">>> APPROVAL REQUEST START\nPlanned action JSON:\n" +
		"{\"command\":[\"/bin/zsh\",\"-lc\",\"git status --short\"],\"tool\":\"exec_command\"}\n" +
		">>> APPROVAL REQUEST END\n"
	lines := []map[string]any{
		{
			"timestamp": "2026-06-30T07:52:41.275Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":               "guardian-id",
				"parent_thread_id": "parent-id",
				"thread_source":    "subagent",
				"source": map[string]any{
					"subagent": map[string]any{"other": "guardian"},
				},
				"cwd": "/Users/example/claude-sessions",
			},
		},
		{
			"timestamp": "2026-06-30T07:52:42.275Z",
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": prompt,
			},
		},
	}
	var rawLines []string
	for _, line := range lines {
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		rawLines = append(rawLines, string(raw))
	}
	if err := os.WriteFile(path, []byte(strings.Join(rawLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanCodexSessionFile(path, info.Size(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if scanned.Session.MsgCount != 0 {
		t.Fatalf("guardian MsgCount = %d, want hidden session with 0", scanned.Session.MsgCount)
	}
	if len(scanned.SearchParts) != 0 {
		t.Fatalf("guardian SearchParts = %#v, want none", scanned.SearchParts)
	}
}

func TestDiscoverReportsMissingCodexSessions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	candidates, err := NewAdapter().Discover(t.Context())
	if err == nil {
		t.Fatalf("Discover() error = nil, want missing sessions error")
	}
	if len(candidates) != 0 {
		t.Fatalf("Discover() candidates = %#v, want none", candidates)
	}
}

func TestDiscoverUsesThreadNameAsMetadataKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "04", "01")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	id := "019d0000-0000-0000-0000-000000000001"
	sessionPath := filepath.Join(sessionsDir, "rollout-2026-04-01T10-00-00-"+id+".jsonl")
	if err := os.WriteFile(sessionPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(session) error = %v", err)
	}
	indexPath := filepath.Join(home, ".codex", "session_index.jsonl")
	if err := os.WriteFile(indexPath, []byte(`{"id":"`+id+`","thread_name":"Updated thread"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(index) error = %v", err)
	}

	candidates, err := NewAdapter().Discover(t.Context())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("Discover() candidates = %d, want 1", len(candidates))
	}
	if candidates[0].MetadataKey != codexMetadataKey("Updated thread") {
		t.Fatalf("MetadataKey = %q, want versioned thread name", candidates[0].MetadataKey)
	}
	if candidates[0].Attributes["thread_name"] != "Updated thread" {
		t.Fatalf("thread_name attribute = %q", candidates[0].Attributes["thread_name"])
	}
}

func TestDiscoverSkipsGuardianSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "30")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	guardianID := "019f1684-7625-7c21-8b34-67de5d986c27"
	normalID := "019f1683-6145-73a2-8456-ad8680e1369a"
	writeJSONLLines(t, filepath.Join(sessionsDir, "rollout-2026-06-30T11-13-27-"+guardianID+".jsonl"), []map[string]any{
		{
			"timestamp": "2026-06-30T03:13:27Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":               guardianID,
				"parent_thread_id": normalID,
				"thread_source":    "subagent",
				"cwd":              "/tmp/project",
				"source": map[string]any{
					"subagent": map[string]any{"other": "guardian"},
				},
			},
		},
	})
	writeJSONLLines(t, filepath.Join(sessionsDir, "rollout-2026-06-30T11-12-00-"+normalID+".jsonl"), []map[string]any{
		{
			"timestamp": "2026-06-30T03:12:00Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":  normalID,
				"cwd": "/tmp/project",
			},
		},
	})

	candidates, err := NewAdapter().Discover(t.Context())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("Discover() candidates = %#v, want only normal session", candidates)
	}
	if got := ExtractCodexSessionID(filepath.Base(candidates[0].Path)); got != normalID {
		t.Fatalf("candidate id = %q, want %q", got, normalID)
	}
}
