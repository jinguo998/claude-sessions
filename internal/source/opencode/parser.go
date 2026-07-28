package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

func (a Adapter) ParsePreview(ctx context.Context, sess domain.Session, opts source.PreviewOptions) ([]domain.ConversationTurn, error) {
	return a.preview(ctx, sess, opts.Verbose, 0)
}

func (a Adapter) ParsePreviewTail(ctx context.Context, sess domain.Session, opts source.TailOptions) ([]domain.ConversationTurn, error) {
	maxMessages := opts.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 20
	}
	turns, err := a.preview(ctx, sess, opts.Verbose, maxMessages)
	if err != nil {
		return nil, err
	}
	return trimTurnsByBudget(turns, opts.MaxMessages, opts.MaxBytes), nil
}

func (a Adapter) preview(ctx context.Context, sess domain.Session, verbose bool, messageLimit int) ([]domain.ConversationTurn, error) {
	sessionID := sess.ID
	if sessionID == "" {
		sessionID = sessionIDFromVirtualPath(sess.FilePath)
	}
	if sessionID == "" {
		return nil, fmt.Errorf("opencode session id is empty")
	}
	var rows []previewRow
	query := previewSQL(sessionID)
	if messageLimit > 0 {
		query = previewTailSQL(sessionID, messageLimit)
	}
	if err := a.dbQuery(ctx, query, &rows); err != nil {
		return nil, err
	}
	return rowsToTurns(rows, verbose), nil
}

func previewSQL(sessionID string) string {
	return `
select
  m.id as message_id,
  json_extract(m.data, '$.role') as role,
  m.time_created as message_time,
  coalesce(p.time_created, m.time_created) as part_time,
  p.data as part_data
from message m
left join part p on p.message_id = m.id
where m.session_id = ` + sqlQuote(sessionID) + `
order by m.time_created, m.id, p.time_created, p.id`
}

func previewTailSQL(sessionID string, limit int) string {
	return `
with recent as (
  select id
  from message
  where session_id = ` + sqlQuote(sessionID) + `
  order by time_created desc, id desc
  limit ` + fmt.Sprintf("%d", limit) + `
)
select
  m.id as message_id,
  json_extract(m.data, '$.role') as role,
  m.time_created as message_time,
  coalesce(p.time_created, m.time_created) as part_time,
  p.data as part_data
from message m
join recent r on r.id = m.id
left join part p on p.message_id = m.id
order by m.time_created, m.id, p.time_created, p.id`
}

func rowsToTurns(rows []previewRow, verbose bool) []domain.ConversationTurn {
	var turns []domain.ConversationTurn
	for _, row := range rows {
		if len(row.PartData) == 0 || string(row.PartData) == "null" {
			continue
		}
		part, ok := decodePartData(row.PartData)
		if !ok {
			continue
		}
		ts := millisToTime(row.PartTime)
		if ts.IsZero() {
			ts = millisToTime(row.MessageTime)
		}
		switch part.Type {
		case "text":
			text := strings.TrimSpace(part.Text)
			if text != "" && (row.Role == "user" || row.Role == "assistant") {
				turns = append(turns, domain.ConversationTurn{Role: row.Role, Text: text, Timestamp: ts})
			}
		case "tool":
			turns = append(turns, domain.ConversationTurn{Role: "tool", Text: toolSummary(part), Timestamp: ts})
			if verbose {
				if output := toolOutput(part); output != "" {
					turns = append(turns, domain.ConversationTurn{Role: "tool_result", Text: output, Timestamp: ts})
				}
			}
		case "patch":
			turns = append(turns, domain.ConversationTurn{Role: "tool", Text: patchSummary(part), Timestamp: ts})
		case "file":
			if text := fileSummary(part); text != "" {
				turns = append(turns, domain.ConversationTurn{Role: "tool", Text: text, Timestamp: ts})
			}
		}
	}
	return turns
}

func decodePartData(raw json.RawMessage) (partData, bool) {
	var part partData
	if json.Unmarshal(raw, &part) == nil && part.Type != "" {
		return part, true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		if json.Unmarshal([]byte(text), &part) == nil && part.Type != "" {
			return part, true
		}
	}
	return partData{}, false
}

