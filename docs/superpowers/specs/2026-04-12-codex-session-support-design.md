# Codex Session Support

Add support for browsing, previewing, and resuming OpenAI Codex CLI sessions alongside existing Claude Code sessions.

## Approach

Parallel scanner/parser with a `Source` field on `Session`. New `codex_scanner.go` and `codex_parser.go` files alongside existing ones. No refactoring of existing Claude code — purely additive. The TUI gains a source badge per row, a source filter, and resume/fork dispatch based on source.

## Session Model Changes

**File:** `internal/session/session.go`

Add a `Source` type and new fields to `Session`:

```go
type Source string

const (
    SourceClaude Source = "claude"
    SourceCodex  Source = "codex"
)
```

New fields on `Session`:

| Field | Type | Description |
|-------|------|-------------|
| `Source` | `Source` | `"claude"` or `"codex"` |
| `ThreadName` | `string` | Codex only: auto-generated session title from `session_index.jsonl` |
| `Originator` | `string` | Codex only: launch origin — `"Codex Desktop"`, etc. (from `session_meta.originator`) |
| `EditorSource` | `string` | Codex only: editor context — `"vscode"`, etc. (from `session_meta.source`) |
| `TokensIn` | `int` | Codex only: total input tokens (from last `token_count` event with non-null `info`) |
| `TokensOut` | `int` | Codex only: total output tokens |

These fields are zero-valued for Claude sessions.

## Codex JSONL Types

**New file:** `internal/session/codex_types.go`

Codex sessions are stored at `~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<UUID>.jsonl`. Each line is a JSON object with structure `{timestamp, type, payload}`.

### Top-level envelope

```go
type CodexJSONLine struct {
    Timestamp string          `json:"timestamp"`
    Type      string          `json:"type"`      // "session_meta", "response_item", "event_msg", "turn_context"
    Payload   json.RawMessage `json:"payload"`
}
```

### Payload types

**session_meta** (1 per session, first line):
```go
type CodexSessionMeta struct {
    ID         string `json:"id"`
    CWD        string `json:"cwd"`
    Originator string `json:"originator"` // "Codex Desktop", etc.
    CLIVersion string `json:"cli_version"`
    Source     string `json:"source"`     // "vscode", etc.
}
```

**response_item** (subtypes via `payload.type`):
```go
type CodexResponseItem struct {
    Type      string          `json:"type"`      // "message", "function_call", "function_call_output", "reasoning", "web_search_call", "custom_tool_call", "custom_tool_call_output"
    Role      string          `json:"role"`      // "user", "developer", "assistant"
    Content   json.RawMessage `json:"content"`   // varies: array of {type, text} for messages, string for outputs
    Name      string          `json:"name"`      // tool name for function_call
    Arguments string          `json:"arguments"` // JSON string for function_call
    CallID    string          `json:"call_id"`   // for function_call / function_call_output
}
```

Roles: `developer` = system/permission content (filter out), `user` = user input, `assistant` = model output.

**event_msg** (subtypes via `payload.type`):
```go
type CodexEventMsg struct {
    Type    string          `json:"type"`    // "user_message", "agent_message", "token_count", "task_started", "task_complete", "turn_aborted"
    Message string          `json:"message"` // for user_message and agent_message
}
```

**token_count** (nested in event_msg, `info` may be null):
```go
type CodexTokenInfo struct {
    Info *struct {
        TotalTokenUsage struct {
            InputTokens     int `json:"input_tokens"`
            OutputTokens    int `json:"output_tokens"`
            ReasoningTokens int `json:"reasoning_output_tokens"`
        } `json:"total_token_usage"`
    } `json:"info"`
}
```

**turn_context** (1 per turn):
```go
type CodexTurnContext struct {
    Model string `json:"model"`
}
```

## Codex Scanner

**New file:** `internal/scanner/codex_scanner.go`

### Session discovery

1. Load `~/.codex/session_index.jsonl` into `map[string]string` (id → thread_name) for quick lookup.
2. Walk `~/.codex/sessions/` recursively for `*.jsonl` files.
3. Extract UUID from filename: `rollout-<timestamp>-<UUID>.jsonl` → UUID is the session ID.

