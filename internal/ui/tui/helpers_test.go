package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
	apppreview "github.com/jinguo998/claude-sessions/internal/app/preview"
	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/source"
	"github.com/jinguo998/claude-sessions/internal/source/claude"
	"github.com/jinguo998/claude-sessions/internal/source/codex"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func testPreviewService() *apppreview.Service {
	claudeAdapter := claude.NewAdapter()
	codexAdapter := codex.NewAdapter()
	return apppreview.NewService([]source.PreviewParser{claudeAdapter, codexAdapter})
}

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		name string
		when time.Time
		want string
	}{
		{name: "just now", when: time.Now().Add(-30 * time.Second), want: "just now"},
		{name: "minutes", when: time.Now().Add(-5 * time.Minute), want: "5m ago"},
		{name: "hours", when: time.Now().Add(-3 * time.Hour), want: "3h ago"},
		{name: "days", when: time.Now().Add(-49 * time.Hour), want: "2d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relativeTime(tt.when); got != tt.want {
				t.Fatalf("relativeTime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{in: "abc", want: 3},
		{in: "你好", want: 4},
		{in: "a你b", want: 4},
	}

	for _, tt := range tests {
		if got := displayWidth(tt.in); got != tt.want {
			t.Fatalf("displayWidth(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		maxWidth int
		want     string
	}{
		{name: "fits", in: "hello", maxWidth: 10, want: "hello"},
		{name: "ascii", in: "hello world", maxWidth: 8, want: "hello..."},
		{name: "cjk", in: "你好世界", maxWidth: 5, want: "你..."},
		{name: "zero width", in: "hello", maxWidth: 0, want: ""},
		{name: "tiny width", in: "hello", maxWidth: 2, want: ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateToWidth(tt.in, tt.maxWidth); got != tt.want {
				t.Fatalf("truncateToWidth(%q, %d) = %q, want %q", tt.in, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestPadToWidth(t *testing.T) {
	got := padToWidth("你a", 5)
	if got != "你a  " {
		t.Fatalf("padToWidth() = %q, want %q", got, "你a  ")
	}
	if displayWidth(got) != 5 {
		t.Fatalf("displayWidth(padToWidth()) = %d, want 5", displayWidth(got))
	}

	if got := padToWidth("already wide", 5); got != "already wide" {
		t.Fatalf("padToWidth() should not trim, got %q", got)
	}
}

func TestWrapText(t *testing.T) {
	if got := wrapText("hello world", 6); got != "hello \nworld" {
		t.Fatalf("wrapText() = %q, want %q", got, "hello \nworld")
	}

	if got := wrapText("unchanged", 0); got != "unchanged" {
		t.Fatalf("wrapText(width<=0) = %q, want unchanged", got)
	}
}

func TestHighlightQuery(t *testing.T) {
	base := lipgloss.NewStyle()
	match := lipgloss.NewStyle().Reverse(true)

	got := highlightQuery("Hello hello", "heLLo", base, match)
	if stripped := stripANSI(got); stripped != "Hello hello" {
		t.Fatalf("highlightQuery stripped text = %q, want original", stripped)
	}
	if got == "Hello hello" {
		t.Fatalf("highlightQuery() did not style matches: %q", got)
	}
}

func TestFormatPreviewRendersMarkdown(t *testing.T) {
	result := formatPreviewWithColors([]domain.ConversationTurn{
		{
			Role: "assistant",
			Text: "# Plan\n\n- inspect\n- patch\n\n```go\nfmt.Println(\"ok\")\n```",
		},
	}, 80, false)

	got := stripANSI(result.content)
	for _, needle := range []string{"Plan", "inspect", "patch", "fmt.Println"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("rendered markdown missing %q in %q", needle, got)
		}
	}
	if strings.Contains(got, "```") {
		t.Fatalf("rendered markdown should not expose raw code fences: %q", got)
	}
}

func TestFormatPreviewCanRenderRawMarkdownText(t *testing.T) {
	result := formatPreviewWithColors([]domain.ConversationTurn{
		{
			Role: "assistant",
			Text: "# Plan\n\n- inspect\n\n```go\nfmt.Println(\"ok\")\n```",
		},
	}, 80, false, false)

	got := stripANSI(result.content)
	for _, needle := range []string{"# Plan", "- inspect", "```go"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("raw preview missing %q in %q", needle, got)
		}
	}
}

func TestFormatPreviewRendersApprovalDistinctFromTool(t *testing.T) {
	result := formatPreviewWithColors([]domain.ConversationTurn{
		{Role: "tool", Text: "exec_command make install"},
		{Role: "approval", Text: "approved make install (medium risk)"},
	}, 80, false, false)

	got := stripANSI(result.content)
	if !strings.Contains(got, "│ exec_command make install") {
		t.Fatalf("tool line missing in %q", got)
	}
	if !strings.Contains(got, "  approved make install (medium risk)") {
		t.Fatalf("approval line missing in %q", got)
	}
	if strings.Contains(got, "│ approved") {
		t.Fatalf("approval should not render as a tool line: %q", got)
	}
}

func TestCompactSidePreviewMessagesTruncatesLongText(t *testing.T) {
	longText := strings.Repeat("x", sidePreviewMaxTextRunes+100)
	got := compactSidePreviewMessages([]domain.ConversationTurn{
		{Role: "assistant", Text: longText},
	})

	if len(got) != 1 {
		t.Fatalf("compact len = %d, want 1", len(got))
	}
	if len([]rune(got[0].Text)) >= len([]rune(longText)) {
		t.Fatal("long side preview text was not truncated")
	}
	if !strings.Contains(got[0].Text, "truncated for side preview") {
		t.Fatalf("truncated text missing suffix: %q", got[0].Text)
	}
}

func TestLoadSidePreviewContentPerformanceFromEnv(t *testing.T) {
	path := os.Getenv("CLAUDE_SESSIONS_PERF_FILE")
	if path == "" {
		t.Skip("set CLAUDE_SESSIONS_PERF_FILE to benchmark a real session JSONL")
	}

	source := session.SourceCodex
	if os.Getenv("CLAUDE_SESSIONS_PERF_SOURCE") == "claude" {
		source = session.SourceClaude
	}

	start := time.Now()
	result, ok := LoadSidePreviewContent(testPreviewService(), session.Session{FilePath: path, Source: source}, 73)
	elapsed := time.Since(start)
	if !ok {
		t.Fatalf("LoadSidePreviewContent(%q) returned no content", path)
	}
	t.Logf("LoadSidePreviewContent(%s) took %s, content=%d bytes, lines=%d",
		filepath.Base(path), elapsed, len(result.content), strings.Count(result.content, "\n")+1)

	if maxMS := os.Getenv("CLAUDE_SESSIONS_PERF_MAX_MS"); maxMS != "" {
		limitMS, err := strconv.Atoi(maxMS)
		if err != nil {
			t.Fatalf("invalid CLAUDE_SESSIONS_PERF_MAX_MS=%q: %v", maxMS, err)
		}
		if elapsed > time.Duration(limitMS)*time.Millisecond {
			t.Fatalf("side preview render took %s, want <= %dms", elapsed, limitMS)
		}
	}
}

func BenchmarkLoadSidePreviewContentSyntheticMarkdown(b *testing.B) {
	path := writeCodexMarkdownFixture(b, 32, sidePreviewMaxTextRunes+2000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, ok := LoadSidePreviewContent(testPreviewService(), session.Session{FilePath: path, Source: session.SourceCodex}, 73)
		if !ok {
			b.Fatal("LoadSidePreviewContent returned no content")
		}
		if result.content == "" {
			b.Fatal("empty side preview content")
		}
	}
}

func writeCodexMarkdownFixture(tb testing.TB, messages int, textRunes int) string {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "codex-markdown-session.jsonl")
	var b strings.Builder
	for i := 0; i < messages; i++ {
		line := map[string]any{
			"timestamp": fmt.Sprintf("2026-04-01T10:00:%02d.000Z", i%60),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]string{
					{
						"type": "output_text",
						"text": syntheticMarkdownText(textRunes),
					},
				},
			},
		}
		encoded, err := json.Marshal(line)
		if err != nil {
			tb.Fatal(err)
		}
		b.Write(encoded)
		b.WriteByte('\n')
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		tb.Fatal(err)
	}
	return path
}

func syntheticMarkdownText(textRunes int) string {
	chunk := "# Heading\n\n- item with **bold** text and `inline code`\n- another item with [link](https://example.com)\n\n```go\nfmt.Println(\"hello\")\n```\n\n"
	var b strings.Builder
	for len([]rune(b.String())) < textRunes {
		b.WriteString(chunk)
	}
	return truncateRunes(b.String(), textRunes)
}

func TestFindMatchSnippet(t *testing.T) {
	text := "alpha beta gamma delta epsilon zeta"
	got := findMatchSnippet(text, "gamma", 12)
	if !strings.Contains(got, "gamma") {
		t.Fatalf("findMatchSnippet() = %q, want snippet containing match", got)
	}
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "...") {
		t.Fatalf("findMatchSnippet() = %q, want ellipses on both sides", got)
	}

	if got := findMatchSnippet(text, "missing", 12); got != "" {
		t.Fatalf("findMatchSnippet(no match) = %q, want empty string", got)
	}
}

func TestRenderHelpBar(t *testing.T) {
	got := stripANSI(renderHelpBar([][2]string{
		{"j/k", "Scroll"},
		{"q", "Quit"},
	}))

	if !strings.Contains(got, "j/k") || !strings.Contains(got, "Scroll") {
		t.Fatalf("renderHelpBar() = %q, want first key and description", got)
	}
	if !strings.Contains(got, "q") || !strings.Contains(got, "Quit") {
		t.Fatalf("renderHelpBar() = %q, want second key and description", got)
	}
	if !strings.Contains(got, "  ") {
		t.Fatalf("renderHelpBar() = %q, want separator spacing between help items", got)
	}
}

func TestRenderTitleBar(t *testing.T) {
	const width = 40

	got := stripANSI(renderTitleBar("Sessions (3)", "Sort: recent", width))
	if !strings.Contains(got, "Sessions (3)") {
		t.Fatalf("renderTitleBar() = %q, want left title", got)
	}
	if !strings.Contains(got, "Sort: recent") {
		t.Fatalf("renderTitleBar() = %q, want right title", got)
	}

	gotWidth := displayWidth(got)
	if gotWidth < width-2 {
		t.Fatalf("renderTitleBar() width = %d, want at least %d", gotWidth, width-2)
	}
}
