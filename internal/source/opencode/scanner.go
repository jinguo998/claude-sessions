package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type Adapter struct {
	runner dbRunner
}

func NewAdapter() Adapter {
	return Adapter{runner: cliRunner{}}
}

func (a Adapter) Source() domain.Source {
	return domain.SourceOpenCode
}

func (a Adapter) Discover(ctx context.Context) ([]source.Candidate, error) {
	dbPath, err := a.dbPath(ctx)
	if err != nil {
		return nil, err
	}
	var rows []sessionRow
	if err := a.dbQuery(ctx, discoverSQL(), &rows); err != nil {
		return nil, err
	}

	candidates := make([]source.Candidate, 0, len(rows))
	for _, row := range rows {
		if row.ID == "" {
			continue
		}
		candidates = append(candidates, source.Candidate{
			Source:      domain.SourceOpenCode,
			Path:        virtualPath(dbPath, row.ID),
			ProjectDir:  projectDir(row.Directory),
			FileSize:    int64(row.MessageCount + row.PartCount),
			ModTime:     millisToTime(maxMillis(row.TimeUpdated, row.MessageUpdated, row.PartUpdated)),
			MetadataKey: metadataKey(row),
			Attributes:  rowAttributes(dbPath, row),
		})
	}
	return candidates, nil
}

func (a Adapter) ScanFile(ctx context.Context, candidate source.Candidate) (source.ScannedSession, error) {
	row := rowFromAttributes(candidate.Attributes)
	if row.ID == "" {
		row.ID = sessionIDFromVirtualPath(candidate.Path)
	}
	if row.ID == "" {
		return source.ScannedSession{}, fmt.Errorf("opencode session id is empty")
	}
	dbPath := candidate.Attributes["db_path"]
	if dbPath == "" {
		dbPath = dbPathFromVirtualPath(candidate.Path)
	}

	texts, err := a.userTexts(ctx, row.ID)
	if err != nil {
		return source.ScannedSession{}, err
	}
	allUserMsgs := make([]string, 0, len(texts))
	firstMsg := ""
	lastMsg := ""
	for _, text := range texts {
		msg := normalizeText(text.Text)
		if msg == "" {
			continue
		}
		allUserMsgs = append(allUserMsgs, msg)
		truncated := truncateRunes(msg, 100)
		if firstMsg == "" {
			firstMsg = truncated
		}
		lastMsg = truncated
	}
	msgCount := row.UserCount
	if msgCount == 0 {
		msgCount = len(allUserMsgs)
	}
	modelName, providerID := parseModel(row.Model)
	projectDir := ""
	if row.Directory != "" {
		projectDir = projectDirFromPath(row.Directory)
	}
	labels := labelValues(row.Agent)
	attrs := map[string]string{}
	if row.Agent != "" {
		attrs["agent"] = row.Agent
	}
	if providerID != "" {
		attrs["provider"] = providerID
	}
	if dbPath != "" {
		attrs["db_path"] = dbPath
	}

	searchParts := append([]string{}, allUserMsgs...)
	searchParts = append(searchParts, row.Title)

	return source.ScannedSession{
		Session: domain.Session{
			ID:          row.ID,
			Source:      domain.SourceOpenCode,
			ProjectDir:  projectDir,
			ProjectPath: row.Directory,
			Title:       row.Title,
			Client:      "opencode",
			Origin:      row.Agent,
			Labels:      labels,
			Attributes:  attrs,
			FirstMsg:    firstMsg,
			LastMsg:     lastMsg,
			StartTime:   millisToTime(row.TimeCreated),
			LastTime:    millisToTime(maxMillis(row.TimeUpdated, row.MessageUpdated, row.PartUpdated)),
			MsgCount:    msgCount,
			ToolCount:   row.ToolCount,
			FileSize:    candidate.FileSize,
			FilePath:    candidate.Path,
			Model:       modelName,
			TokenUsage:  domain.TokenUsage{Input: row.TokensInput, Output: row.TokensOutput},
		},
		SearchParts: searchParts,
	}, nil
}

