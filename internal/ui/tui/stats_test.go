package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func TestCalculateSessionStats(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	sessions := []session.Session{
		{
			ID:          "a",
			ProjectPath: "/Users/example/alpha",
			FirstMsg:    "Fix alpha bug",
			LastTime:    now.Add(-2 * time.Hour),
			StartTime:   now.Add(-3 * time.Hour),
			MsgCount:    10,
			ToolCount:   4,
			TokenUsage:  session.TokenUsage{Input: 1000, Output: 500},
			Model:       "sonnet",
			Source:      session.SourceClaude,
		},
		{
			ID:          "b",
			ProjectPath: "/Users/example/bravo",
			FirstMsg:    "Refactor bravo",
			LastTime:    now.Add(-3 * 24 * time.Hour),
			StartTime:   now.Add(-3*24*time.Hour - 90*time.Minute),
			MsgCount:    20,
			ToolCount:   9,
			TokenUsage:  session.TokenUsage{Input: 3000, Output: 1000},
			Model:       "sonnet",
			Source:      session.SourceClaude,
		},
		{
			ID:          "c",
			ProjectPath: "/Users/example/alpha",
			Title:       "Codex thread",
			LastTime:    now.Add(-10 * 24 * time.Hour),
			StartTime:   now.Add(-10*24*time.Hour - 30*time.Minute),
			MsgCount:    6,
			ToolCount:   1,
			TokenUsage:  session.TokenUsage{Input: 100, Output: 50},
			Model:       "gpt-5.4",
			Source:      session.SourceCodex,
		},
	}

	stats := calculateSessionStats(sessions, now)
	if stats.TotalSessions != 3 || stats.TotalProjects != 2 {
		t.Fatalf("unexpected counts: %#v", stats)
	}
	sourceCounts := map[string]int{}
	for _, bucket := range stats.SourceBuckets {
		sourceCounts[bucket.Label] = bucket.Count
	}
	if sourceCounts[string(session.SourceClaude)] != 2 || sourceCounts[string(session.SourceCodex)] != 1 {
		t.Fatalf("unexpected source split: %#v", stats.SourceBuckets)
	}
	if stats.Active24Hours != 1 || stats.Active7Days != 2 || stats.Active30Days != 3 || stats.OlderSessions != 0 {
		t.Fatalf("unexpected active counts: %#v", stats)
	}
	if stats.TotalTools != 14 || stats.TotalTokens != 5650 {
		t.Fatalf("unexpected totals: %#v", stats)
	}
	if stats.TotalTokensIn != 4100 || stats.TotalTokensOut != 1550 {
		t.Fatalf("unexpected token split: %#v", stats)
	}
	if len(stats.TopProjects) == 0 || stats.TopProjects[0].Label != "alpha" || stats.TopProjects[0].Sessions != 2 {
		t.Fatalf("unexpected top projects: %#v", stats.TopProjects)
	}
	if len(stats.TopModels) == 0 || stats.TopModels[0].Label != "sonnet" || stats.TopModels[0].Sessions != 2 {
		t.Fatalf("unexpected top models: %#v", stats.TopModels)
	}
	if len(stats.ResumeQueue) == 0 || stats.ResumeQueue[0].Label != "Fix alpha bug" {
		t.Fatalf("unexpected resume queue: %#v", stats.ResumeQueue)
	}
	if stats.ResumeQueue[0].Tokens != 1500 || stats.ResumeQueue[0].Tools != 4 {
		t.Fatalf("unexpected top queue metrics: %#v", stats.ResumeQueue[0])
	}
	if stats.ResumeQueue[0].Why != "used today" {
		t.Fatalf("resume queue reason = %q, want a direct activity reason", stats.ResumeQueue[0].Why)
	}
	if len(stats.Insights) < 3 {
		t.Fatalf("expected multiple insights, got %#v", stats.Insights)
	}
}