### Metadata extraction (`ScanCodexSessionFile`)

Same bounded-goroutine pattern as Claude (max 16 workers). Per file:

| Source | Extracted fields |
|--------|-----------------|
| `session_meta` | `ID` (from payload, cross-check with filename), `ProjectPath` (from `cwd`), `ProjectDir` (basename of `cwd`), `Originator`, `EditorSource` |
| `event_msg/user_message` | Turn counting, `FirstMsg`, `LastMsg`, `SearchText` (all user messages joined, lowercased) |
| `event_msg/token_count` (last with non-null `info`) | `TokensIn`, `TokensOut` |
| `turn_context` (first encountered) | `Model` |
| `response_item/function_call`, `web_search_call`, `custom_tool_call` | `ToolCount` (count of all tool invocations) |
| Top-level `timestamp` (first and last lines) | `StartTime`, `LastTime` |

`MsgCount` counts conversation turns: incremented on each `event_msg/user_message`.

`ThreadName` is looked up from the session index map using the session ID.

`Source` is set to `SourceCodex` for all sessions from this scanner.

### Integration with `ScanAllSessions`

The existing `ScanAllSessions` function gains a second phase: after scanning Claude sessions, call `ScanCodexSessions()`, append results to the same slice. Each source scans independently and tolerates the other being absent — if `~/.claude/projects/` doesn't exist, Codex sessions are still returned (and vice versa). Missing directories produce an empty slice, not an error.

## Codex Parser

**New file:** `internal/parser/codex_parser.go`

**Function:** `ParseCodexMessages(path string, verbose bool) ([]Message, error)`

### Message mapping

