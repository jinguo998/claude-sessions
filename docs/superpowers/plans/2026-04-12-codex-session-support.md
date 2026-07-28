# Codex Session Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Codex CLI session browsing, preview, and resume alongside existing Claude Code sessions.

**Architecture:** Additive approach — new `codex_types.go`, `codex_scanner.go`, `codex_parser.go` alongside existing files. `Source` field on `Session` struct. TUI gains source badge, source filter, and source-aware resume/fork dispatch.

**Tech Stack:** Go, Bubble Tea, lipgloss, encoding/json

**Spec:** `docs/superpowers/specs/2026-04-12-codex-session-support-design.md`

**Format corrections from review (must be used instead of spec where they differ):**
- Assistant message content blocks use `type: "output_text"` (NOT `"input_text"`)
- `web_search_call` payload has `action.query`/`action.queries` (NOT `name`/`arguments`)
- `custom_tool_call` has `input` field (string), not `arguments`
- `session_meta.source` can be a string OR a JSON object (subagent sessions) — use `json.RawMessage`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/session/session.go` | Modify | Add `Source` type, constants, new fields on `Session` |
| `internal/session/codex_types.go` | Create | Codex JSONL deserialization types |
| `internal/scanner/codex_scanner.go` | Create | Codex session discovery + metadata extraction |
| `internal/scanner/scanner.go` | Modify | `ScanAllSessions` merges Claude + Codex, tolerates missing dirs |
| `internal/parser/codex_parser.go` | Create | Codex message parsing for preview |
| `internal/tui/helpers.go` | Modify | `LoadPreviewContent` dispatches by source |
| `internal/tui/list.go` | Modify | Source badge column, source filter, Codex detail metadata |
| `internal/tui/preview.go` | Modify | Pass source to `LoadPreviewContent` in `Reload` |
| `internal/tui/model.go` | Modify | Carry `Source` through `Result`, pass source to preview |
| `internal/tui/styles.go` | Modify | Add Codex badge style |
| `cmd/claude-sessions/main.go` | Modify | Dispatch exec based on `Source` |
| `testdata/codex/basic_session.jsonl` | Create | Test fixture |
| `testdata/codex/empty_session.jsonl` | Create | Test fixture |
| `testdata/codex/session_index.jsonl` | Create | Test fixture |
| `internal/scanner/codex_scanner_test.go` | Create | Scanner tests |
| `internal/parser/codex_parser_test.go` | Create | Parser tests |

---

### Task 1: Session Model — Add Source Type and Fields

**Files:**
- Modify: `internal/session/session.go:1-48`

- [ ] **Step 1: Add Source type and constants after the imports**

In `internal/session/session.go`, after line 10 (closing paren of imports), add:

```go
// Source identifies which CLI tool created a session.
type Source string

const (
	SourceClaude Source = "claude"
	SourceCodex  Source = "codex"
)
```

- [ ] **Step 2: Add new fields to Session struct**

In `internal/session/session.go`, after the `Model` field (line 46) and before `SearchText`, add:

```go
	Source       Source // "claude" or "codex"
	ThreadName   string // Codex only: session title from session_index.jsonl
	Originator   string // Codex only: "Codex Desktop", etc.
	EditorSource string // Codex only: "vscode", "cli", etc. (may be empty for subagent sessions)
	TokensIn     int    // Codex only: total input tokens
	TokensOut    int    // Codex only: total output tokens
```

- [ ] **Step 3: Run tests to verify no regressions**

Run: `cd /Users/example/claude-sessions && go build ./... && go test ./internal/session/ ./internal/scanner/ ./internal/parser/`
Expected: All PASS — new fields are zero-valued, no existing code breaks.

- [ ] **Step 4: Commit**

```bash
git add internal/session/session.go
git commit -m "feat: add Source type and Codex fields to Session struct"
```

---

### Task 2: Codex JSONL Types

**Files:**
- Create: `internal/session/codex_types.go`

- [ ] **Step 1: Create the Codex types file**

Create `internal/session/codex_types.go`:

```go
package session

import "encoding/json"

// CodexJSONLine is the top-level envelope for each line in a Codex session JSONL file.
type CodexJSONLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"` // "session_meta", "response_item", "event_msg", "turn_context"
	Payload   json.RawMessage `json:"payload"`
}

// CodexSessionMeta is the first line of a Codex session file.
type CodexSessionMeta struct {
	ID         string          `json:"id"`
	CWD        string          `json:"cwd"`
	Originator string          `json:"originator"`  // "Codex Desktop", etc.
	CLIVersion string          `json:"cli_version"`
	Source     json.RawMessage `json:"source"` // string ("vscode") or object (subagent spawn)
}

// CodexSourceString extracts a string value from the polymorphic source field.
// Returns empty string if source is an object or missing.
func CodexSourceString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// CodexResponseItem represents a response_item payload.
type CodexResponseItem struct {
	Type    string          `json:"type"` // "message", "function_call", "function_call_output", "reasoning", "web_search_call", "custom_tool_call", "custom_tool_call_output"
	Role    string          `json:"role"` // "user", "developer", "assistant"
	Content json.RawMessage `json:"content"`
	Name    string          `json:"name"`      // function_call / custom_tool_call: tool name
	Args    string          `json:"arguments"` // function_call: JSON string arguments
	Input   string          `json:"input"`     // custom_tool_call: input string
	CallID  string          `json:"call_id"`
	Action  json.RawMessage `json:"action"` // web_search_call: {type, query, queries}
}

// CodexContentBlock represents a block in response_item message content arrays.
type CodexContentBlock struct {
	Type string `json:"type"` // "output_text", "input_text", "input_image"
	Text string `json:"text"`
}

