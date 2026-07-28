package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type Message = domain.ConversationTurn

// contentBlock represents one item in an assistant's content array.
type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`  // tool_use: tool name
	Input json.RawMessage `json:"input"` // tool_use: tool input
}

// extractStringParam extracts a string value from a JSON params map.
func extractStringParam(params map[string]json.RawMessage, key string) string {
	if v, ok := params[key]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	}
	return ""
}

// toolInfo returns a full human-readable description of a tool_use block.
// Always returns complete content — truncation is the renderer's job.
func toolInfo(name string, input json.RawMessage) string {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(input, &params); err != nil {
		return name
	}
	get := func(key string) string { return extractStringParam(params, key) }

	switch name {
	case "Read", "NotebookEdit":
		if p := get("file_path"); p != "" {
			return name + " " + p
		}
	case "Write":
		s := name + " " + get("file_path")
		if c := get("content"); c != "" {
			s += "\n" + c
		}
		return s
	case "Edit":
		s := name + " " + get("file_path")
		if old := get("old_string"); old != "" {
			s += "\n- " + old
		}
		if nw := get("new_string"); nw != "" {
			s += "\n+ " + nw
		}
		return s
	case "Bash", "bash":
		if c := get("command"); c != "" {
			return name + " " + c
		}
	case "Grep":
		s := name
		if p := get("pattern"); p != "" {
			s += " " + p
		}
		if d := get("path"); d != "" {
			s += " in " + d
		}
		return s
	case "Glob":
		if p := get("pattern"); p != "" {
			return name + " " + p
		}
	case "Agent":
		s := name
		if d := get("description"); d != "" {
			s += " " + d
		}
		if p := get("prompt"); p != "" {
			s += "\n" + p
		}
		return s
	case "ToolSearch", "WebSearch":
		if q := get("query"); q != "" {
			return name + " " + q
		}
	case "WebFetch":
		if u := get("url"); u != "" {
			return name + " " + u
		}
	case "Skill":
		if sk := get("skill"); sk != "" {
			return name + " " + sk
		}
	case "TaskCreate", "TaskUpdate", "TaskOutput":
		if s := get("subject"); s != "" {
			return name + " " + s
		}
		if id := get("taskId"); id != "" {
			return name + " #" + id
		}
		if id := get("task_id"); id != "" {
			return name + " #" + id
		}
	case "TaskStop", "TaskGet":
		if id := get("taskId"); id != "" {
			return name + " #" + id
		}
		if id := get("task_id"); id != "" {
			return name + " #" + id
		}
	case "StructuredOutput":
		return structuredOutputInfo(name, params)
	case "AskUserQuestion":
		return askUserQuestionInfo(name, params)
	case "Monitor":
		if c := get("command"); c != "" {
			return name + " " + firstLine(c)
		}
		if d := get("description"); d != "" {
			return name + " " + firstLine(d)
		}
		if target := get("target"); target != "" {
			return name + " " + target
		}
		if id := monitorID(params); id != "" {
			return name + " " + id
		}
	case "ScheduleWakeup":
		return joinToolParts(name, suffixIfSet(extractScalarParam(params, "delaySeconds"), "s"), get("reason"))
	case "Workflow":
		for _, key := range []string{"name", "script", "scriptPath", "resumeFromRunId", "description"} {
			if v := get(key); v != "" {
				return name + " " + firstLine(v)
			}
		}
	case "TaskList":
		if q := get("query"); q != "" {
			return name + " " + q
		}
	case "SendUserMessage":
		return joinToolParts(name, get("status"), firstLine(get("message")))
	case "Artifact":
		return joinToolParts(name, get("label"), get("file_path"))
	case "SendUserFile":
		if n := extractArrayLen(params, "files"); n > 0 {
			return joinToolParts(name, strconv.Itoa(n), "files")
		}
		return joinToolParts(name, get("status"), firstLine(get("caption")))
	case "ExitPlanMode":
		if p := get("planFilePath"); p != "" {
			return name + " " + p
		}
		if p := get("plan"); p != "" {
			return name + " " + firstLine(p)
		}
	default:
		// MCP and other tools: try common parameter names
		for _, key := range []string{"query", "url", "title", "name", "path", "command", "prompt", "description"} {
			if v := get(key); v != "" {
				return name + "\n  " + key + ": " + v
			}
		}
	}
	return name
}

func structuredOutputInfo(name string, params map[string]json.RawMessage) string {
	get := func(key string) string { return extractStringParam(params, key) }
	for _, key := range []string{"summary", "verdict_summary", "reason", "reasoning", "notes", "explanation", "status", "verdict", "severity", "file", "module", "interface", "group"} {
		if v := get(key); v != "" {
			return name + " " + firstLine(v)
		}
	}
	for _, key := range []string{"refuted", "ok"} {
		if v := extractScalarParam(params, key); v != "" {
			return joinToolParts(name, key, v)
		}
	}
	for _, key := range []string{"findings", "issues", "claims", "commentsRemoved", "filesEdited", "perFile", "fixed", "skipped", "results", "evidence"} {
		if n := extractArrayLen(params, key); n > 0 {
			return joinToolParts(name, strconv.Itoa(n), key)
		}
	}
	for _, key := range []string{"confidence", "counterSource"} {
		if v := get(key); v != "" {
			return name + " " + firstLine(v)
		}
	}
	return name
}

func monitorID(params map[string]json.RawMessage) string {
	for _, key := range []string{"task_id", "taskId"} {
		if id := extractStringParam(params, key); id != "" {
			return "#" + id
		}
	}
	for _, key := range []string{"bash_id", "bashId", "shellId"} {
		if id := extractStringParam(params, key); id != "" {
			return id
		}
	}
	return ""
}

func askUserQuestionInfo(name string, params map[string]json.RawMessage) string {
	raw, ok := params["questions"]
	if !ok {
		return name
	}
	var questions []struct {
		Header   string `json:"header"`
		Question string `json:"question"`
	}
	if json.Unmarshal(raw, &questions) != nil || len(questions) == 0 {
		return name
	}
	first := strings.TrimSpace(questions[0].Question)
	if first == "" {
		first = strings.TrimSpace(questions[0].Header)
	}
	count := strconv.Itoa(len(questions)) + " questions:"
	if len(questions) == 1 {
		count = "1 question:"
	}
	return joinToolParts(name, count, firstLine(first))
}

func extractScalarParam(params map[string]json.RawMessage, key string) string {
	raw, ok := params[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return strconv.Itoa(n)
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return strconv.FormatBool(b)
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return ""
}

func extractArrayLen(params map[string]json.RawMessage, key string) int {
	raw, ok := params[key]
	if !ok {
		return 0
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) == nil {
		return len(values)
	}
	return 0
}

func joinToolParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return strings.Join(out, " ")
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return text
}

func suffixIfSet(text, suffix string) string {
	if text == "" {
		return ""
	}
	return text + suffix
}

// toolResultBlock represents a tool_result entry in user message content arrays.
type toolResultBlock struct {
	Type      string          `json:"type"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// extractToolResultText extracts displayable text from a tool_result block.
