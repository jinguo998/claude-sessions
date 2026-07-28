package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type Message = domain.ConversationTurn

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
		messages = appendCodexLineMessages(messages, sc.Bytes(), verbose)
	}

	return mergeCodexApprovalEvents(path, messages, 0), nil
}

// ParseCodexMessagesTail returns only recent displayable messages by reading
// from the end of a Codex session file. It is intended for small side previews.
func ParseCodexMessagesTail(path string, verbose bool, maxMessages int, maxBytes int64) ([]Message, error) {
	messages, err := source.ParseTailMessages(path, verbose, maxMessages, maxBytes, appendCodexLineMessages)
	if err != nil {
		return nil, err
	}
	return mergeCodexApprovalEvents(path, messages, maxMessages), nil
}

func (a Adapter) ParsePreview(ctx context.Context, sess domain.Session, opts source.PreviewOptions) ([]domain.ConversationTurn, error) {
	_ = ctx
	return ParseCodexMessages(sess.FilePath, opts.Verbose)
}

func (a Adapter) ParsePreviewTail(ctx context.Context, sess domain.Session, opts source.TailOptions) ([]domain.ConversationTurn, error) {
	_ = ctx
	return ParseCodexMessagesTail(sess.FilePath, opts.Verbose, opts.MaxMessages, opts.MaxBytes)
}

func appendCodexLineMessages(messages []Message, raw []byte, verbose bool) []Message {
	var line CodexJSONLine
	if json.Unmarshal(raw, &line) != nil {
		return messages
	}

	var ts time.Time
	if line.Timestamp != "" {
		ts, _ = time.Parse(time.RFC3339Nano, line.Timestamp)
	}

	switch line.Type {
	case "event_msg":
		var evt CodexEventMsg
		if json.Unmarshal(line.Payload, &evt) != nil {
			return messages
		}
		switch evt.Type {
		case "user_message":
			text := strings.TrimSpace(evt.Message)
			text = stripTaskPrefix(text)
			if summary, ok := codexApprovalRequestSummary(text); ok {
				text = summary
			}
			if text != "" {
				messages = append(messages, Message{Role: "user", Text: text, Timestamp: ts})
			}
			// agent_message intentionally skipped — same text is already captured
			// via response_item/message with role "assistant", avoiding duplicates.
		}

	case "response_item":
		var item CodexResponseItem
		if json.Unmarshal(line.Payload, &item) != nil {
			return messages
		}

		switch item.Type {
		case "message":
			if item.Role == "developer" {
				return messages
			}
			if item.Role == "user" {
				text := extractCodexMessageText(item.Content)
				if summary, ok := codexApprovalRequestSummary(text); ok {
					messages = append(messages, Message{Role: "user", Text: summary, Timestamp: ts})
				}
				return messages
			}
			if item.Role == "assistant" {
				text := extractCodexMessageText(item.Content)
				if summary, ok := codexApprovalDecisionSummary(text); ok {
					text = summary
				}
				if text != "" {
					messages = append(messages, Message{Role: "assistant", Text: text, Timestamp: ts})
				}
			}

		case "function_call":
			summary := codexToolInfo(item.Name, item.Args)
			messages = append(messages, Message{Role: "tool", Text: summary, Timestamp: ts})

		case "function_call_output":
			if verbose {
				text := strings.TrimSpace(item.Output)
				if text == "" {
					text = extractCodexOutputText(item.Content)
				}
				if text != "" {
					messages = append(messages, Message{Role: "tool_result", Text: text, Timestamp: ts})
				}
			}

		case "web_search_call":
			summary := codexWebSearchInfo(item.Action)
			messages = append(messages, Message{Role: "tool", Text: summary, Timestamp: ts})

		case "custom_tool_call":
			summary := codexCustomToolInfo(item.Name, item.Input)
			messages = append(messages, Message{Role: "tool", Text: summary, Timestamp: ts})

		case "custom_tool_call_output":
			if verbose {
				text := strings.TrimSpace(item.Output)
				if text == "" {
					text = extractCodexOutputText(item.Content)
				}
				if text != "" {
					messages = append(messages, Message{Role: "tool_result", Text: text, Timestamp: ts})
				}
			}
		}
	}
	return messages
}