// CodexEventMsg represents an event_msg payload.
type CodexEventMsg struct {
	Type    string          `json:"type"` // "user_message", "agent_message", "token_count", etc.
	Message string          `json:"message"`
	Info    json.RawMessage `json:"info"` // for token_count events
}

// CodexTokenInfo holds token usage from a token_count event.
type CodexTokenInfo struct {
	TotalTokenUsage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"total_token_usage"`
}

// CodexTurnContext represents a turn_context payload.
type CodexTurnContext struct {
	Model string `json:"model"`
}

// CodexWebSearchAction represents the action field in a web_search_call.
type CodexWebSearchAction struct {
	Type    string   `json:"type"` // "search" or "open_page"
	Query   string   `json:"query"`
	Queries []string `json:"queries"`
	URL     string   `json:"url"` // for open_page
}

// CodexSessionIndexEntry represents one line of session_index.jsonl.
type CodexSessionIndexEntry struct {
	ID         string `json:"id"`
	ThreadName string `json:"thread_name"`
	UpdatedAt  string `json:"updated_at"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/example/claude-sessions && go build ./internal/session/`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/session/codex_types.go
git commit -m "feat: add Codex JSONL deserialization types"
```

---

### Task 3: Test Fixtures for Codex Sessions

**Files:**
- Create: `testdata/codex/basic_session.jsonl`
- Create: `testdata/codex/empty_session.jsonl`
- Create: `testdata/codex/session_index.jsonl`

- [ ] **Step 1: Create testdata/codex directory and basic_session.jsonl**

```bash
mkdir -p /Users/example/claude-sessions/testdata/codex
```

Create `testdata/codex/basic_session.jsonl` — a realistic multi-turn session:

```jsonl
{"timestamp":"2026-04-01T10:00:00.000Z","type":"session_meta","payload":{"id":"019d0000-0000-0000-0000-000000000001","timestamp":"2026-04-01T10:00:00.000Z","cwd":"/Users/test/myproject","originator":"Codex Desktop","cli_version":"0.117.0","source":"vscode"}}
{"timestamp":"2026-04-01T10:00:00.100Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/Users/test/myproject","model":"gpt-5.4","personality":"default"}}
{"timestamp":"2026-04-01T10:00:00.200Z","type":"event_msg","payload":{"type":"task_started"}}
{"timestamp":"2026-04-01T10:00:00.300Z","type":"event_msg","payload":{"type":"user_message","message":"查看项目结构","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-01T10:00:00.400Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"codex"}}}
{"timestamp":"2026-04-01T10:00:01.000Z","type":"response_item","payload":{"type":"reasoning","summary":[],"content":null,"encrypted_content":"xxx"}}
{"timestamp":"2026-04-01T10:00:02.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"ls -la\",\"workdir\":\"/Users/test/myproject\"}","call_id":"call_001"}}
{"timestamp":"2026-04-01T10:00:03.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_001","output":"total 32\ndrwxr-xr-x  5 test  staff  160 Apr  1 10:00 .\n-rw-r--r--  1 test  staff  256 Apr  1 09:00 main.go"}}
{"timestamp":"2026-04-01T10:00:04.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"项目结构如下：\n- main.go — 入口文件"}]}}
{"timestamp":"2026-04-01T10:00:05.000Z","type":"event_msg","payload":{"type":"agent_message","message":"项目结构如下：\n- main.go — 入口文件","phase":"final_answer"}}
{"timestamp":"2026-04-01T10:00:05.100Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":5000,"cached_input_tokens":1000,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":5200},"model_context_window":258400},"rate_limits":{"limit_id":"codex"}}}
{"timestamp":"2026-04-01T10:00:05.200Z","type":"event_msg","payload":{"type":"task_complete"}}
{"timestamp":"2026-04-01T10:00:10.000Z","type":"turn_context","payload":{"turn_id":"turn-2","cwd":"/Users/test/myproject","model":"gpt-5.4"}}
{"timestamp":"2026-04-01T10:00:10.100Z","type":"event_msg","payload":{"type":"task_started"}}
{"timestamp":"2026-04-01T10:00:10.200Z","type":"event_msg","payload":{"type":"user_message","message":"搜索一下 Go 的 test 框架","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-01T10:00:11.000Z","type":"response_item","payload":{"type":"web_search_call","status":"completed","action":{"type":"search","query":"Go testing framework best practices","queries":["Go testing framework best practices","golang test libraries 2026"]}}}
{"timestamp":"2026-04-01T10:00:12.000Z","type":"response_item","payload":{"type":"custom_tool_call","status":"completed","call_id":"call_002","name":"apply_patch","input":"*** Begin Patch\n*** Modify File: main.go\n..."}}
{"timestamp":"2026-04-01T10:00:12.500Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_002","output":"Patch applied successfully."}}
{"timestamp":"2026-04-01T10:00:13.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"已完成搜索和补丁应用。"}]}}
{"timestamp":"2026-04-01T10:00:13.100Z","type":"event_msg","payload":{"type":"agent_message","message":"已完成搜索和补丁应用。","phase":"final_answer"}}
{"timestamp":"2026-04-01T10:00:13.200Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":12000,"cached_input_tokens":5000,"output_tokens":500,"reasoning_output_tokens":100,"total_tokens":12500},"model_context_window":258400},"rate_limits":{"limit_id":"codex"}}}
{"timestamp":"2026-04-01T10:00:13.300Z","type":"event_msg","payload":{"type":"task_complete"}}
{"timestamp":"2026-04-01T10:00:14.000Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>sandbox config here</permissions instructions>"}]}}
{"timestamp":"2026-04-01T10:00:14.100Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"搜索一下 Go 的 test 框架"}]}}
```

- [ ] **Step 2: Create empty_session.jsonl**

Create `testdata/codex/empty_session.jsonl`:

```jsonl
{"timestamp":"2026-04-02T09:00:00.000Z","type":"session_meta","payload":{"id":"019d0000-0000-0000-0000-000000000002","timestamp":"2026-04-02T09:00:00.000Z","cwd":"/Users/test/empty","originator":"Codex Desktop","cli_version":"0.117.0","source":"cli"}}
```

- [ ] **Step 3: Create session_index.jsonl**

Create `testdata/codex/session_index.jsonl`:

```jsonl
{"id":"019d0000-0000-0000-0000-000000000001","thread_name":"查看项目结构和测试框架","updated_at":"2026-04-01T10:00:13.300Z"}
{"id":"019d0000-0000-0000-0000-000000000002","thread_name":"空会话","updated_at":"2026-04-02T09:00:00.000Z"}
```

- [ ] **Step 4: Commit**

```bash
git add testdata/codex/
git commit -m "feat: add Codex session test fixtures"
```

---

### Task 4: Codex Scanner — Session Index and ID Extraction

**Files:**
- Create: `internal/scanner/codex_scanner.go`
- Create: `internal/scanner/codex_scanner_test.go`

- [ ] **Step 1: Write tests for session index loading and ID extraction**

Create `internal/scanner/codex_scanner_test.go`:

```go
package scanner

import (
	"path/filepath"
	"testing"
)

func TestLoadCodexSessionIndex(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "codex", "session_index.jsonl")
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/example/claude-sessions && go test -v -run "TestLoadCodexSessionIndex|TestExtractCodexSessionID" ./internal/scanner/`
Expected: FAIL — functions not defined.

- [ ] **Step 3: Implement LoadCodexSessionIndex and ExtractCodexSessionID**

Create `internal/scanner/codex_scanner.go`:

```go
package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

// codexUUIDRe matches the UUID portion at the end of a Codex session filename.
// Format: rollout-<timestamp>-<UUID>.jsonl where UUID is 5 groups of hex.
var codexUUIDRe = regexp.MustCompile(`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$`)

// ExtractCodexSessionID extracts the UUID from a Codex session filename.
func ExtractCodexSessionID(filename string) string {
	m := codexUUIDRe.FindStringSubmatch(filename)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// LoadCodexSessionIndex reads session_index.jsonl and returns a map of id → thread_name.
func LoadCodexSessionIndex(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	idx := make(map[string]string)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	for sc.Scan() {
		var entry session.CodexSessionIndexEntry
		if json.Unmarshal(sc.Bytes(), &entry) == nil && entry.ID != "" {
			idx[entry.ID] = entry.ThreadName
		}
	}
	return idx, sc.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/example/claude-sessions && go test -v -run "TestLoadCodexSessionIndex|TestExtractCodexSessionID" ./internal/scanner/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/codex_scanner.go internal/scanner/codex_scanner_test.go
git commit -m "feat: add Codex session index loading and ID extraction"
```

---

### Task 5: Codex Scanner — ScanCodexSessionFile

**Files:**
- Modify: `internal/scanner/codex_scanner.go`
- Modify: `internal/scanner/codex_scanner_test.go`

- [ ] **Step 1: Write test for ScanCodexSessionFile**

Append to `internal/scanner/codex_scanner_test.go`:

```go
func TestScanCodexSessionFile(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "codex", "basic_session.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	sess, err := ScanCodexSessionFile(path, info.Size(), map[string]string{
		"019d0000-0000-0000-0000-000000000001": "查看项目结构和测试框架",
	})
	if err != nil {
		t.Fatal(err)
	}

	if sess.ID != "019d0000-0000-0000-0000-000000000001" {
		t.Errorf("ID = %q", sess.ID)
	}
	if sess.Source != session.SourceCodex {
		t.Errorf("Source = %q, want codex", sess.Source)
	}
	if sess.ProjectPath != "/Users/test/myproject" {
		t.Errorf("ProjectPath = %q", sess.ProjectPath)
	}
	if sess.ProjectDir != "myproject" {
		t.Errorf("ProjectDir = %q", sess.ProjectDir)
	}
	if sess.ThreadName != "查看项目结构和测试框架" {
		t.Errorf("ThreadName = %q", sess.ThreadName)
	}
	if sess.Originator != "Codex Desktop" {
		t.Errorf("Originator = %q", sess.Originator)
	}
	if sess.EditorSource != "vscode" {
		t.Errorf("EditorSource = %q", sess.EditorSource)
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
	if sess.TokensIn != 12000 {
		t.Errorf("TokensIn = %d, want 12000", sess.TokensIn)
	}
	if sess.TokensOut != 500 {
		t.Errorf("TokensOut = %d, want 500", sess.TokensOut)
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
	path := filepath.Join("..", "..", "testdata", "codex", "empty_session.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	sess, err := ScanCodexSessionFile(path, info.Size(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sess.MsgCount != 0 {
		t.Errorf("MsgCount = %d, want 0", sess.MsgCount)
	}
	if sess.ProjectPath != "/Users/test/empty" {
		t.Errorf("ProjectPath = %q", sess.ProjectPath)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/example/claude-sessions && go test -v -run "TestScanCodexSessionFile" ./internal/scanner/`
Expected: FAIL — `ScanCodexSessionFile` not defined.

- [ ] **Step 3: Implement ScanCodexSessionFile**

Append to `internal/scanner/codex_scanner.go`:

```go
// ScanCodexSessionFile reads a single Codex JSONL file and extracts session metadata.
func ScanCodexSessionFile(path string, fileSize int64, threadIndex map[string]string) (session.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return session.Session{}, err
	}
	defer f.Close()

	filenameID := ExtractCodexSessionID(filepath.Base(path))

	var (
		id           string
		projectPath  string
		originator   string
		editorSource string
		modelName    string
		firstMsg     string
		lastMsg      string
		startTime    time.Time
		lastTime     time.Time
		msgCount     int
		toolCount    int
		tokensIn     int
		tokensOut    int
		allUserMsgs  []string
	)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for sc.Scan() {
		var line session.CodexJSONLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}

		// Track timestamps
		if line.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
				if startTime.IsZero() || ts.Before(startTime) {
					startTime = ts
				}
				if ts.After(lastTime) {
					lastTime = ts
				}
			}
		}

		switch line.Type {
		case "session_meta":
			var meta session.CodexSessionMeta
			if json.Unmarshal(line.Payload, &meta) == nil {
				id = meta.ID
				projectPath = meta.CWD
				originator = meta.Originator
				editorSource = session.CodexSourceString(meta.Source)
			}

		case "turn_context":
			if modelName == "" {
				var tc session.CodexTurnContext
				if json.Unmarshal(line.Payload, &tc) == nil && tc.Model != "" {
					modelName = tc.Model
				}
			}

		case "event_msg":
			var evt session.CodexEventMsg
			if json.Unmarshal(line.Payload, &evt) != nil {
				continue
			}
			switch evt.Type {
			case "user_message":
				msgCount++
				msg := strings.TrimSpace(evt.Message)
				if msg == "" {
					continue
				}
				msg = strings.Join(strings.Fields(msg), " ")
				allUserMsgs = append(allUserMsgs, msg)
				truncated := msg
				if len([]rune(truncated)) > 100 {
					truncated = string([]rune(truncated)[:100])
				}
				if firstMsg == "" {
					firstMsg = truncated
				}
				lastMsg = truncated
			case "token_count":
				// evt.Info is json.RawMessage via CodexEventMsg; re-parse from payload
				var full struct {
					Type string          `json:"type"`
					Info json.RawMessage `json:"info"`
				}
				if json.Unmarshal(line.Payload, &full) == nil && full.Info != nil && string(full.Info) != "null" {
					var ti session.CodexTokenInfo
					if json.Unmarshal(full.Info, &ti) == nil && ti.TotalTokenUsage != nil {
						tokensIn = ti.TotalTokenUsage.InputTokens
						tokensOut = ti.TotalTokenUsage.OutputTokens
					}
				}
			}

		case "response_item":
			var item session.CodexResponseItem
			if json.Unmarshal(line.Payload, &item) == nil {
				switch item.Type {
				case "function_call", "web_search_call", "custom_tool_call":
					toolCount++
				}
			}
		}
	}

	if id == "" {
		id = filenameID
	}

	projectDir := ""
	if projectPath != "" {
		projectDir = filepath.Base(projectPath)
	}

	threadName := ""
	if threadIndex != nil {
		threadName = threadIndex[id]
	}

	return session.Session{
		ID:           id,
		ProjectDir:   projectDir,
		ProjectPath:  projectPath,
		FirstMsg:     firstMsg,
		LastMsg:      lastMsg,
		StartTime:    startTime,
		LastTime:     lastTime,
		MsgCount:     msgCount,
		ToolCount:    toolCount,
		FileSize:     fileSize,
		FilePath:     path,
		Model:        modelName,
		SearchText:   strings.ToLower(strings.Join(allUserMsgs, " ")),
		Source:       session.SourceCodex,
		ThreadName:   threadName,
		Originator:   originator,
		EditorSource: editorSource,
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/example/claude-sessions && go test -v -run "TestScanCodexSessionFile" ./internal/scanner/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/codex_scanner.go internal/scanner/codex_scanner_test.go
git commit -m "feat: implement ScanCodexSessionFile with metadata extraction"
```

---

### Task 6: Codex Scanner — ScanCodexSessions and ScanAllSessions Integration

**Files:**
- Modify: `internal/scanner/codex_scanner.go`
- Modify: `internal/scanner/scanner.go:154-221`

- [ ] **Step 1: Implement ScanCodexSessions in codex_scanner.go**

Append to `internal/scanner/codex_scanner.go`:

```go
// ScanCodexSessions walks ~/.codex/sessions/ and scans all Codex session JSONL files in parallel.
// Returns nil slice (not error) if the directory doesn't exist.
func ScanCodexSessions() ([]session.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	sessionsDir := filepath.Join(home, ".codex", "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return nil, nil
	}

	indexPath := filepath.Join(home, ".codex", "session_index.jsonl")
	threadIndex, _ := LoadCodexSessionIndex(indexPath) // best-effort
	if threadIndex == nil {
		threadIndex = make(map[string]string)
	}

	type result struct {
		session session.Session
		err     error
	}

	const maxWorkers = 16
	var wg sync.WaitGroup
	ch := make(chan result, 100)
	sem := make(chan struct{}, maxWorkers)

	filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		wg.Add(1)
		go func(fp string, sz int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sess, err := ScanCodexSessionFile(fp, sz, threadIndex)
			ch <- result{sess, err}
		}(path, info.Size())
		return nil
	})

	go func() {
		wg.Wait()
		close(ch)
	}()

	var sessions []session.Session
	for r := range ch {
		if r.err == nil && r.session.MsgCount > 0 {
			sessions = append(sessions, r.session)
		}
	}
	return sessions, nil
}
```

- [ ] **Step 2: Update ScanAllSessions to merge Claude + Codex and tolerate missing dirs**

In `internal/scanner/scanner.go`, replace the `ScanAllSessions` function (lines 154-221) with:

```go
// ScanAllSessions scans both Claude and Codex session directories in parallel.
// Tolerates either source being absent.
func ScanAllSessions() ([]session.Session, error) {
	var allSessions []session.Session

	// Scan Claude sessions
	claudeSessions, err := scanClaudeSessions()
	if err == nil {
		for i := range claudeSessions {
			claudeSessions[i].Source = session.SourceClaude
		}
		allSessions = append(allSessions, claudeSessions...)
	}

	// Scan Codex sessions
	codexSessions, _ := ScanCodexSessions()
	allSessions = append(allSessions, codexSessions...)

	return allSessions, nil
}

// scanClaudeSessions walks ~/.claude/projects/ and scans session JSONL files.
// Returns nil slice if the directory doesn't exist.
func scanClaudeSessions() ([]session.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	baseDir := filepath.Join(home, ".claude", "projects")

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, nil
	}

	type result struct {
		session session.Session
		err     error
	}

	const maxWorkers = 16
	var wg sync.WaitGroup
	ch := make(chan result, 100)
	sem := make(chan struct{}, maxWorkers)

	for _, projEntry := range entries {
		if !projEntry.IsDir() {
			continue
		}
		projDir := projEntry.Name()
		projPath := filepath.Join(baseDir, projDir)

		files, err := os.ReadDir(projPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			filePath := filepath.Join(projPath, f.Name())
			info, err := f.Info()
			if err != nil {
				continue
			}
			wg.Add(1)
			go func(fp, pd string, sz int64) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				sess, err := ScanSessionFile(fp, pd, sz)
				ch <- result{sess, err}
			}(filePath, projDir, info.Size())
		}
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var sessions []session.Session
	for r := range ch {
		if r.err == nil && r.session.MsgCount > 0 {
			sessions = append(sessions, r.session)
		}
	}

	return sessions, nil
}
```

- [ ] **Step 3: Run all scanner tests**

Run: `cd /Users/example/claude-sessions && go test -v ./internal/scanner/`
Expected: All PASS.

- [ ] **Step 4: Run full build**

Run: `cd /Users/example/claude-sessions && go build ./...`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/codex_scanner.go internal/scanner/scanner.go
git commit -m "feat: integrate Codex scanning into ScanAllSessions"
```

---

### Task 7: Codex Parser — Test Fixtures and Tests

**Files:**
- Create: `internal/parser/codex_parser_test.go`

- [ ] **Step 1: Write parser tests**

Create `internal/parser/codex_parser_test.go`:

```go
package parser

import (
	"path/filepath"
	"testing"
)

func TestParseCodexMessages(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "codex", "basic_session.jsonl")
	msgs, err := ParseCodexMessages(path, false)
	if err != nil {
		t.Fatal(err)
	}

	// Expected non-verbose messages:
	// 1. user: "查看项目结构"
	// 2. tool: "exec_command ls -la"
	// 3. assistant: "项目结构如下：..."
	// 4. user: "搜索一下 Go 的 test 框架"
	// 5. tool: WebSearch "Go testing framework best practices"
	// 6. tool: apply_patch
	// 7. assistant: "已完成搜索和补丁应用。"
	// (developer message skipped, user message with role="user" skipped)

	if len(msgs) < 5 {
		t.Fatalf("got %d messages, want >= 5", len(msgs))
	}

	if msgs[0].Role != "user" || msgs[0].Text != "查看项目结构" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}

	// Find tool messages
	toolCount := 0
	for _, m := range msgs {
		if m.Role == "tool" {
			toolCount++
		}
	}
	if toolCount < 3 {
		t.Errorf("tool messages = %d, want >= 3", toolCount)
	}

	// Last message should be assistant
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" {
		t.Errorf("last message role = %q, want assistant", last.Role)
	}
}