| Codex type | Condition | Maps to `Message.Role` | Text source |
|------------|-----------|------------------------|-------------|
| `event_msg` | `type: "user_message"` | `"user"` | `payload.message` |
| `response_item` | `type: "message"`, role `"assistant"` | `"assistant"` | Extract text from `payload.content` array (`input_text` blocks) |
| `response_item` | `type: "function_call"` | `"tool"` | Tool name + one-line summary from arguments (same style as Claude's `toolInfo()`) |
| `response_item` | `type: "function_call_output"` | `"tool_result"` | Verbose mode only |
| `response_item` | `type: "web_search_call"` | `"tool"` | "WebSearch: <query>" |
| `response_item` | `type: "custom_tool_call"` | `"tool"` | Tool name + one-line summary |
| `response_item` | `type: "custom_tool_call_output"` | `"tool_result"` | Verbose mode only |
| `event_msg` | `type: "agent_message"` | `"assistant"` | `payload.message` |

**Skipped types:**
- `response_item/reasoning` — encrypted/hidden, same as Claude's thinking blocks
- `response_item/message` with role `"developer"` — system/permission content
- `response_item/message` with role `"user"` — duplicates of user input already captured via `event_msg/user_message`
- `turn_context`, `token_count`, `task_started`, `task_complete`, `turn_aborted` — metadata

**Verbose mode:** Include `function_call_output` and `custom_tool_call_output` with full content. Show full `arguments` in function_call entries.

### Dispatch

Both preview paths (`preview.go` and `side_preview.go`) funnel through `LoadPreviewContent()` in `helpers.go`. The dispatch happens there: check `session.Source` and call `ParseCodexMessages` or `ParseSessionMessages` accordingly. Single dispatch point, no duplication. No changes needed to the existing `ParseSessionMessages` signature.

## TUI Changes

### Source badge (list.go)

Add a 3-char column before the project column:

- Claude sessions: `C` in the existing accent/blue color
- Codex sessions: `X` in green

### Source filter (list.go)

New `sourceFilter` field on `ListModel` cycling through: All → Claude → Codex → All.

Triggered by `F` (shift-f) key. Displayed in the title bar alongside existing sort/filter labels.

Applied in `applyFilter()` — sessions where `Source` doesn't match the filter are excluded.

### Detail panel (list.go)

For Codex sessions, the detail area shows:
- `ThreadName` as a subtitle if available
- `EditorSource` in the metadata line (e.g. "vscode") — shows editor context, more useful than `Originator`
- Token usage: `TokensIn`/`TokensOut` in the metadata line

### Resume/Fork (model.go, cmd/claude-sessions/main.go)

Dispatch based on `session.Source`:

| Source | Resume command | Fork command |
|--------|---------------|--------------|
| Claude | `claude --resume <id>` | `claude --continue <id>` |
| Codex | `codex resume <id>` | `codex fork <id>` |

Both use `syscall.Exec` to replace the process. `os.Chdir(session.ProjectPath)` works for both since Codex sessions have `cwd` mapped to `ProjectPath`.

Currently `model.go` returns only ID/Fork/Dir to `main.go`. The `Source` field must also be carried through — add it to the TUI result struct so `main.go` can dispatch the correct binary.

### Search

No changes. `SearchText` is populated the same way for both sources: all user messages joined and lowercased.

## Testing

### Test fixtures

Add `testdata/codex/` with sample JSONL files:
- `basic_session.jsonl` — typical session with session_meta, user messages, assistant responses, function calls, token counts (both `info:null` and populated `info`)
- `empty_session.jsonl` — session_meta only, no conversation
- `multi_turn.jsonl` — multiple turns with turn_context entries, includes `web_search_call`, `custom_tool_call`, and `custom_tool_call_output`

Add `testdata/codex/session_index.jsonl` — sample index with thread names.

### Scanner tests (`internal/scanner/codex_scanner_test.go`)

- `TestScanCodexSessionFile` — extracts correct metadata (id, cwd→ProjectPath, timestamps, message count, model, tool count, token usage, originator)
- `TestScanCodexSessionFile_Empty` — handles session with no conversation turns
- `TestScanCodexSessionFile_MalformedLine` — skips bad lines gracefully
- `TestLoadSessionIndex` — parses session_index.jsonl into map correctly
- `TestExtractCodexSessionID` — extracts UUID from various filename patterns

### Parser tests (`internal/parser/codex_parser_test.go`)

- `TestParseCodexMessages` — maps all message types to correct roles and text
- `TestParseCodexMessages_Verbose` — includes function_call_output in verbose mode
- `TestParseCodexMessages_SkipsReasoning` — reasoning blocks excluded
- `TestParseCodexMessages_SkipsDeveloper` — developer role messages excluded
- `TestParseCodexMessages_ToolSummary` — function_call produces correct one-line summary

### Integration tests

- `TestScanAllSessions_Mixed` — verifies Claude + Codex sessions are merged and sortable
- `TestScanAllSessions_MissingSource` — verifies one source missing (no `~/.codex/` or no `~/.claude/`) returns the other source's sessions without error
- `TestResumeCommand` — returns correct binary + args for each source
- `TestSourceFilter` — filter logic correctly includes/excludes by source

## Files Changed

| File | Change |
|------|--------|
| `internal/session/session.go` | Add `Source` type, constants, new fields (`Source`, `ThreadName`, `Originator`, `EditorSource`, `TokensIn`, `TokensOut`) on `Session` |
| `internal/session/codex_types.go` | **New.** Codex JSONL deserialization types |
| `internal/scanner/scanner.go` | `ScanAllSessions` calls Codex scanner and merges; tolerate missing source directories |
| `internal/scanner/codex_scanner.go` | **New.** Codex session discovery and metadata extraction |
| `internal/parser/parser.go` | Minor: add dispatch helper or export for source-based selection |
| `internal/parser/codex_parser.go` | **New.** Codex message parsing |
| `internal/tui/list.go` | Source badge column, source filter (`F` key), filter display, Codex metadata in detail panel |
| `internal/tui/messages.go` | Add source filter message type if needed |
| `internal/tui/model.go` | Resume/fork dispatch based on source |
| `internal/tui/styles.go` | Add Codex badge color style |
| `internal/tui/helpers.go` | Token usage formatting helper, parser dispatch in `LoadPreviewContent()` |
| `cmd/claude-sessions/main.go` | Resume/fork exec dispatch based on source |
| `internal/scanner/codex_scanner_test.go` | **New.** Scanner tests |
| `internal/parser/codex_parser_test.go` | **New.** Parser tests |
| `testdata/codex/*.jsonl` | **New.** Test fixtures |