func extractCodexMessageText(raw json.RawMessage) string {
	var blocks []CodexContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if (b.Type == "output_text" || b.Type == "input_text") && strings.TrimSpace(b.Text) != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func extractCodexOutputText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func mergeCodexApprovalEvents(parentPath string, messages []Message, maxMessages int) []Message {
	meta, ok := readCodexSessionMeta(parentPath)
	if !ok || (meta.ID == "" && meta.SessionID == "") || codexMetaIsGuardian(meta) {
		return messages
	}
	events := codexApprovalEventsForParent(parentPath, codexParentIDs(meta))
	if len(events) == 0 {
		return messages
	}
	out := append([]Message{}, messages...)
	out = append(out, events...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp.IsZero() || out[j].Timestamp.IsZero() {
			return false
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	if maxMessages > 0 && len(out) > maxMessages {
		out = out[len(out)-maxMessages:]
	}
	return out
}

func codexApprovalEventsForParent(parentPath string, parentIDs map[string]struct{}) []Message {
	root := codexSessionsRoot(parentPath)
	var events []Message
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") || path == parentPath {
			return nil
		}
		meta, ok := readCodexSessionMeta(path)
		if !ok || !codexMetaIsGuardian(meta) {
			return nil
		}
		if _, ok := parentIDs[meta.ParentThreadID]; !ok {
			return nil
		}
		events = append(events, codexApprovalEventsFromFile(path)...)
		return nil
	})
	return events
}

func codexParentIDs(meta CodexSessionMeta) map[string]struct{} {
	ids := make(map[string]struct{}, 2)
	if meta.ID != "" {
		ids[meta.ID] = struct{}{}
	}
	if meta.SessionID != "" {
		ids[meta.SessionID] = struct{}{}
	}
	return ids
}

func codexApprovalEventsFromFile(path string) []Message {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var events []Message
	var request string
	var decision string
	var ts time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var line CodexJSONLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		var lineTS time.Time
		if line.Timestamp != "" {
			lineTS, _ = time.Parse(time.RFC3339Nano, line.Timestamp)
		}
		msgs := appendCodexLineMessages(nil, sc.Bytes(), false)
		for _, msg := range msgs {
			switch {
			case strings.HasPrefix(msg.Text, "Approval request: "):
				nextRequest := strings.TrimPrefix(msg.Text, "Approval request: ")
				if request != "" && decision == "" && request == nextRequest {
					if !lineTS.IsZero() {
						ts = lineTS
					}
					continue
				}
				if request != "" || decision != "" {
					if event, ok := codexApprovalEvent(request, decision, ts); ok {
						events = append(events, event)
					}
					decision = ""
				}
				request = nextRequest
				if !lineTS.IsZero() {
					ts = lineTS
				}
			case strings.HasPrefix(msg.Text, "Approval decision: "):
				if request == "" {
					continue
				}
				decision = strings.TrimPrefix(msg.Text, "Approval decision: ")
				if !lineTS.IsZero() {
					ts = lineTS
				}
				if event, ok := codexApprovalEvent(request, decision, ts); ok {
					events = append(events, event)
				}
				request = ""
				decision = ""
			}
		}
	}
	if event, ok := codexApprovalEvent(request, decision, ts); ok {
		events = append(events, event)
	}
	return events
}

func codexApprovalEvent(request, decision string, ts time.Time) (Message, bool) {
	if decision == "" {
		return Message{}, false
	}
	text := approvalDisplayLabel(decision)
	if request != "" {
		text += " " + request
	}
	if risk := approvalRiskLabel(decision); risk != "" && risk != "low" {
		text += " (" + risk + " risk)"
	}
	return Message{Role: "approval", Text: text, Timestamp: ts}, true
}

func approvalDisplayLabel(decision string) string {
	outcome := approvalOutcome(decision)
	switch outcome {
	case "allow":
		return "approved"
	case "deny", "denied", "reject", "rejected":
		return "denied"
	case "":
		return "approval"
	default:
		return "approval " + outcome
	}
}

func approvalOutcome(decision string) string {
	label := approvalDecisionLabel(decision)
	if idx := strings.Index(label, " ("); idx >= 0 {
		label = label[:idx]
	}
	return strings.TrimSpace(label)
}

func approvalRiskLabel(decision string) string {
	label := approvalDecisionLabel(decision)
	start := strings.LastIndex(label, "(")
	if start < 0 || !strings.HasSuffix(label, ")") {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(label[start+1:], ")"))
}

func approvalDecisionLabel(decision string) string {
	if idx := strings.Index(decision, ":"); idx >= 0 {
		return strings.TrimSpace(decision[:idx])
	}
	return firstLine(decision)
}

func readCodexSessionMeta(path string) (CodexSessionMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return CodexSessionMeta{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var line CodexJSONLine
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.Type != "session_meta" {
			continue
		}
		var meta CodexSessionMeta
		if json.Unmarshal(line.Payload, &meta) == nil {
			return meta, true
		}
		return CodexSessionMeta{}, false
	}
	return CodexSessionMeta{}, false
}

func codexMetaIsGuardian(meta CodexSessionMeta) bool {
	return meta.ThreadSource == "subagent" && CodexSourceSubagentOther(meta.Source) == "guardian"
}

func codexSessionsRoot(path string) string {
	dir := filepath.Dir(path)
	for {
		if filepath.Base(dir) == "sessions" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(path)
		}
		dir = parent
	}
}