func TestFilterSessionsByStatsRange(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	sessions := []session.Session{
		{ID: "day", LastTime: now.Add(-2 * time.Hour)},
		{ID: "week", LastTime: now.Add(-3 * 24 * time.Hour)},
		{ID: "month", LastTime: now.Add(-20 * 24 * time.Hour)},
		{ID: "old", LastTime: now.Add(-45 * 24 * time.Hour)},
		{ID: "unknown"},
	}

	checks := []struct {
		name string
		mode statsRange
		want []string
	}{
		{name: "all", mode: statsRangeAll, want: []string{"day", "week", "month", "old", "unknown"}},
		{name: "30d", mode: statsRange30Days, want: []string{"day", "week", "month"}},
		{name: "7d", mode: statsRange7Days, want: []string{"day", "week"}},
		{name: "24h", mode: statsRange24Hours, want: []string{"day"}},
	}

	for _, tt := range checks {
		t.Run(tt.name, func(t *testing.T) {
			gotSessions := filterSessionsByStatsRange(sessions, now, tt.mode)
			got := make([]string, 0, len(gotSessions))
			for _, sess := range gotSessions {
				got = append(got, sess.ID)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("range sessions = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestStatsModelRangeKeys(t *testing.T) {
	now := time.Now()
	sessions := []session.Session{
		{ID: "day", ProjectPath: "/tmp/day", FirstMsg: "Day", LastTime: now.Add(-2 * time.Hour), Source: session.SourceClaude},
		{ID: "week", ProjectPath: "/tmp/week", FirstMsg: "Week", LastTime: now.Add(-3 * 24 * time.Hour), Source: session.SourceClaude},
		{ID: "month", ProjectPath: "/tmp/month", FirstMsg: "Month", LastTime: now.Add(-20 * 24 * time.Hour), Source: session.SourceClaude},
		{ID: "old", ProjectPath: "/tmp/old", FirstMsg: "Old", LastTime: now.Add(-45 * 24 * time.Hour), Source: session.SourceClaude},
	}
	m := NewStatsModel().SetSize(120, 30).Open(sessions, len(sessions), "scope: test")

	got := stripANSI(m.RenderResultForTest(now).content)
	if !strings.Contains(got, "Session statistics (4 shown)") || !strings.Contains(got, "range=all") {
		t.Fatalf("initial stats range render = %q", got)
	}

	m, _ = m.Update(testKeyRunes("2"))
	got = stripANSI(m.RenderResultForTest(now).content)
	if !strings.Contains(got, "Session statistics (2 shown / 4 total)") || !strings.Contains(got, "range=7d") {
		t.Fatalf("7d stats range render = %q", got)
	}

	m, _ = m.Update(testKeyRunes("1"))
	got = stripANSI(m.RenderResultForTest(now).content)
	if !strings.Contains(got, "Session statistics (1 shown / 4 total)") || !strings.Contains(got, "range=24h") {
		t.Fatalf("24h stats range render = %q", got)
	}

	m, _ = m.Update(testKeyRunes("4"))
	got = stripANSI(m.RenderResultForTest(now).content)
	if !strings.Contains(got, "Session statistics (4 shown)") || !strings.Contains(got, "range=all") {
		t.Fatalf("all stats range render = %q", got)
	}
}

func TestBar(t *testing.T) {
	if got := bar(0, 10, 5); got != "....." {
		t.Fatalf("bar(0) = %q, want dots", got)
	}
	if got := bar(5, 10, 5); got != "##..." {
		t.Fatalf("bar(5/10) = %q, want %q", got, "##...")
	}
	if got := bar(10, 10, 5); got != "#####" {
		t.Fatalf("bar(full) = %q, want %q", got, "#####")
	}
}

func TestRenderStatsUsesFilteredSessions(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	m := InitialModel()
	m.width = 120
	m.list = m.list.SetSessions([]session.Session{
		{ID: "a", ProjectPath: "/Users/example/alpha", FirstMsg: "Alpha", LastTime: now, MsgCount: 4, ToolCount: 2, TokenUsage: session.TokenUsage{Input: 1000}, Source: session.SourceClaude},
		{ID: "b", ProjectPath: "/Users/example/bravo", FirstMsg: "Bravo", LastTime: now, MsgCount: 8, ToolCount: 5, TokenUsage: session.TokenUsage{Input: 2000}, Source: session.SourceCodex},
	})
	m.list.filtered = m.list.filtered[:1]

	got := stripANSI(m.renderStats())
	checks := []string{
		"Session statistics (1 shown / 2 total)",
		"scope: entire visible workspace",
		"Overview",
		"Tokens by direction",
		"Projects",
		"Models",
		"Resume candidates",
		"Selected session",
		"alpha",
		"Alpha",
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("renderStats missing %q in %q", needle, got)
		}
	}
	for _, unwanted := range []string{"What Matters", "Pulse", "fresh-heavy", "tool-rich"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("renderStats retained template copy %q in %q", unwanted, got)
		}
	}
	if count := strings.Count(got, "q close"); count != 1 {
		t.Fatalf("stats help footer count = %d, want 1 in %q", count, got)
	}
}

func TestStatsLayoutFitsTerminal(t *testing.T) {
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	sessions := []session.Session{
		{
			ID:          "alpha",
			ProjectPath: "/Users/example/projects/alpha-with-a-long-project-name",
			FirstMsg:    "Fix the session browser layout without pushing details beyond the viewport",
			LastMsg:     "Verify the selected session remains visible beside the resume candidates",
			LastTime:    now.Add(-2 * time.Hour),
			MsgCount:    120,
			ToolCount:   150,
			TokenUsage:  session.TokenUsage{Input: 2_000_000, Output: 100_000},
			Source:      session.SourceClaude,
		},
	}

	for _, terminalWidth := range []int{80, 160} {
		t.Run(fmt.Sprintf("width_%d", terminalWidth), func(t *testing.T) {
			m := NewStatsModel().SetSize(terminalWidth, 60).Open(sessions, 1, "scope: test")
			got := stripANSI(m.RenderResultForTest(now).content)
			for i, line := range strings.Split(got, "\n") {
				if width := displayWidth(line); width > terminalWidth {
					t.Fatalf("stats line %d width = %d, want <= %d: %q", i+1, width, terminalWidth, line)
				}
			}
			if terminalWidth == 80 && !strings.Contains(got, "  - alpha-with-a-long-project-name: 1 of 1 sessions (100%).") {
				t.Fatalf("narrow stats sections should use the full terminal width: %q", got)
			}
		})
	}
}

func TestStatsModelActions(t *testing.T) {
	sessions := []session.Session{
		{
			ID:          "alpha",
			ProjectPath: "/Users/example/alpha",
			FilePath:    "/tmp/alpha.jsonl",
			FirstMsg:    "Alpha",
			LastTime:    time.Now().Add(-1 * time.Hour),
			Source:      session.SourceClaude,
		},
		{
			ID:          "bravo",
			ProjectPath: "/Users/example/bravo",
			FirstMsg:    "Bravo",
			LastTime:    time.Now().Add(-2 * time.Hour),
			Source:      session.SourceClaude,
		},
	}

	m := NewStatsModel().SetSize(120, 30).Open(sessions, len(sessions), "scope: test")
	m, _ = m.Update(testKeyRunes("j"))
	if m.Cursor() != 1 {
		t.Fatalf("cursor after j = %d, want 1", m.Cursor())
	}

	_, cmd := m.Update(testKey(tea.KeyEnter))
	if got, ok := runCmd(t, cmd).(StatsPreviewMsg); !ok || got.Session.ID != "bravo" {
		t.Fatalf("preview cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	_, cmd = m.Update(testKeyRunes("f"))
	if got, ok := runCmd(t, cmd).(StatsProjectFilterMsg); !ok || got.ProjectPath != "/Users/example/bravo" {
		t.Fatalf("filter cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}

	_, cmd = m.Update(testKeyRunes("l"))
	if got, ok := runCmd(t, cmd).(StatsListFocusMsg); !ok || got.Session.ID != "bravo" {
		t.Fatalf("focus cmd = %T %#v", runCmd(t, cmd), runCmd(t, cmd))
	}
}
