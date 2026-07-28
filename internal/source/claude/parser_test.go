package claude

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestParseSessionMessages(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "sample.jsonl")
	msgs, err := ParseSessionMessages(path)
	if err != nil {
		t.Fatal(err)
	}

	// sample.jsonl has: 1 meta user (skip), 2 non-meta users, 2 assistants = 4 messages
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}

	if msgs[0].Role != "user" || msgs[0].Text != "拉一下最新代码" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Text != "好的，已拉取。" {
		t.Errorf("msgs[1] = %+v", msgs[1])
	}
	// msgs[3] should skip thinking block, only have text
	if msgs[3].Role != "assistant" || msgs[3].Text != "日志如下..." {
		t.Errorf("msgs[3] = %+v, want text='日志如下...'", msgs[3])
	}
}

func TestParseSessionMessagesVerboseMode(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "verbose_sample.jsonl")

	nonVerbose, err := ParseSessionMessages(path, false)
	if err != nil {
		t.Fatalf("ParseSessionMessages(..., false) error = %v", err)
	}

	verbose, err := ParseSessionMessages(path, true)
	if err != nil {
		t.Fatalf("ParseSessionMessages(..., true) error = %v", err)
	}

	if len(verbose) < len(nonVerbose) {
		t.Fatalf("verbose messages = %d, want >= non-verbose %d", len(verbose), len(nonVerbose))
	}

	if len(nonVerbose) != 5 {
		t.Fatalf("non-verbose messages = %d, want 5", len(nonVerbose))
	}
	if len(verbose) != 6 {
		t.Fatalf("verbose messages = %d, want 6", len(verbose))
	}

	if nonVerbose[2].Role != "tool" || nonVerbose[2].Text == "" {
		t.Fatalf("non-verbose tool summary = %+v, want tool summary message", nonVerbose[2])
	}
	if verbose[2].Role != "tool" || verbose[2].Text == "" {
		t.Fatalf("verbose tool detail = %+v, want tool detail message", verbose[2])
	}
	if verbose[3].Role != "tool_result" {
		t.Fatalf("verbose tool result role = %q, want tool_result", verbose[3].Role)
	}
	if verbose[3].Text == "" {
		t.Fatal("verbose tool result text should not be empty")
	}
}

func TestParseSessionMessagesTail(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "sample.jsonl")

	msgs, err := ParseSessionMessagesTail(path, false, 2, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 2 {
		t.Fatalf("tail messages = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Text != "看看日志" {
		t.Fatalf("tail first = %+v, want final user message", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Text != "日志如下..." {
		t.Fatalf("tail second = %+v, want final assistant message", msgs[1])
	}
}

func TestToolInfoAdditionalSummaries(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  string
	}{
		{
			name:  "structured output summary",
			tool:  "StructuredOutput",
			input: map[string]any{"summary": "all good", "severity": "low"},
			want:  "StructuredOutput all good",
		},
		{
			name:  "structured output findings count",
			tool:  "StructuredOutput",
			input: map[string]any{"findings": []any{"a", "b"}},
			want:  "StructuredOutput 2 findings",
		},
		{
			name:  "structured output confidence refuted",
			tool:  "StructuredOutput",
			input: map[string]any{"confidence": "high", "refuted": true, "evidence": []any{"log"}, "counterSource": "trace"},
			want:  "StructuredOutput refuted true",
		},
		{
			name:  "structured output ok issues",
			tool:  "StructuredOutput",
			input: map[string]any{"ok": false, "issues": []any{"missing scope"}},
			want:  "StructuredOutput ok false",
		},
		{
			name:  "structured output fixed skipped",
			tool:  "StructuredOutput",
			input: map[string]any{"fixed": []any{"a"}, "skipped": []any{"b", "c"}},
			want:  "StructuredOutput 1 fixed",
		},
		{
			name:  "structured output results",
			tool:  "StructuredOutput",
			input: map[string]any{"results": []any{"a", "b", "c"}},
			want:  "StructuredOutput 3 results",
		},
		{
			name: "ask user question",
			tool: "AskUserQuestion",
			input: map[string]any{"questions": []any{
				map[string]any{"header": "Scope", "question": "Pick one"},
				map[string]any{"question": "Confirm?"},
			}},
			want: "AskUserQuestion 2 questions: Pick one",
		},
		{
			name:  "task output snake id",
			tool:  "TaskOutput",
			input: map[string]any{"task_id": "task-1", "block": true},
			want:  "TaskOutput #task-1",
		},
		{
			name:  "task stop snake id",
			tool:  "TaskStop",
			input: map[string]any{"task_id": "task-1"},
			want:  "TaskStop #task-1",
		},
		{
			name:  "task get camel id",
			tool:  "TaskGet",
			input: map[string]any{"taskId": "task-1"},
			want:  "TaskGet #task-1",
		},
		{
			name:  "monitor command",
			tool:  "Monitor",
			input: map[string]any{"command": "go test ./...", "description": "run tests"},
			want:  "Monitor go test ./...",
		},
		{
			name:  "monitor task id",
			tool:  "Monitor",
			input: map[string]any{"task_id": "task-1"},
			want:  "Monitor #task-1",
		},
		{
			name:  "monitor shell id",
			tool:  "Monitor",
			input: map[string]any{"shellId": "shell-1", "timeoutSeconds": 30},
			want:  "Monitor shell-1",
		},
		{
			name:  "monitor bash id",
			tool:  "Monitor",
			input: map[string]any{"bashId": "bash-1"},
			want:  "Monitor bash-1",
		},
		{
			name:  "schedule wakeup",
			tool:  "ScheduleWakeup",
			input: map[string]any{"delaySeconds": 60, "reason": "retry checks"},
			want:  "ScheduleWakeup 60s retry checks",
		},
		{
			name:  "workflow script",
			tool:  "Workflow",
			input: map[string]any{"script": "go test ./...\necho done"},
			want:  "Workflow go test ./...",
		},
		{
			name:  "task list query",
			tool:  "TaskList",
			input: map[string]any{"query": "active work"},
			want:  "TaskList active work",
		},
		{
			name:  "send user message",
			tool:  "SendUserMessage",
			input: map[string]any{"status": "delivered", "message": "hello"},
			want:  "SendUserMessage delivered hello",
		},
		{
			name:  "artifact label and path",
			tool:  "Artifact",
			input: map[string]any{"label": "report", "file_path": "/tmp/report.html"},
			want:  "Artifact report /tmp/report.html",
		},
		{
			name:  "send user file count",
			tool:  "SendUserFile",
			input: map[string]any{"files": []any{"a.png", "b.png"}},
			want:  "SendUserFile 2 files",
		},
		{
			name:  "exit plan mode",
			tool:  "ExitPlanMode",
			input: map[string]any{"planFilePath": "/tmp/plan.md"},
			want:  "ExitPlanMode /tmp/plan.md",
		},
		{
			name:  "lowercase bash",
			tool:  "bash",
			input: map[string]any{"command": "ls -la"},
			want:  "bash ls -la",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolInfo(tt.tool, mustRawInput(t, tt.input))
			if got != tt.want {
				t.Fatalf("toolInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustRawInput(t *testing.T, input map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