func codexApprovalRequestSummary(text string) (string, bool) {
	if !strings.Contains(text, "APPROVAL REQUEST START") || !strings.Contains(text, "Planned action JSON:") {
		return "", false
	}
	raw := extractJSONObjectAfter(text, "Planned action JSON:")
	if raw == "" {
		return "Approval request", true
	}
	var action struct {
		Tool    string          `json:"tool"`
		Command json.RawMessage `json:"command"`
	}
	if json.Unmarshal([]byte(raw), &action) != nil {
		return "Approval request", true
	}
	if command := codexApprovalCommand(action.Command); command != "" {
		return "Approval request: " + command, true
	}
	if action.Tool != "" {
		return "Approval request: " + action.Tool, true
	}
	return "Approval request", true
}

func codexApprovalDecisionSummary(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") || !strings.Contains(text, `"outcome"`) {
		return "", false
	}
	var decision struct {
		Outcome   string `json:"outcome"`
		RiskLevel string `json:"risk_level"`
		Rationale string `json:"rationale"`
	}
	if json.Unmarshal([]byte(text), &decision) != nil || decision.Outcome == "" {
		return "", false
	}
	summary := "Approval decision: " + decision.Outcome
	if decision.RiskLevel != "" {
		summary += " (" + decision.RiskLevel + ")"
	}
	if decision.Rationale != "" {
		summary += ": " + firstLine(decision.Rationale)
	}
	return summary, true
}

func codexApprovalCommand(raw json.RawMessage) string {
	var command string
	if json.Unmarshal(raw, &command) == nil {
		return firstLine(command)
	}
	var argv []string
	if json.Unmarshal(raw, &argv) != nil || len(argv) == 0 {
		return ""
	}
	if len(argv) >= 3 {
		shell := filepath.Base(argv[0])
		if (shell == "sh" || shell == "bash" || shell == "zsh") && (argv[1] == "-c" || argv[1] == "-lc") {
			return firstLine(argv[2])
		}
	}
	return strings.Join(argv, " ")
}

