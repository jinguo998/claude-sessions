package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCodexMessages(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "basic_session.jsonl")
	msgs, err := ParseCodexMessages(path, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) < 5 {
		t.Fatalf("got %d messages, want >= 5", len(msgs))
	}

	if msgs[0].Role != "user" || msgs[0].Text != "查看项目结构" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}

	toolCount := 0
	for _, m := range msgs {
		if m.Role == "tool" {
			toolCount++
		}
	}
	if toolCount < 3 {
		t.Errorf("tool messages = %d, want >= 3", toolCount)
	}

	last := msgs[len(msgs)-1]
	if last.Role != "assistant" {
		t.Errorf("last message role = %q, want assistant", last.Role)
	}
}

func TestParseCodexMessages_Verbose(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "basic_session.jsonl")

	nonVerbose, err := ParseCodexMessages(path, false)
	if err != nil {
		t.Fatal(err)
	}

	verbose, err := ParseCodexMessages(path, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(verbose) <= len(nonVerbose) {
		t.Fatalf("verbose messages (%d) should be > non-verbose (%d)", len(verbose), len(nonVerbose))
	}

	hasToolResult := false
	for _, m := range verbose {
		if m.Role == "tool_result" {
			hasToolResult = true
			break
		}
	}
	if !hasToolResult {
		t.Error("verbose mode should include tool_result messages")
	}
}

func TestParseCodexMessagesTail(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "basic_session.jsonl")

	full, err := ParseCodexMessages(path, false)
	if err != nil {
		t.Fatal(err)
	}
	tail, err := ParseCodexMessagesTail(path, false, 2, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}

	if len(tail) != 2 {
		t.Fatalf("tail messages = %d, want 2", len(tail))
	}
	if tail[0] != full[len(full)-2] || tail[1] != full[len(full)-1] {
		t.Fatalf("tail messages = %+v, want final two full messages %+v", tail, full[len(full)-2:])
	}
}

