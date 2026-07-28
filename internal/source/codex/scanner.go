package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

// codexUUIDRe matches the UUID portion at the end of a Codex session filename.
var codexUUIDRe = regexp.MustCompile(`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$`)

const codexMetadataKeyVersion = "codex-meta-v2"

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
		var entry CodexSessionIndexEntry
		if json.Unmarshal(sc.Bytes(), &entry) == nil && entry.ID != "" {
			idx[entry.ID] = entry.ThreadName
		}
	}
	return idx, sc.Err()
}

// ScanCodexSessionFile reads a single Codex JSONL file and extracts session metadata.
func ScanCodexSessionFile(path string, fileSize int64, threadIndex map[string]string) (source.ScannedSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return source.ScannedSession{}, err
	}
	defer f.Close()

	filenameID := ExtractCodexSessionID(filepath.Base(path))

	var (
		id           string
		isGuardian   bool
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
		var line CodexJSONLine
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
			var meta CodexSessionMeta
			if json.Unmarshal(line.Payload, &meta) == nil {
				id = meta.ID
				projectPath = meta.CWD
				originator = meta.Originator
				editorSource = codexClientLabel(meta)
				isGuardian = meta.ThreadSource == "subagent" && CodexSourceSubagentOther(meta.Source) == "guardian"
			}

		case "turn_context":
			if modelName == "" {
				var tc CodexTurnContext
				if json.Unmarshal(line.Payload, &tc) == nil && tc.Model != "" {
					modelName = tc.Model
				}
			}

		case "event_msg":
			var evt CodexEventMsg
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
				msg = stripTaskPrefix(msg)
				if summary, ok := codexApprovalRequestSummary(msg); ok {
					msg = summary
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
				if evt.Info != nil && string(evt.Info) != "null" {
					var ti CodexTokenInfo
					if json.Unmarshal(evt.Info, &ti) == nil && ti.TotalTokenUsage != nil {
						tokensIn = ti.TotalTokenUsage.InputTokens
						tokensOut = ti.TotalTokenUsage.OutputTokens
					}
				}
			}

		case "response_item":
			var item CodexResponseItem
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

	if isGuardian {
		msgCount = 0
		allUserMsgs = nil
	}
	searchParts := append([]string{}, allUserMsgs...)
	if threadName != "" {
		searchParts = append(searchParts, threadName)
	}

	return source.ScannedSession{
		Session: domain.Session{
			ID:          id,
			ProjectDir:  projectDir,
			ProjectPath: projectPath,
			Title:       threadName,
			Origin:      originator,
			Client:      editorSource,
			FirstMsg:    firstMsg,
			LastMsg:     lastMsg,
			StartTime:   startTime,
			LastTime:    lastTime,
			MsgCount:    msgCount,
			ToolCount:   toolCount,
			FileSize:    fileSize,
			FilePath:    path,
			Model:       modelName,
			Source:      domain.SourceCodex,
			TokenUsage:  domain.TokenUsage{Input: tokensIn, Output: tokensOut},
		},
		SearchParts: searchParts,
	}, nil
}

type Adapter struct{}

func NewAdapter() Adapter {
	return Adapter{}
}

func (Adapter) Source() domain.Source {
	return domain.SourceCodex
}

func (a Adapter) Discover(ctx context.Context) ([]source.Candidate, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	sessionsDir := filepath.Join(home, ".codex", "sessions")
	if _, err := os.Stat(sessionsDir); err != nil {
		return nil, err
	}

	indexPath := filepath.Join(home, ".codex", "session_index.jsonl")
	threadIndex, indexErr := LoadCodexSessionIndex(indexPath)
	if os.IsNotExist(indexErr) {
		indexErr = nil
	}
	if threadIndex == nil {
		threadIndex = make(map[string]string)
	}

	var candidates []source.Candidate
	var firstErr error
	if indexErr != nil {
		firstErr = indexErr
	}
	walkErr := filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return nil
		}
		if info == nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		if meta, ok := readCodexSessionMeta(path); ok && codexMetaIsGuardian(meta) {
			return nil
		}
		id := ExtractCodexSessionID(info.Name())
		threadName := threadIndex[id]
		candidates = append(candidates, source.Candidate{
			Source:      domain.SourceCodex,
			Path:        path,
			FileSize:    info.Size(),
			ModTime:     info.ModTime(),
			MetadataKey: codexMetadataKey(threadName),
			Attributes: map[string]string{
				"thread_name": threadName,
			},
		})
		return nil
	})
	if walkErr != nil {
		return candidates, walkErr
	}
	if err := ctx.Err(); err != nil {
		return candidates, err
	}
	return candidates, firstErr
}

func codexClientLabel(meta CodexSessionMeta) string {
	if originator := strings.TrimSpace(meta.Originator); originator != "" {
		return originator
	}
	return CodexSourceString(meta.Source)
}

func codexMetadataKey(threadName string) string {
	return codexMetadataKeyVersion + "\x00" + threadName
}

func (a Adapter) ScanFile(ctx context.Context, candidate source.Candidate) (source.ScannedSession, error) {
	_ = ctx
	threadName := ""
	if candidate.Attributes != nil {
		threadName = candidate.Attributes["thread_name"]
	}
	threadIndex := map[string]string{}
	if id := ExtractCodexSessionID(filepath.Base(candidate.Path)); id != "" {
		threadIndex[id] = threadName
	}
	return ScanCodexSessionFile(candidate.Path, candidate.FileSize, threadIndex)
}

// ScanCodexSessions scans Codex sessions without cache. App scan.Repository owns cache.
func ScanCodexSessions(ctx context.Context) ([]source.ScannedSession, error) {
	adapter := NewAdapter()
	candidates, err := adapter.Discover(ctx)
	if err != nil {
		return nil, err
	}
	type result struct {
		session source.ScannedSession
		err     error
	}
	const maxWorkers = 16
	var wg sync.WaitGroup
	ch := make(chan result, 100)
	sem := make(chan struct{}, maxWorkers)
	for _, candidate := range candidates {
		wg.Add(1)
		go func(candidate source.Candidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sess, err := adapter.ScanFile(ctx, candidate)
			ch <- result{sess, err}
		}(candidate)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()
	var sessions []source.ScannedSession
	for r := range ch {
		if r.err == nil && r.session.Session.MsgCount > 0 {
			sessions = append(sessions, r.session)
		}
	}
	return sessions, nil
}