// Content can be a plain string or an array like [{"type":"text","text":"..."}].
func extractToolResultText(b toolResultBlock) string {
	var text string
	if json.Unmarshal(b.Content, &text) == nil {
		return strings.TrimSpace(text)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(b.Content, &blocks) == nil {
		var parts []string
		for _, bl := range blocks {
			if bl.Type == "text" && strings.TrimSpace(bl.Text) != "" {
				parts = append(parts, strings.TrimSpace(bl.Text))
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ParseSessionMessages reads a full JSONL file and returns user/assistant text messages.
// In summary mode (full=false), tool_result blocks are skipped and tool_use shows one-line summaries.
// In full mode (full=true), tool results are included and tool_use shows full input details.
func ParseSessionMessages(path string, full ...bool) ([]Message, error) {
	verbose := len(full) > 0 && full[0]
	return parseMessages(path, verbose)
}

func parseMessages(path string, verbose bool) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []Message

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		messages = appendSessionLineMessages(messages, scanner.Bytes(), verbose)
	}

	if err := scanner.Err(); err != nil {
		// Return what we have — partial preview is better than nothing
		return messages, nil
	}

	return messages, nil
}

// ParseSessionMessagesTail returns only recent displayable messages by reading
// from the end of a session file. It is intended for small side previews.
func ParseSessionMessagesTail(path string, verbose bool, maxMessages int, maxBytes int64) ([]Message, error) {
	return source.ParseTailMessages(path, verbose, maxMessages, maxBytes, appendSessionLineMessages)
}

func (a Adapter) ParsePreview(ctx context.Context, sess domain.Session, opts source.PreviewOptions) ([]domain.ConversationTurn, error) {
	_ = ctx
	return ParseSessionMessages(sess.FilePath, opts.Verbose)
}

func (a Adapter) ParsePreviewTail(ctx context.Context, sess domain.Session, opts source.TailOptions) ([]domain.ConversationTurn, error) {
	_ = ctx
	return ParseSessionMessagesTail(sess.FilePath, opts.Verbose, opts.MaxMessages, opts.MaxBytes)
}

func appendSessionLineMessages(messages []Message, raw []byte, verbose bool) []Message {
	var line jsonLine
	if err := json.Unmarshal(raw, &line); err != nil {
		return messages
	}

	if line.Message == nil {
		return messages
	}

	var ts time.Time
	if line.Timestamp != "" {
		ts, _ = time.Parse(time.RFC3339Nano, line.Timestamp)
	}

	switch line.Type {
	case "user":
		if line.IsMeta {
			return messages
		}
		// User content can be a plain string or an array (tool_result blocks)
		var content string
		if err := json.Unmarshal(line.Message.Content, &content); err != nil {
			// Try as array of content blocks (tool_result messages)
			var blocks []toolResultBlock
			if json.Unmarshal(line.Message.Content, &blocks) == nil {
				if verbose {
					for _, b := range blocks {
						if b.Type == "tool_result" {
							text := extractToolResultText(b)
							if text != "" {
								messages = append(messages, Message{
									Role:      "tool_result",
									Text:      text,
									Timestamp: ts,
								})
							}
						}
					}
				}
				return messages
			}
			return messages
		}
		content = strings.TrimSpace(content)
		if content == "" || isSystemContent(content) {
			return messages
		}
		if strings.HasPrefix(content, "<command-") {
			content = extractCommandArgs(content)
		}
		content = strings.TrimSpace(content)
		if content == "" {
			return messages
		}
		messages = append(messages, Message{Role: "user", Text: content, Timestamp: ts})

	case "assistant":
		var blocks []contentBlock
		if err := json.Unmarshal(line.Message.Content, &blocks); err == nil {
			var texts []string
			for _, b := range blocks {
				if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
					texts = append(texts, b.Text)
				} else if b.Type == "tool_use" && b.Name != "" {
					messages = append(messages, Message{
						Role:      "tool",
						Text:      toolInfo(b.Name, b.Input),
						Timestamp: ts,
					})
				}
			}
			if len(texts) > 0 {
				messages = append(messages, Message{
					Role:      "assistant",
					Text:      strings.Join(texts, "\n"),
					Timestamp: ts,
				})
			}
		} else {
			// Fallback: content might be a plain string
			var plainText string
			if json.Unmarshal(line.Message.Content, &plainText) == nil && strings.TrimSpace(plainText) != "" {
				messages = append(messages, Message{
					Role:      "assistant",
					Text:      strings.TrimSpace(plainText),
					Timestamp: ts,
				})
			}
		}
	}
	return messages
}
