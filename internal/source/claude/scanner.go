package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
)

type Adapter struct{}

func NewAdapter() Adapter {
	return Adapter{}
}

func (Adapter) Source() domain.Source {
	return domain.SourceClaude
}

func (a Adapter) Discover(ctx context.Context) ([]source.Candidate, error) {
	return discoverSessions(ctx)
}

func (a Adapter) ScanFile(ctx context.Context, candidate source.Candidate) (source.ScannedSession, error) {
	_ = ctx
	return ScanSessionFile(candidate.Path, candidate.ProjectDir, candidate.FileSize)
}

// ScanSessionFile reads a single JSONL file and extracts session metadata.
func ScanSessionFile(path, projectDir string, fileSize int64) (source.ScannedSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return source.ScannedSession{}, err
	}
	defer f.Close()

	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	var (
		firstMsg    string
		lastMsg     string
		modelName   string
		startTime   time.Time
		lastTime    time.Time
		msgCount    int
		toolCount   int
		tokensIn    int
		tokensOut   int
		allUserMsgs []string
		lastRole    string // track last role to count conversation turns, not events
	)
	seenUsageIDs := make(map[string]struct{})

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // 10MB max line, matching parser

	for sc.Scan() {
		var line jsonLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}

		// Collect timestamps from top-level and snapshot fields.
		timestamps := []string{}
		if line.Timestamp != "" {
			timestamps = append(timestamps, line.Timestamp)
		}
		if line.Snapshot != nil && line.Snapshot.Timestamp != "" {
			timestamps = append(timestamps, line.Snapshot.Timestamp)
		}
		for _, raw := range timestamps {
			if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				if startTime.IsZero() || ts.Before(startTime) {
					startTime = ts
				}
				if ts.After(lastTime) {
					lastTime = ts
				}
			}
		}

		// Count conversation turns, not raw events.
		// Consecutive lines with the same role count as one turn.
		if line.Type == "user" || line.Type == "assistant" {
			role := line.Type
			if role == "user" && line.IsMeta {
				role = "" // skip meta
			}
			if role != "" && role != lastRole {
				msgCount++
				lastRole = role
			}
		}

		// Extract model name from assistant messages
		if line.Type == "assistant" && line.Message != nil && line.Message.Model != "" && modelName == "" {
			m := line.Message.Model
			switch {
			case strings.Contains(m, "opus"):
				modelName = "opus"
			case strings.Contains(m, "sonnet"):
				modelName = "sonnet"
			case strings.Contains(m, "haiku"):
				modelName = "haiku"
			default:
				modelName = m
			}
		}

		// Claude Code can write multiple JSONL rows for the same assistant
		// message id (for example thinking then tool_use). Their usage block is
		// duplicated, so count each message id once.
		if line.Type == "assistant" && line.Message != nil && line.Message.Usage != nil {
			usageID := line.Message.ID
			shouldCount := true
			if usageID != "" {
				if _, ok := seenUsageIDs[usageID]; ok {
					shouldCount = false
				} else {
					seenUsageIDs[usageID] = struct{}{}
				}
			}
			if shouldCount {
				usage := line.Message.Usage
				tokensIn += usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
				tokensOut += usage.OutputTokens
			}
		}

		// Count tool_use blocks in assistant content
		if line.Type == "assistant" && line.Message != nil {
			var items []contentItem
			if err := json.Unmarshal(line.Message.Content, &items); err == nil {
				for _, item := range items {
					if item.Type == "tool_use" {
						toolCount++
					}
				}
			}
		}

		// Extract user messages for search, first/last tracking
		if line.Type == "user" && !line.IsMeta && line.Message != nil {
			var content string
			if err := json.Unmarshal(line.Message.Content, &content); err == nil && content != "" {
				if isSystemContent(content) {
					continue
				}
				if strings.HasPrefix(content, "<command-") {
					content = extractCommandArgs(content)
				}
				content = strings.TrimSpace(content)
				if content == "" {
					continue
				}
				content = strings.Join(strings.Fields(content), " ")
				allUserMsgs = append(allUserMsgs, content)
				truncated := content
				if len([]rune(truncated)) > 100 {
					truncated = string([]rune(truncated)[:100])
				}
				if firstMsg == "" {
					firstMsg = truncated
				}
				lastMsg = truncated
			}
		}
	}
	// Log scan errors (e.g. line too long) — don't fail, just use what we got
	if err := sc.Err(); err != nil {
		// silent: partial data is better than no data for metadata scanning
		_ = err
	}

	projectPath := decodeProjectDirCached(projectDir)

	return source.ScannedSession{
		Session: domain.Session{
			ID:          id,
			ProjectDir:  projectDir,
			ProjectPath: projectPath,
			FirstMsg:    firstMsg,
			LastMsg:     lastMsg,
			StartTime:   startTime,
			LastTime:    lastTime,
			MsgCount:    msgCount,
			ToolCount:   toolCount,
			FileSize:    fileSize,
			FilePath:    path,
			Model:       modelName,
			TokenUsage:  domain.TokenUsage{Input: tokensIn, Output: tokensOut},
			Source:      domain.SourceClaude,
		},
		SearchParts: allUserMsgs,
	}, nil
}

func discoverSessions(ctx context.Context) ([]source.Candidate, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Join(home, ".claude", "projects")

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	var candidates []source.Candidate
	var firstErr error
	for _, projEntry := range entries {
		if err := ctx.Err(); err != nil {
			return candidates, err
		}
		if !projEntry.IsDir() {
			continue
		}
		projDir := projEntry.Name()
		projPath := filepath.Join(baseDir, projDir)

		files, err := os.ReadDir(projPath)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			filePath := filepath.Join(projPath, f.Name())
			info, err := f.Info()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			candidates = append(candidates, source.Candidate{
				Source:     domain.SourceClaude,
				Path:       filePath,
				ProjectDir: projDir,
				FileSize:   info.Size(),
				ModTime:    info.ModTime(),
			})
		}
	}
	return candidates, firstErr
}

const maxWorkers = 16

func ScanAllSessions(ctx context.Context) ([]source.ScannedSession, error) {
	candidates, err := discoverSessions(ctx)
	if err != nil {
		return nil, err
	}
	type result struct {
		session source.ScannedSession
		err     error
	}
	var wg sync.WaitGroup
	ch := make(chan result, 100)
	sem := make(chan struct{}, maxWorkers)
	for _, candidate := range candidates {
		wg.Add(1)
		go func(candidate source.Candidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sess, err := ScanSessionFile(candidate.Path, candidate.ProjectDir, candidate.FileSize)
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