func (a Adapter) userTexts(ctx context.Context, sessionID string) ([]userTextRow, error) {
	var rows []userTextRow
	if err := a.dbQuery(ctx, userTextsSQL(sessionID), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (a Adapter) dbPath(ctx context.Context) (string, error) {
	if a.runner == nil {
		a.runner = cliRunner{}
	}
	return a.runner.DBPath(ctx)
}

func (a Adapter) dbQuery(ctx context.Context, sql string, dest any) error {
	if a.runner == nil {
		a.runner = cliRunner{}
	}
	return a.runner.Query(ctx, sql, dest)
}

func discoverSQL() string {
	return `
select
  s.id as id,
  s.directory as directory,
  s.title as title,
  s.agent as agent,
  s.model as model,
  s.tokens_input as tokens_input,
  s.tokens_output as tokens_output,
  s.time_created as time_created,
  s.time_updated as time_updated,
  coalesce((select max(time_updated) from message where session_id = s.id), 0) as message_updated,
  coalesce((select max(time_updated) from part where session_id = s.id), 0) as part_updated,
  (select count(*) from message where session_id = s.id) as message_count,
  (select count(*) from part where session_id = s.id) as part_count,
  (select count(*) from message where session_id = s.id and json_extract(data, '$.role') = 'user') as user_count,
  (select count(*) from part where session_id = s.id and json_extract(data, '$.type') = 'tool') as tool_count
from session s
where s.time_archived is null
  and s.parent_id is null
order by s.time_updated desc, s.id desc`
}

func userTextsSQL(sessionID string) string {
	return `
select
  p.time_created as time_created,
  json_extract(p.data, '$.text') as text
from message m
join part p on p.message_id = m.id
where m.session_id = ` + sqlQuote(sessionID) + `
  and json_extract(m.data, '$.role') = 'user'
  and json_extract(p.data, '$.type') = 'text'
order by m.time_created, m.id, p.time_created, p.id`
}

func rowAttributes(dbPath string, row sessionRow) map[string]string {
	return map[string]string{
		"db_path":         dbPath,
		"id":              row.ID,
		"directory":       row.Directory,
		"title":           row.Title,
		"agent":           row.Agent,
		"model":           row.Model,
		"tokens_input":    strconv.Itoa(row.TokensInput),
		"tokens_output":   strconv.Itoa(row.TokensOutput),
		"time_created":    strconv.FormatInt(row.TimeCreated, 10),
		"time_updated":    strconv.FormatInt(row.TimeUpdated, 10),
		"message_updated": strconv.FormatInt(row.MessageUpdated, 10),
		"part_updated":    strconv.FormatInt(row.PartUpdated, 10),
		"message_count":   strconv.Itoa(row.MessageCount),
		"part_count":      strconv.Itoa(row.PartCount),
		"user_count":      strconv.Itoa(row.UserCount),
		"tool_count":      strconv.Itoa(row.ToolCount),
	}
}

func rowFromAttributes(attrs map[string]string) sessionRow {
	if attrs == nil {
		return sessionRow{}
	}
	return sessionRow{
		ID:             attrs["id"],
		Directory:      attrs["directory"],
		Title:          attrs["title"],
		Agent:          attrs["agent"],
		Model:          attrs["model"],
		TokensInput:    attrInt(attrs, "tokens_input"),
		TokensOutput:   attrInt(attrs, "tokens_output"),
		TimeCreated:    attrInt64(attrs, "time_created"),
		TimeUpdated:    attrInt64(attrs, "time_updated"),
		MessageUpdated: attrInt64(attrs, "message_updated"),
		PartUpdated:    attrInt64(attrs, "part_updated"),
		MessageCount:   attrInt(attrs, "message_count"),
		PartCount:      attrInt(attrs, "part_count"),
		UserCount:      attrInt(attrs, "user_count"),
		ToolCount:      attrInt(attrs, "tool_count"),
	}
}

func attrInt(attrs map[string]string, key string) int {
	v, _ := strconv.Atoi(attrs[key])
	return v
}

func attrInt64(attrs map[string]string, key string) int64 {
	v, _ := strconv.ParseInt(attrs[key], 10, 64)
	return v
}

func metadataKey(row sessionRow) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d:%d:%d:%d:%s:%s:%d:%d",
		metadataVersion,
		row.ID,
		row.TimeUpdated,
		row.MessageUpdated,
		row.PartUpdated,
		row.MessageCount,
		row.PartCount,
		row.UserCount,
		row.Title,
		row.Model,
		row.TokensInput,
		row.TokensOutput,
	)
}

func virtualPath(dbPath, sessionID string) string {
	return dbPath + "#" + sessionID
}

func dbPathFromVirtualPath(path string) string {
	if idx := strings.LastIndex(path, "#"); idx >= 0 {
		return path[:idx]
	}
	return ""
}

func sessionIDFromVirtualPath(path string) string {
	if idx := strings.LastIndex(path, "#"); idx >= 0 {
		return path[idx+1:]
	}
	return ""
}

func millisToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

func maxMillis(values ...int64) int64 {
	var max int64
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func normalizeText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func truncateRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}

func labelValues(values ...string) []string {
	var labels []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			labels = append(labels, value)
		}
	}
	return labels
}

func projectDir(path string) string {
	return projectDirFromPath(path)
}

func projectDirFromPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	trimmed := strings.TrimRight(path, string(filepath.Separator))
	if trimmed == "" {
		return path
	}
	return filepath.Base(trimmed)
}

func parseModel(raw string) (display, providerID string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	var model modelData
	if json.Unmarshal([]byte(raw), &model) == nil {
		id := model.ID
		if id == "" {
			id = model.ModelID
		}
		if model.ProviderID != "" && id != "" {
			return model.ProviderID + "/" + id, model.ProviderID
		}
		if id != "" {
			return id, model.ProviderID
		}
	}
	var s string
	if json.Unmarshal([]byte(raw), &s) == nil {
		return s, ""
	}
	return raw, ""
}