func TestParseCodexMessagesTailExpandsPastLargeNonDisplayableLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large-tail.jsonl")
	huge := strings.Repeat("x", 2048)
	lines := []string{
		`{"timestamp":"2026-06-03T16:48:49.398Z","type":"event_msg","payload":{"type":"user_message","message":"first prompt"}}`,
		`{"timestamp":"2026-06-03T16:48:50.398Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first answer"}]}}`,
		`{"timestamp":"2026-06-03T16:48:51.398Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"` + huge + `"}]}}`,
		`{"timestamp":"2026-06-03T16:48:52.398Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"final answer"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tail, err := ParseCodexMessagesTail(path, false, 3, 512)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 3 {
		t.Fatalf("tail messages = %d, want 3: %+v", len(tail), tail)
	}
	if tail[0].Role != "user" || tail[0].Text != "first prompt" {
		t.Fatalf("tail[0] = %+v, want first prompt", tail[0])
	}
	if tail[1].Role != "assistant" || tail[1].Text != "first answer" {
		t.Fatalf("tail[1] = %+v, want first answer", tail[1])
	}
	if tail[2].Role != "assistant" || tail[2].Text != "final answer" {
		t.Fatalf("tail[2] = %+v, want final answer", tail[2])
	}
}

func TestParseCodexMessages_SkipsReasoning(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "basic_session.jsonl")
	msgs, err := ParseCodexMessages(path, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.Role == "reasoning" {
			t.Error("reasoning messages should be skipped")
		}
	}
}

func TestParseCodexMessages_SkipsDeveloper(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "codex", "basic_session.jsonl")
	msgs, err := ParseCodexMessages(path, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.Text == "<permissions instructions>sandbox config here</permissions instructions>" {
			t.Error("developer messages should be skipped")
		}
	}
}

func TestParseCodexMessagesSummarizesApprovalReview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approval.jsonl")
	prompt := "The following is the Codex agent history added since your last approval assessment.\n" +
		">>> TRANSCRIPT DELTA START\n[1] tool exec_command call: {}\n>>> TRANSCRIPT DELTA END\n" +
		">>> APPROVAL REQUEST START\nAssess the exact planned action below.\nPlanned action JSON:\n" +
		"{\"command\":[\"/bin/zsh\",\"-lc\",\"go test ./...\"],\"tool\":\"exec_command\"}\n" +
		">>> APPROVAL REQUEST END\n"
	decision := `{"risk_level":"low","user_authorization":"high","outcome":"allow","rationale":"Running tests is a routine local verification step."}`
	lines := []map[string]any{
		{
			"timestamp": "2026-06-30T07:52:41.275Z",
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": prompt,
			},
		},
		{
			"timestamp": "2026-06-30T07:52:45.488Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]string{{
					"type": "output_text",
					"text": decision,
				}},
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

	msgs, err := ParseCodexMessages(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Text != "Approval request: go test ./..." {
		t.Fatalf("request message = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Text != "Approval decision: allow (low): Running tests is a routine local verification step." {
		t.Fatalf("decision message = %+v", msgs[1])
	}
	if strings.Contains(msgs[0].Text, "TRANSCRIPT DELTA") || strings.HasPrefix(msgs[1].Text, "{") {
		t.Fatalf("approval review was not summarized: %+v", msgs)
	}
}

func TestParseCodexMessagesAddsGuardianApprovalEventsToParent(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".codex", "sessions", "2026", "06", "30")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(root, "rollout-parent.jsonl")
	guardianPath := filepath.Join(root, "rollout-guardian.jsonl")

	parentLines := []map[string]any{
		{
			"timestamp": "2026-06-30T07:50:00Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"session_id": "parent-session-id",
				"id":         "parent-rollout-id",
				"cwd":        "/tmp/project",
			},
		},
		{
			"timestamp": "2026-06-30T07:50:01Z",
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": "do the work",
			},
		},
	}
	writeJSONLLines(t, parentPath, parentLines)

	prompt := "The following is the Codex agent history added since your last approval assessment.\n" +
		">>> APPROVAL REQUEST START\nPlanned action JSON:\n" +
		"{\"command\":[\"/bin/zsh\",\"-lc\",\"git status --short\"],\"tool\":\"exec_command\"}\n" +
		">>> APPROVAL REQUEST END\n"
	decision := `{"risk_level":"low","user_authorization":"high","outcome":"allow","rationale":"Local read-only git status is low risk."}`
	guardianLines := []map[string]any{
		{
			"timestamp": "2026-06-30T07:51:00Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":               "guardian-id",
				"parent_thread_id": "parent-session-id",
				"thread_source":    "subagent",
				"source": map[string]any{
					"subagent": map[string]any{"other": "guardian"},
				},
			},
		},
		{
			"timestamp": "2026-06-30T07:51:01Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]string{{
					"type": "input_text",
					"text": prompt,
				}},
			},
		},
		{
			"timestamp": "2026-06-30T07:51:02Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]string{{
					"type": "output_text",
					"text": decision,
				}},
			},
		},
	}
	writeJSONLLines(t, guardianPath, guardianLines)

	msgs, err := ParseCodexMessages(parentPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Text != "do the work" {
		t.Fatalf("parent message = %+v", msgs[0])
	}
	if msgs[1].Role != "approval" || msgs[1].Text != "approved git status --short" {
		t.Fatalf("approval event = %+v", msgs[1])
	}
}

func TestParseCodexMessagesAddsMultipleGuardianApprovalEvents(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".codex", "sessions", "2026", "06", "30")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(root, "rollout-parent.jsonl")
	guardianPath := filepath.Join(root, "rollout-guardian.jsonl")

	writeJSONLLines(t, parentPath, []map[string]any{
		{
			"timestamp": "2026-06-30T07:50:00Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"session_id": "parent-session-id",
				"id":         "parent-rollout-id",
				"cwd":        "/tmp/project",
			},
		},
		{
			"timestamp": "2026-06-30T07:50:01Z",
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": "do the work",
			},
		},
		{
			"timestamp": "2026-06-30T07:53:00Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]string{{
					"type": "output_text",
					"text": "done",
				}},
			},
		},
	})

	requestOne := "The following is the Codex agent history whose request action you are assessing.\n" +
		">>> APPROVAL REQUEST START\nPlanned action JSON:\n" +
		"{\"command\":[\"/bin/zsh\",\"-lc\",\"git status --short\"],\"tool\":\"exec_command\"}\n" +
		">>> APPROVAL REQUEST END\n"
	requestTwo := "The following is the Codex agent history added since your last approval assessment.\n" +
		">>> APPROVAL REQUEST START\nPlanned action JSON:\n" +
		"{\"command\":[\"/bin/zsh\",\"-lc\",\"make install\"],\"tool\":\"exec_command\"}\n" +
		">>> APPROVAL REQUEST END\n"
	writeJSONLLines(t, guardianPath, []map[string]any{
		{
			"timestamp": "2026-06-30T07:51:00Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":               "guardian-id",
				"parent_thread_id": "parent-session-id",
				"thread_source":    "subagent",
				"source": map[string]any{
					"subagent": map[string]any{"other": "guardian"},
				},
			},
		},
		{
			"timestamp": "2026-06-30T07:51:01Z",
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": requestOne,
			},
		},
		{
			"timestamp": "2026-06-30T07:51:01Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]string{{
					"type": "input_text",
					"text": requestOne,
				}},
			},
		},
		{
			"timestamp": "2026-06-30T07:51:02Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]string{{
					"type": "output_text",
					"text": `{"risk_level":"low","outcome":"allow"}`,
				}},
			},
		},
		{
			"timestamp": "2026-06-30T07:52:01Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]string{{
					"type": "input_text",
					"text": requestTwo,
				}},
			},
		},
		{
			"timestamp": "2026-06-30T07:52:02Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]string{{
					"type": "output_text",
					"text": `{"risk_level":"medium","outcome":"allow"}`,
				}},
			},
		},
		{
			"timestamp": "2026-06-30T07:52:03Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]string{{
					"type": "output_text",
					"text": `{"risk_level":"low","outcome":"allow"}`,
				}},
			},
		},
	})

	msgs, err := ParseCodexMessages(parentPath, false)
	if err != nil {
		t.Fatal(err)
	}
	got := texts(msgs)
	want := []string{
		"do the work",
		"approved git status --short",
		"approved make install (medium risk)",
		"done",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}

	tail, err := ParseCodexMessagesTail(parentPath, false, 2, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	gotTail := texts(tail)
	wantTail := []string{"approved make install (medium risk)", "done"}
	if strings.Join(gotTail, "\n") != strings.Join(wantTail, "\n") {
		t.Fatalf("tail = %#v, want %#v", gotTail, wantTail)
	}
}

func TestCodexFunctionToolSummaries(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{
			name: "write stdin control character",
			tool: "write_stdin",
			args: map[string]any{"session_id": 42, "chars": "\x04"},
			want: "write_stdin session 42 Ctrl-D",
		},
		{
			name: "write stdin text",
			tool: "write_stdin",
			args: map[string]any{"session_id": 42, "chars": "go test ./...\n"},
			want: "write_stdin session 42 go test ./...",
		},
		{
			name: "press key",
			tool: "press_key",
			args: map[string]any{"app": "Chrome", "key": "Enter"},
			want: "press_key Chrome Enter",
		},
		{
			name: "press key pid",
			tool: "press_key",
			args: map[string]any{"pid": 1234, "key": "Enter"},
			want: "press_key pid 1234 Enter",
		},
		{
			name: "hotkey",
			tool: "hotkey",
			args: map[string]any{"keys": []any{"cmd", "l"}},
			want: "hotkey cmd+l",
		},
		{
			name: "click element",
			tool: "click",
			args: map[string]any{"app": "Chrome", "element_index": 7},
			want: "click Chrome element #7",
		},
		{
			name: "click coordinates",
			tool: "click",
			args: map[string]any{"app": "Chrome", "x": 12, "y": 34},
			want: "click Chrome 12,34",
		},
		{
			name: "click pid coordinates",
			tool: "click",
			args: map[string]any{"pid": 1234, "x": 12, "y": 34},
			want: "click pid 1234 12,34",
		},
		{
			name: "type text",
			tool: "type_text",
			args: map[string]any{"app": "Terminal", "text": "hello world"},
			want: "type_text Terminal \"hello world\"",
		},
		{
			name: "scroll",
			tool: "scroll",
			args: map[string]any{"app": "Chrome", "direction": "down", "pages": 2},
			want: "scroll Chrome down 2p",
		},
		{
			name: "set value",
			tool: "set_value",
			args: map[string]any{"app": "Chrome", "element_index": 3, "value": "search term"},
			want: "set_value Chrome element #3 \"search term\"",
		},
		{
			name: "get app state",
			tool: "get_app_state",
			args: map[string]any{"app": "Chrome"},
			want: "get_app_state Chrome",
		},
		{
			name: "update plan active step",
			tool: "update_plan",
			args: map[string]any{"plan": []any{
				map[string]any{"step": "write tests", "status": "completed"},
				map[string]any{"step": "implement summaries", "status": "in_progress"},
			}},
			want: "update_plan implement summaries",
		},
		{
			name: "update goal status",
			tool: "update_goal",
			args: map[string]any{"status": "complete"},
			want: "update_goal complete",
		},
		{
			name: "request user input question count",
			tool: "request_user_input",
			args: map[string]any{"questions": []any{
				map[string]any{"question": "Pick one"},
				map[string]any{"question": "Confirm?"},
			}},
			want: "request_user_input 2 questions",
		},
		{
			name: "js title",
			tool: "js",
			args: map[string]any{"title": "inspect DOM", "code": "document.body.innerText"},
			want: "js inspect DOM",
		},
		{
			name: "spawn agent",
			tool: "spawn_agent",
			args: map[string]any{"agent_type": "reviewer", "message": "check parser behavior"},
			want: "spawn_agent reviewer check parser behavior",
		},
		{
			name: "wait agent",
			tool: "wait_agent",
			args: map[string]any{"targets": []any{"agent-a", "agent-b"}},
			want: "wait_agent 2 targets",
		},
		{
			name: "close agent",
			tool: "close_agent",
			args: map[string]any{"target": "agent-a"},
			want: "close_agent agent-a",
		},
		{
			name: "generic title before large markdown",
			tool: "mcp__docs__create",
			args: map[string]any{"title": "Launch notes", "markdown": strings.Repeat("x", 1000)},
			want: "mcp__docs__create Launch notes",
		},
		{
			name: "no argument utility",
			tool: "list_apps",
			args: map[string]any{},
			want: "list_apps",
		},
		{
			name: "workspace dependencies utility",
			tool: "load_workspace_dependencies",
			args: map[string]any{},
			want: "load_workspace_dependencies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codexToolInfo(tt.tool, mustJSONArgs(t, tt.args))
			if got != tt.want {
				t.Fatalf("codexToolInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseCodexMessagesCustomToolSummaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-tools.jsonl")
	lines := []string{
		`{"timestamp":"2026-06-03T16:48:49.398Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch\n*** Update File: a.go\n@@\n*** Add File: b.go\n+package b\n*** End Patch"}}`,
		`{"timestamp":"2026-06-03T16:48:50.398Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch\n*** End Patch"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs, err := ParseCodexMessages(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2: %+v", len(msgs), msgs)
	}
	if msgs[0].Text != "apply_patch 2 files: update a.go, add b.go" {
		t.Fatalf("first tool = %q", msgs[0].Text)
	}
	if msgs[1].Text != "apply_patch" {
		t.Fatalf("second tool = %q", msgs[1].Text)
	}
}

func mustJSONArgs(t *testing.T, v map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func writeJSONLLines(t *testing.T, path string, lines []map[string]any) {
	t.Helper()
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
}

func texts(msgs []Message) []string {
	out := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, msg.Text)
	}
	return out
}