func TestParseCodexMessages_Verbose(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "codex", "basic_session.jsonl")

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

	// Verbose should include tool_result messages
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

func TestParseCodexMessages_SkipsReasoning(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "codex", "basic_session.jsonl")
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
	path := filepath.Join("..", "..", "testdata", "codex", "basic_session.jsonl")
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/example/claude-sessions && go test -v -run "TestParseCodex" ./internal/parser/`
Expected: FAIL — `ParseCodexMessages` not defined.

- [ ] **Step 3: Commit test file**

```bash
git add internal/parser/codex_parser_test.go
git commit -m "test: add Codex parser tests"
```

---

### Task 8: Codex Parser — Implementation

**Files:**
- Create: `internal/parser/codex_parser.go`

- [ ] **Step 1: Implement ParseCodexMessages**

Create `internal/parser/codex_parser.go`:

```go
package parser

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

// ParseCodexMessages reads a Codex JSONL file and returns displayable messages.
func ParseCodexMessages(path string, verbose bool) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []Message

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for sc.Scan() {
		var line session.CodexJSONLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}

		var ts time.Time
		if line.Timestamp != "" {
			ts, _ = time.Parse(time.RFC3339Nano, line.Timestamp)
		}

		switch line.Type {
		case "event_msg":
			var evt session.CodexEventMsg
			if json.Unmarshal(line.Payload, &evt) != nil {
				continue
			}
			switch evt.Type {
			case "user_message":
				text := strings.TrimSpace(evt.Message)
				if text != "" {
					messages = append(messages, Message{Role: "user", Text: text, Timestamp: ts})
				}
			case "agent_message":
				// Skip agent_message — we get the same text from response_item/message
			}

		case "response_item":
			var item session.CodexResponseItem
			if json.Unmarshal(line.Payload, &item) != nil {
				continue
			}

			switch item.Type {
			case "message":
				if item.Role == "developer" || item.Role == "user" {
					continue // skip system content and duplicate user messages
				}
				if item.Role == "assistant" {
					text := extractCodexMessageText(item.Content)
					if text != "" {
						messages = append(messages, Message{Role: "assistant", Text: text, Timestamp: ts})
					}
				}

			case "function_call":
				summary := codexToolInfo(item.Name, item.Args)
				messages = append(messages, Message{Role: "tool", Text: summary, Timestamp: ts})

			case "function_call_output":
				if verbose {
					text := extractCodexOutputText(item.Content)
					if text != "" {
						messages = append(messages, Message{Role: "tool_result", Text: text, Timestamp: ts})
					}
				}

			case "web_search_call":
				summary := codexWebSearchInfo(item.Action)
				messages = append(messages, Message{Role: "tool", Text: summary, Timestamp: ts})

			case "custom_tool_call":
				summary := item.Name
				if item.Input != "" {
					first := strings.SplitN(item.Input, "\n", 2)[0]
					summary += " " + first
				}
				messages = append(messages, Message{Role: "tool", Text: summary, Timestamp: ts})

			case "custom_tool_call_output":
				if verbose {
					text := extractCodexOutputText(item.Content)
					if text != "" {
						messages = append(messages, Message{Role: "tool_result", Text: text, Timestamp: ts})
					}
				}

			case "reasoning":
				// Skip — encrypted/hidden
			}
		}
	}

	return messages, nil
}

