package stats

import (
	"strings"
	"testing"
	"time"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

func TestCalculateDashboard(t *testing.T) {
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
		{
			ID:          "d",
			ProjectPath: "/Users/example/delta",
			Title:       "OpenCode thread",
			LastTime:    now.Add(-6 * time.Hour),
			StartTime:   now.Add(-7 * time.Hour),
			MsgCount:    4,
			ToolCount:   2,
			TokenUsage:  session.TokenUsage{Input: 200, Output: 100},
			Model:       "minimax/MiniMax-M3",
			Source:      session.SourceOpenCode,
		},
	}

	stats := Calculate(sessions, now)
	if stats.TotalSessions != 4 || stats.TotalProjects != 3 {
		t.Fatalf("unexpected counts: %#v", stats)
	}
	sourceCounts := map[string]int{}
	for _, bucket := range stats.SourceBuckets {
		sourceCounts[bucket.Label] = bucket.Count
	}
	if sourceCounts[string(session.SourceClaude)] != 2 ||
		sourceCounts[string(session.SourceCodex)] != 1 ||
		sourceCounts[string(session.SourceOpenCode)] != 1 {
		t.Fatalf("unexpected source split: %#v", stats.SourceBuckets)
	}
	if stats.Active24Hours != 2 || stats.Active7Days != 3 || stats.Active30Days != 4 || stats.OlderSessions != 0 {
		t.Fatalf("unexpected active counts: %#v", stats)
	}
	if stats.TotalTools != 16 || stats.TotalTokens != 5950 {
		t.Fatalf("unexpected totals: %#v", stats)
	}
	if stats.TotalTokensIn != 4300 || stats.TotalTokensOut != 1650 {
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
	if !strings.Contains(stats.Insights[len(stats.Insights)-1], "ordered by recency, tokens, tool calls, and turns") {
		t.Fatalf("resume insight should explain the ranking: %#v", stats.Insights)
	}
}

func TestFilterByDuration(t *testing.T) {
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
		dur  time.Duration
		want []string
	}{
		{name: "all", dur: 0, want: []string{"day", "week", "month", "old", "unknown"}},
		{name: "30d", dur: 30 * 24 * time.Hour, want: []string{"day", "week", "month"}},
		{name: "7d", dur: 7 * 24 * time.Hour, want: []string{"day", "week"}},
		{name: "24h", dur: 24 * time.Hour, want: []string{"day"}},
	}

	for _, tt := range checks {
		t.Run(tt.name, func(t *testing.T) {
			gotSessions := FilterByDuration(sessions, now, tt.dur)
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
