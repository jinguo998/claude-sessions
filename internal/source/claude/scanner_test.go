package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanSessionFile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "sample.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanSessionFile(path, "test-project", info.Size())
	if err != nil {
		t.Fatal(err)
	}
	sess := scanned.Session

	if sess.FirstMsg != "拉一下最新代码" {
		t.Errorf("FirstMsg = %q, want %q", sess.FirstMsg, "拉一下最新代码")
	}
	if sess.MsgCount != 4 {
		t.Errorf("MsgCount = %d, want 4", sess.MsgCount)
	}
	expectedStart := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if !sess.StartTime.Equal(expectedStart) {
		t.Errorf("StartTime = %v, want %v", sess.StartTime, expectedStart)
	}
	expectedLast := time.Date(2026, 4, 1, 10, 5, 1, 0, time.UTC)
	if !sess.LastTime.Equal(expectedLast) {
		t.Errorf("LastTime = %v, want %v", sess.LastTime, expectedLast)
	}
}

func TestScanSessionFileCountsClaudeUsageOncePerMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	data := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"start"},"isMeta":false,"timestamp":"2026-04-01T10:00:00.000Z"}`,
		`{"type":"assistant","message":{"id":"msg_1","model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"working"}],"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":30,"output_tokens":40}},"timestamp":"2026-04-01T10:00:01.000Z"}`,
		`{"type":"assistant","message":{"id":"msg_1","model":"claude-opus-4-6","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{}}],"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":30,"output_tokens":40}},"timestamp":"2026-04-01T10:00:02.000Z"}`,
		`{"type":"assistant","message":{"id":"msg_2","model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":1,"output_tokens":2}},"timestamp":"2026-04-01T10:00:03.000Z"}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanSessionFile(path, "test-project", info.Size())
	if err != nil {
		t.Fatal(err)
	}
	sess := scanned.Session

	if sess.TokenUsage.Input != 61 {
		t.Fatalf("TokenUsage.Input = %d, want 61", sess.TokenUsage.Input)
	}
	if sess.TokenUsage.Output != 42 {
		t.Fatalf("TokenUsage.Output = %d, want 42", sess.TokenUsage.Output)
	}
	if sess.TotalTokens() != 103 {
		t.Fatalf("TotalTokens() = %d, want 103", sess.TotalTokens())
	}
}

func TestDiscoverReportsMissingClaudeProjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	candidates, err := NewAdapter().Discover(t.Context())
	if err == nil {
		t.Fatalf("Discover() error = nil, want missing projects error")
	}
	if len(candidates) != 0 {
		t.Fatalf("Discover() candidates = %#v, want none", candidates)
	}
}

func TestDecodeProjectDir(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "example.com", "sample-org", ".worktree", "agent_console_phase2")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	encodedProjectPath := strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(projectPath)

	tests := []struct {
		input string
		want  string
	}{
		{"-Users-example", "/Users/example"},
		{encodedProjectPath, projectPath},
		{"", ""},
	}
	for _, tt := range tests {
		got := decodeProjectDir(tt.input)
		if got != tt.want {
			t.Errorf("DecodeProjectDir(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsSystemContent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "command", in: "<command-run>", want: true},
		{name: "local command", in: "<local-command foo>", want: true},
		{name: "json payload", in: "{\"key\":\"value\"}", want: true},
		{name: "task", in: "<task-run>", want: true},
		{name: "regular text", in: "hello world", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSystemContent(tt.in); got != tt.want {
				t.Fatalf("isSystemContent(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