func toolSummary(part partData) string {
	name := strings.TrimSpace(part.Tool)
	if name == "" {
		name = "tool"
	}
	var state toolState
	if len(part.State) == 0 || json.Unmarshal(part.State, &state) != nil || len(state.Input) == 0 {
		return name
	}
	if detail := toolInputDetail(name, state.Input); detail != "" {
		return name + " " + detail
	}
	if detail := inputDetail(state.Input); detail != "" {
		return name + " " + detail
	}
	return name
}

func toolInputDetail(name string, raw json.RawMessage) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return ""
	}
	switch strings.ToLower(name) {
	case "task":
		for _, key := range []string{"description", "prompt"} {
			if text := stringValue(obj[key]); text != "" {
				return firstLine(text)
			}
		}
	case "todowrite":
		if n := arrayLen(obj["todos"]); n > 0 {
			return fmt.Sprintf("%d todos", n)
		}
	case "skill":
		if text := stringValue(obj["name"]); text != "" {
			return firstLine(text)
		}
	case "question":
		if n := arrayLen(obj["questions"]); n > 0 {
			if n == 1 {
				return "1 question"
			}
			return fmt.Sprintf("%d questions", n)
		}
	}
	return ""
}

func toolOutput(part partData) string {
	var state toolState
	if len(part.State) == 0 || json.Unmarshal(part.State, &state) != nil || len(state.Output) == 0 {
		return ""
	}
	return strings.TrimSpace(jsonTextValue(state.Output))
}

func inputDetail(raw json.RawMessage) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return strings.TrimSpace(jsonTextValue(raw))
	}
	for _, key := range []string{"command", "cmd", "filePath", "file_path", "path", "query", "url", "pattern"} {
		if value, ok := obj[key]; ok {
			if text := stringValue(value); text != "" {
				return firstLine(text)
			}
		}
	}
	return ""
}

func patchSummary(part partData) string {
	files := patchFiles(part.Files)
	if len(files) == 0 {
		return "patch"
	}
	shown := files
	if len(shown) > 3 {
		shown = shown[:3]
	}
	summary := fmt.Sprintf("patch %d files: %s", len(files), strings.Join(shown, ", "))
	if len(files) > len(shown) {
		summary += ", ..."
	}
	return summary
}

func patchFiles(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var stringsList []string
	if json.Unmarshal(raw, &stringsList) == nil {
		return nonEmptyStrings(stringsList)
	}
	var objects []map[string]any
	if json.Unmarshal(raw, &objects) == nil {
		var files []string
		for _, obj := range objects {
			for _, key := range []string{"path", "filename", "file", "name"} {
				if text := stringValue(obj[key]); text != "" {
					files = append(files, firstLine(text))
					break
				}
			}
		}
		return files
	}
	return nil
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func jsonTextValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for _, key := range []string{"value", "text", "content", "output"} {
			if value, ok := obj[key]; ok {
				if text := stringValue(value); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, item := range v {
			if text := stringValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"value", "text", "content", "output", "command", "path"} {
			if item, ok := v[key]; ok {
				if text := stringValue(item); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func arrayLen(value any) int {
	if list, ok := value.([]any); ok {
		return len(list)
	}
	return 0
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return text
}

func fileSummary(part partData) string {
	if part.Filename != "" {
		return "file " + part.Filename
	}
	if part.URL != "" {
		return "file " + part.URL
	}
	return "file"
}

func trimTurnsByBudget(turns []domain.ConversationTurn, maxMessages int, maxBytes int64) []domain.ConversationTurn {
	if maxMessages > 0 && len(turns) > maxMessages {
		turns = turns[len(turns)-maxMessages:]
	}
	if maxBytes <= 0 {
		return turns
	}
	var total int64
	start := len(turns)
	for start > 0 {
		next := int64(len(turns[start-1].Text))
		if total > 0 && total+next > maxBytes {
			break
		}
		total += next
		start--
	}
	return turns[start:]
}
