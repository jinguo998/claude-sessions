package codex

import "encoding/json"

// CodexJSONLine is the top-level envelope for each line in a Codex session JSONL file.
type CodexJSONLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"` // "session_meta", "response_item", "event_msg", "turn_context"
	Payload   json.RawMessage `json:"payload"`
}

// CodexSessionMeta is the first line of a Codex session file.
type CodexSessionMeta struct {
	SessionID      string          `json:"session_id"`
	ID             string          `json:"id"`
	ParentThreadID string          `json:"parent_thread_id"`
	ThreadSource   string          `json:"thread_source"`
	CWD            string          `json:"cwd"`
	Originator     string          `json:"originator"`
	CLIVersion     string          `json:"cli_version"`
	Source         json.RawMessage `json:"source"` // string ("vscode") or object (subagent spawn)
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

func CodexSourceSubagentOther(raw json.RawMessage) string {
	var obj struct {
		Subagent struct {
			Other string `json:"other"`
		} `json:"subagent"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Subagent.Other
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
	Output  string          `json:"output"`    // function_call_output / custom_tool_call_output: result string
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
	Info    json.RawMessage `json:"info"` // for token_count events (may be null)
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
