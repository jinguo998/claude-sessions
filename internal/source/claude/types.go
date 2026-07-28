package claude

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// jsonLine represents the minimal fields needed from each Claude JSONL line.
type jsonLine struct {
	Type      string `json:"type"`
	IsMeta    bool   `json:"isMeta"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Model   string          `json:"model"`
		Usage   *claudeUsage    `json:"usage"`
	} `json:"message"`
	Snapshot *struct {
		Timestamp string `json:"timestamp"`
	} `json:"snapshot"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

type contentItem struct {
	Type string `json:"type"`
}

func isSystemContent(content string) bool {
	if strings.HasPrefix(content, "<command-") {
		return !strings.Contains(content, "<command-args>")
	}
	return strings.HasPrefix(content, "<local-command") ||
		strings.HasPrefix(content, "{") ||
		strings.HasPrefix(content, "<task-") ||
		strings.HasPrefix(content, "<system-reminder")
}

func extractCommandArgs(content string) string {
	const open = "<command-args>"
	const close = "</command-args>"
	start := strings.Index(content, open)
	if start < 0 {
		return content
	}
	start += len(open)
	end := strings.Index(content[start:], close)
	if end < 0 {
		return content[start:]
	}
	return content[start : start+end]
}

func decodeProjectDir(encoded string) string {
	if encoded == "" {
		return ""
	}
	if encoded[0] != '-' {
		return encoded
	}

	naive := strings.ReplaceAll(encoded, "-", "/")
	if _, err := os.Stat(naive); err == nil {
		return naive
	}

	parts := strings.Split(encoded[1:], "-")
	if len(parts) == 0 {
		return naive
	}
	if resolved := resolveSegments("/"+parts[0], parts[1:]); resolved != "" {
		return resolved
	}
	return naive
}

func resolveSegments(prefix string, remaining []string) string {
	if len(remaining) == 0 {
		return prefix
	}

	part := remaining[0]
	rest := remaining[1:]

	for _, candidate := range []string{
		prefix + "." + part,
		prefix + "_" + part,
		prefix + "-" + part,
		prefix + "/" + part,
	} {
		if dirExists(candidate) || (len(rest) > 0 && hasPrefixDir(candidate)) {
			if result := resolveSegments(candidate, rest); result != "" {
				return result
			}
		}
	}

	if len(rest) == 0 {
		return prefix + "/" + part
	}
	return resolveSegments(prefix+"/"+part, rest)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func hasPrefixDir(path string) bool {
	if dirExists(path) {
		return true
	}
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return false
	}
	parent := path[:idx]
	base := path[idx+1:]
	entries, err := os.ReadDir(parent)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), base) && e.Name() != base {
			return true
		}
	}
	return false
}

var decodedPathCache sync.Map

func decodeProjectDirCached(encoded string) string {
	if v, ok := decodedPathCache.Load(encoded); ok {
		return v.(string)
	}
	v := decodeProjectDir(encoded)
	decodedPathCache.Store(encoded, v)
	return v
}