func extractJSONObjectAfter(text, marker string) string {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	start := strings.IndexByte(text[idx+len(marker):], '{')
	if start < 0 {
		return ""
	}
	start += idx + len(marker)
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(text); i++ {
		switch ch := text[i]; {
		case inString:
			if escape {
				escape = false
			} else if ch == '\\' {
				escape = true
			} else if ch == '"' {
				inString = false
			}
		case ch == '"':
			inString = true
		case ch == '{':
			depth++
		case ch == '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

func codexToolInfo(name, argsJSON string) string {
	if argsJSON == "" {
		return name
	}
	var params map[string]json.RawMessage
	if json.Unmarshal([]byte(argsJSON), &params) != nil {
		return name
	}

	switch name {
	case "write_stdin":
		return codexWriteStdinInfo(name, params)
	case "press_key":
		return joinToolParts(name, appOrPID(params), rawText(params, "key"))
	case "hotkey":
		if keys := rawStringList(params, "keys"); len(keys) > 0 {
			return joinToolParts(name, strings.Join(keys, "+"))
		}
	case "click":
		if app := appOrPID(params); app != "" {
			if idx := rawScalar(params, "element_index"); idx != "" {
				return joinToolParts(name, app, "element #"+idx)
			}
			x, y := rawScalar(params, "x"), rawScalar(params, "y")
			if x != "" && y != "" {
				return joinToolParts(name, app, x+","+y)
			}
			return joinToolParts(name, app)
		}
	case "type_text":
		return joinToolParts(name, rawText(params, "app"), quoteToolText(rawText(params, "text")))
	case "type_text_chars":
		return joinToolParts(name, rawScalar(params, "pid"), quoteToolText(rawText(params, "text")))
	case "scroll":
		return joinToolParts(name, rawText(params, "app"), rawText(params, "direction"), suffixIfSet(rawScalar(params, "pages"), "p"))
	case "set_value":
		if app := rawText(params, "app"); app != "" {
			if idx := rawScalar(params, "element_index"); idx != "" {
				return joinToolParts(name, app, "element #"+idx, quoteToolText(rawText(params, "value")))
			}
			return joinToolParts(name, app, quoteToolText(rawText(params, "value")))
		}
	case "get_app_state":
		return joinToolParts(name, rawText(params, "app"))
	case "update_plan":
		return codexUpdatePlanInfo(name, params)
	case "update_goal":
		return joinToolParts(name, rawText(params, "status"))
	case "request_user_input":
		if n := rawJSONListLen(params, "questions"); n > 0 {
			label := "questions"
			if n == 1 {
				label = "question"
			}
			return joinToolParts(name, strconv.Itoa(n), label)
		}
	case "js":
		if title := rawText(params, "title"); title != "" {
			return joinToolParts(name, title)
		}
		if code := rawText(params, "code"); code != "" {
			return joinToolParts(name, firstLine(code))
		}
	case "spawn_agent":
		return joinToolParts(name, rawText(params, "agent_type"), firstLine(rawText(params, "message")))
	case "wait_agent":
		if targets := rawJSONListLen(params, "targets"); targets > 0 {
			return joinToolParts(name, strconv.Itoa(targets), "targets")
		}
	case "close_agent":
		return joinToolParts(name, rawText(params, "target"))
	}

	for _, key := range []string{"cmd", "command", "file_path", "path", "query", "url", "pattern"} {
		if v, ok := params[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				return name + " " + s
			}
		}
	}
	for _, key := range []string{"title", "section", "mode", "workingDirectory", "server", "uri", "id", "name", "destination", "prompt", "message", "target"} {
		if v := rawText(params, key); v != "" {
			return name + " " + firstLine(v)
		}
	}
	return name
}

func codexCustomToolInfo(name, input string) string {
	if name == "apply_patch" {
		return codexApplyPatchInfo(name, input)
	}
	if input == "" {
		return name
	}
	return joinToolParts(name, firstLine(input))
}

func codexApplyPatchInfo(name, input string) string {
	changes := patchFileChanges(input)
	if len(changes) == 0 {
		return name
	}
	return joinToolParts(name, strconv.Itoa(len(changes)), "files:", strings.Join(changes, ", "))
}

func patchFileChanges(input string) []string {
	var changes []string
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			changes = append(changes, "add "+strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: ")))
		case strings.HasPrefix(line, "*** Delete File: "):
			changes = append(changes, "delete "+strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: ")))
		case strings.HasPrefix(line, "*** Update File: "):
			changes = append(changes, "update "+strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: ")))
		case strings.HasPrefix(line, "*** Modify File: "):
			changes = append(changes, "update "+strings.TrimSpace(strings.TrimPrefix(line, "*** Modify File: ")))
		}
	}
	return changes
}

func codexWriteStdinInfo(name string, params map[string]json.RawMessage) string {
	session := rawScalar(params, "session_id")
	chars := rawText(params, "chars")
	detail := stdinDetail(chars)
	if session != "" {
		return joinToolParts(name, "session "+session, detail)
	}
	return joinToolParts(name, detail)
}

func stdinDetail(chars string) string {
	switch chars {
	case "\x04":
		return "Ctrl-D"
	case "\x03":
		return "Ctrl-C"
	case "\n", "\r", "\r\n":
		return "Enter"
	}
	return firstLine(chars)
}

func codexUpdatePlanInfo(name string, params map[string]json.RawMessage) string {
	var items []struct {
		Step   string `json:"step"`
		Status string `json:"status"`
	}
	if raw, ok := params["plan"]; ok && json.Unmarshal(raw, &items) == nil {
		completed := 0
		for _, item := range items {
			if item.Status == "in_progress" && strings.TrimSpace(item.Step) != "" {
				return joinToolParts(name, item.Step)
			}
			if item.Status == "completed" {
				completed++
			}
		}
		if len(items) > 0 {
			return joinToolParts(name, strconv.Itoa(completed)+"/"+strconv.Itoa(len(items))+" completed")
		}
	}
	return name
}

func rawText(params map[string]json.RawMessage, key string) string {
	if raw, ok := params[key]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func appOrPID(params map[string]json.RawMessage) string {
	if app := rawText(params, "app"); app != "" {
		return app
	}
	if pid := rawScalar(params, "pid"); pid != "" {
		return "pid " + pid
	}
	return ""
}

func rawScalar(params map[string]json.RawMessage, key string) string {
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
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return ""
}

func rawStringList(params map[string]json.RawMessage, key string) []string {
	raw, ok := params[key]
	if !ok {
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var anyList []any
	if json.Unmarshal(raw, &anyList) == nil {
		out := make([]string, 0, len(anyList))
		for _, item := range anyList {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

func rawJSONListLen(params map[string]json.RawMessage, key string) int {
	raw, ok := params[key]
	if !ok {
		return 0
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		return len(list)
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

func quoteToolText(text string) string {
	text = firstLine(text)
	if text == "" {
		return ""
	}
	return strconv.Quote(text)
}

func suffixIfSet(text, suffix string) string {
	if text == "" {
		return ""
	}
	return text + suffix
}

// stripTaskPrefix removes a leading "<task>" tag from s, returning the text after it.
// E.g. "<task> Review the spec" → "Review the spec".
func stripTaskPrefix(s string) string {
	if !strings.HasPrefix(s, "<task>") {
		return s
	}
	rest := strings.TrimPrefix(s, "<task>")
	return strings.TrimSpace(rest)
}

func codexWebSearchInfo(actionRaw json.RawMessage) string {
	var action CodexWebSearchAction
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