// extractCodexMessageText extracts text from an assistant message content array.
// Content is an array of blocks like [{"type":"output_text","text":"..."}].
func extractCodexMessageText(raw json.RawMessage) string {
	var blocks []session.CodexContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if (b.Type == "output_text" || b.Type == "input_text") && strings.TrimSpace(b.Text) != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	// Fallback: try as plain string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

// extractCodexOutputText extracts text from a function output content field.
func extractCodexOutputText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

// codexToolInfo returns a one-line summary of a Codex function_call.
func codexToolInfo(name, argsJSON string) string {
	if argsJSON == "" {
		return name
	}
	var params map[string]json.RawMessage
	if json.Unmarshal([]byte(argsJSON), &params) != nil {
		return name
	}
	// Try common parameter names for a one-line summary
	for _, key := range []string{"cmd", "command", "file_path", "path", "query", "url", "pattern"} {
		if v, ok := params[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				return name + " " + s
			}
		}
	}
	return name
}

// codexWebSearchInfo extracts a summary from a web_search_call action.
func codexWebSearchInfo(actionRaw json.RawMessage) string {
	var action session.CodexWebSearchAction
	if json.Unmarshal(actionRaw, &action) != nil {
		return "WebSearch"
	}
	if action.Type == "open_page" && action.URL != "" {
		return "WebFetch " + action.URL
	}
	if action.Query != "" {
		return "WebSearch " + action.Query
	}
	if len(action.Queries) > 0 {
		return "WebSearch " + action.Queries[0]
	}
	return "WebSearch"
}
```

- [ ] **Step 2: Run parser tests**

Run: `cd /Users/example/claude-sessions && go test -v -run "TestParseCodex" ./internal/parser/`
Expected: All PASS.

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/example/claude-sessions && go test ./...`
Expected: All PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/parser/codex_parser.go
git commit -m "feat: implement Codex session parser"
```

---

### Task 9: TUI — LoadPreviewContent Dispatch by Source

**Files:**
- Modify: `internal/tui/helpers.go:156-166`
- Modify: `internal/tui/model.go:283-291` (openPreview)
- Modify: `internal/tui/side_preview.go:82-101` (LoadSession)

- [ ] **Step 1: Update LoadPreviewContent to accept Source**

In `internal/tui/helpers.go`, replace the `LoadPreviewContent` function (lines 156-166):

```go
// LoadPreviewContent parses a session file and returns the formatted preview content.
// Returns the content string and true on success, or ("", false) on failure.
// If verbose is true, tool results and full tool inputs are included.
func LoadPreviewContent(filePath string, width int, source session.Source, verbose ...bool) (string, bool) {
	full := len(verbose) > 0 && verbose[0]
	var msgs []parser.Message
	var err error
	if source == session.SourceCodex {
		msgs, err = parser.ParseCodexMessages(filePath, full)
	} else {
		msgs, err = parser.ParseSessionMessages(filePath, full)
	}
	if err != nil || len(msgs) == 0 {
		return "", false
	}
	return formatPreviewWithColors(msgs, width, full), true
}
```

Add import for `session` package at top of `helpers.go`:

```go
import (
	...
	session "github.com/jinguo998/claude-sessions/internal/app/model"
)
```

- [ ] **Step 2: Update callers — openPreview in model.go**

In `internal/tui/model.go`, update `openPreview` (line 285):

```go
	if content, ok := LoadPreviewContent(sess.FilePath, m.width-4, sess.Source); ok {
```

- [ ] **Step 3: Update callers — Reload in preview.go**

In `internal/tui/preview.go`, update `Reload` (line 63):

```go
	if content, ok := LoadPreviewContent(p.filePath, width, p.session.Source, verbose); ok {
```

- [ ] **Step 4: Update callers — LoadSession in side_preview.go**

In `internal/tui/side_preview.go`, update the `LoadSession` method. The closure at line 94 needs the source. Add `source := sess.Source` before the closure, then use it:

```go
	source := sess.Source
	cmd := func() tea.Msg {
		if content, ok := LoadPreviewContent(filePath, previewWidth, source); ok {
			return SidePreviewLoadedMsg{SessionID: id, Content: content}
		}
		return SidePreviewLoadedMsg{SessionID: id, Content: "(no previewable content)"}
	}
```

- [ ] **Step 5: Build and run all tests**

Run: `cd /Users/example/claude-sessions && go build ./... && go test ./...`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/helpers.go internal/tui/model.go internal/tui/preview.go internal/tui/side_preview.go
git commit -m "feat: dispatch preview parsing by session source"
```

---

### Task 10: TUI — Source Badge and Source Filter

**Files:**
- Modify: `internal/tui/styles.go`
- Modify: `internal/tui/list.go`

- [ ] **Step 1: Add Codex badge style in styles.go**

In `internal/tui/styles.go`, after `borderStyle` (line 127), add:

```go
	claudeBadgeStyle = lipgloss.NewStyle().
		Foreground(adaptive("33", "75")).
		Bold(true)

	codexBadgeStyle = lipgloss.NewStyle().
		Foreground(adaptive("28", "40")).
		Bold(true)
```

- [ ] **Step 2: Add sourceFilter field and type to ListModel**

In `internal/tui/list.go`, add a `sourceFilter` type after the `sortMode` block (after line 37):

```go
type sourceFilter int

const (
	sourceAll    sourceFilter = iota
	sourceClaude
	sourceCodex
	sourceFilterCount
)

func (sf sourceFilter) String() string {
	switch sf {
	case sourceClaude:
		return "Claude"
	case sourceCodex:
		return "Codex"
	default:
		return "All"
	}
}
```

Add field to `ListModel` struct (after `filterProj` at line 64):

```go
	sourceFilter  sourceFilter // All, Claude, Codex
```

- [ ] **Step 3: Add source badge to row rendering**

In `internal/tui/list.go`, inside `renderSessionRows` (around lines 356-365), modify the line construction to include a badge. Replace the compact/full `fmt.Sprintf` blocks:

For compact mode:
```go
			badge := claudeBadgeStyle.Render("C")
			if s.Source == session.SourceCodex {
				badge = codexBadgeStyle.Render("X")
			}
			line = fmt.Sprintf(" %s %s %s %s",
				badge, timeStr, padToWidth(proj, cfg.projWidth), msg)
```

For full mode:
```go
			badge := claudeBadgeStyle.Render("C")
			if s.Source == session.SourceCodex {
				badge = codexBadgeStyle.Render("X")
			}
			msgCount := fmt.Sprintf("%4d", s.MsgCount)
			relTime := relativeTime(s.LastTime)
			line = fmt.Sprintf(" %s %s  %s  %s %s  %s",
				badge, timeStr, padToWidth(proj, cfg.projWidth), padToWidth(msg, cfg.maxMsgW), msgCount, dimStyle.Render(relTime))
```

- [ ] **Step 4: Add F key handler for source filter**

In `internal/tui/list.go`, inside `updateList` (around line 220 after the `"f"` case), add:

```go
	case "F":
		l.sourceFilter = (l.sourceFilter + 1) % sourceFilterCount
		l.applyFilter()
		return l, func() tea.Msg { return FilterChangedMsg{} }
```

- [ ] **Step 5: Update applyFilter to include source filtering**

In `internal/tui/list.go`, inside `applyFilter` (around line 617), add source filter check after the `filterProj` check:

```go
		// Source filter
		if l.sourceFilter == sourceClaude && s.Source != session.SourceClaude {
			continue
		}
		if l.sourceFilter == sourceCodex && s.Source != session.SourceCodex {
			continue
		}
```

- [ ] **Step 6: Update title bar to show source filter**

In `internal/tui/list.go`, update the `View()` title bar right side (line 457) and `CompactView()` title bar (line 526) to include source filter:

```go
	right := fmt.Sprintf("Sort: %s  Source: %s  Filter: %s", l.sortMode, l.sourceFilter, l.filterLabel())
```

- [ ] **Step 7: Update help bar to include F key**

In `list.go` View() help bar (around line 511) and CompactView() help bar (around line 570), add `{"F", l.sourceFilter.String()}` to the help items.

- [ ] **Step 8: Update empty state message**

In `list.go` View() empty state (around line 482), update to show a generic message when both sources are scanned:

```go
		b.WriteString(emptyStyle.Render("No sessions found"))
```

- [ ] **Step 9: Build and verify**

Run: `cd /Users/example/claude-sessions && go build ./...`
Expected: Build succeeds.

- [ ] **Step 10: Commit**

```bash
git add internal/tui/styles.go internal/tui/list.go
git commit -m "feat: add source badge and source filter to session list"
```

---

### Task 11: TUI — Codex Detail Panel Metadata

**Files:**
- Modify: `internal/tui/list.go`

- [ ] **Step 1: Add Codex metadata to compact detail line**

In `internal/tui/list.go`, inside `renderSessionRows` in the compact detail section (around line 385-391), add Codex-specific metadata:

```go
		if cfg.compact {
			var meta []string
			if sess.Model != "" {
				meta = append(meta, sess.Model)
			}
			if sess.EditorSource != "" {
				meta = append(meta, sess.EditorSource)
			}
			meta = append(meta, sess.FormatDuration(), sess.FormatSize(), fmt.Sprintf("%d tools", sess.ToolCount))
			if sess.TokensIn > 0 {
				meta = append(meta, fmt.Sprintf("%dk tok", (sess.TokensIn+sess.TokensOut)/1000))
			}
			detail.WriteString("\n")
			detail.WriteString(dimStyle.Render(" " + strings.Join(meta, " | ")))
```

- [ ] **Step 2: Add Codex metadata to full detail panel**

In the full detail section (around lines 393-424), after the Tools detail line, add:

```go
				if sess.EditorSource != "" {
					details = append(details, detailLabelStyle.Render("Editor: ")+detailValueStyle.Render(sess.EditorSource))
				}
				if sess.TokensIn > 0 || sess.TokensOut > 0 {
					details = append(details, detailLabelStyle.Render("Tokens: ")+detailValueStyle.Render(
						fmt.Sprintf("%d in / %d out", sess.TokensIn, sess.TokensOut)))
				}
```

And after the path line, if ThreadName is available:

```go
				if sess.ThreadName != "" {
					detail.WriteString(" " + detailLabelStyle.Render("Thread: ") + detailValueStyle.Render(sess.ThreadName))
					detail.WriteString("\n")
				}
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/example/claude-sessions && go build ./...`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/list.go
git commit -m "feat: show Codex metadata in session detail panel"
```

---

### Task 12: Resume/Fork Dispatch by Source

**Files:**
- Modify: `internal/tui/model.go:25-31` (Result struct)
- Modify: `internal/tui/model.go` (all Result construction sites)
- Modify: `cmd/claude-sessions/main.go:82-108`

- [ ] **Step 1: Add Source to Result struct**

In `internal/tui/model.go`, add `Source` field to the `Result` struct (line 25-31):

```go
type Result struct {
	Dir       string
	ID        string
	Fork      bool
	SkipPerms bool
	CdOnly    bool
	Source    session.Source
}
```

- [ ] **Step 2: Update all Result construction sites to include Source**

In `model.go`, update each place a `Result` is created:

Line 141 (SessionSelectedMsg):
```go
	m.result = &Result{Dir: msg.Session.ProjectPath, ID: msg.Session.ID, SkipPerms: true, Source: msg.Session.Source}
```

Line 145 (SessionForkMsg):
```go
	m.result = &Result{Dir: msg.Session.ProjectPath, ID: msg.Session.ID, Fork: true, SkipPerms: true, Source: msg.Session.Source}
```

Line 259 (ActionResume):
```go
	m.result = &Result{Dir: sess.ProjectPath, ID: sess.ID, Source: sess.Source}
```

Line 263 (ActionFork):
```go
	m.result = &Result{Dir: sess.ProjectPath, ID: sess.ID, Fork: true, SkipPerms: true, Source: sess.Source}
```

Line 265 (ActionCd):
```go
	m.result = &Result{Dir: sess.ProjectPath, ID: sess.ID, CdOnly: true, Source: sess.Source}
```

- [ ] **Step 3: Update main.go to dispatch by source**

In `cmd/claude-sessions/main.go`, replace lines 82-108 (after the CdOnly block) with:

```go
	shortID := result.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	var binName string
	var args []string

	if result.Source == session.SourceCodex {
		binName = "codex"
		if result.Fork {
			args = []string{"codex", "fork", result.ID}
			fmt.Fprintf(os.Stderr, "\033[2m→ Forking Codex session %s...\033[0m\n", shortID)
		} else {
			args = []string{"codex", "resume", result.ID}
			fmt.Fprintf(os.Stderr, "\033[2m→ Resuming Codex session %s...\033[0m\n", shortID)
		}
	} else {
		binName = "claude"
		args = []string{"claude", "--resume", result.ID}
		if result.Fork {
			args = append(args, "--fork-session")
			fmt.Fprintf(os.Stderr, "\033[2m→ Forking session %s...\033[0m\n", shortID)
		} else {
			fmt.Fprintf(os.Stderr, "\033[2m→ Resuming session %s...\033[0m\n", shortID)
		}
		if result.SkipPerms {
			args = append(args, "--dangerously-skip-permissions")
		}
	}

	binPath, err := exec.LookPath(binName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s not found in PATH: %v\n", binName, err)
		os.Exit(1)
	}

	if err := syscall.Exec(binPath, args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec failed: %v\n", err)
		os.Exit(1)
	}
```

Add import for `session` package in `main.go`:
```go
	session "github.com/jinguo998/claude-sessions/internal/app/model"
```

- [ ] **Step 4: Build and verify**

Run: `cd /Users/example/claude-sessions && go build ./...`
Expected: Build succeeds.

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/example/claude-sessions && go test ./...`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/model.go cmd/claude-sessions/main.go
git commit -m "feat: dispatch resume/fork to correct CLI based on session source"
```

---

### Task 13: Final Integration — Build, Lint, and Manual Test

**Files:** None (verification only)

- [ ] **Step 1: Run full build**

Run: `cd /Users/example/claude-sessions && make build`
Expected: Binary builds successfully.

- [ ] **Step 2: Run all tests**

Run: `cd /Users/example/claude-sessions && make test`
Expected: All PASS.

- [ ] **Step 3: Run lint**

Run: `cd /Users/example/claude-sessions && make lint`
Expected: No issues.

- [ ] **Step 4: Manual smoke test (if TTY available)**

Run `./claude-sessions` in a terminal. Verify:
- Codex sessions appear alongside Claude sessions
- `C`/`X` badges render correctly
- `F` key cycles source filter
- Codex session detail shows ThreadName, EditorSource, token counts
- Preview of Codex sessions renders correctly
- `r` on a Codex session attempts `codex resume`

- [ ] **Step 5: Commit any final fixes if needed**

```bash
git add -A
git commit -m "fix: final integration fixes for Codex session support"
```
