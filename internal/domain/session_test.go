package domain

import (
	"sort"
	"testing"
	"time"
)

func TestSessionDuration(t *testing.T) {
	start := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		session Session
		want    time.Duration
	}{
		{
			name:    "zero timestamps",
			session: Session{},
			want:    0,
		},
		{
			name: "under one minute",
			session: Session{
				StartTime: start,
				LastTime:  start.Add(30 * time.Second),
			},
			want: 30 * time.Second,
		},
		{
			name: "minutes",
			session: Session{
				StartTime: start,
				LastTime:  start.Add(42 * time.Minute),
			},
			want: 42 * time.Minute,
		},
		{
			name: "whole hours",
			session: Session{
				StartTime: start,
				LastTime:  start.Add(2 * time.Hour),
			},
			want: 2 * time.Hour,
		},
		{
			name: "hours and minutes",
			session: Session{
				StartTime: start,
				LastTime:  start.Add(2*time.Hour + 5*time.Minute),
			},
			want: 2*time.Hour + 5*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.Duration(); got != tt.want {
				t.Fatalf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionProjectShortName(t *testing.T) {
	tests := []struct {
		name string
		s    Session
		want string
	}{
		{
			name: "uses decoded path leaf",
			s: Session{
				ProjectDir:  "-Users-example-demo",
				ProjectPath: "/Users/example/demo/project",
			},
			want: "project",
		},
		{
			name: "trims trailing slash",
			s: Session{
				ProjectPath: "/Users/example/demo/project/",
			},
			want: "project",
		},
		{
			name: "falls back to project dir",
			s: Session{
				ProjectDir: "-Users-example-demo",
			},
			want: "-Users-example-demo",
		},
		{
			name: "path without slash",
			s: Session{
				ProjectPath: "project",
			},
			want: "project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.ProjectShortName(); got != tt.want {
				t.Fatalf("ProjectShortName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortByLastTimeDescending(t *testing.T) {
	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	sessions := []Session{
		{ID: "old", LastTime: now.Add(-3 * time.Hour)},
		{ID: "new", LastTime: now.Add(-1 * time.Hour)},
		{ID: "mid", LastTime: now.Add(-2 * time.Hour)},
	}

	sort.Sort(SortByLastTime(sessions))

	got := []string{sessions[0].ID, sessions[1].ID, sessions[2].ID}
	want := []string{"new", "mid", "old"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortByLastTime order = %v, want %v", got, want)
		}
	}
}

func TestSortByMsgCountDescending(t *testing.T) {
	sessions := []Session{
		{ID: "few", MsgCount: 3},
		{ID: "many", MsgCount: 10},
		{ID: "mid", MsgCount: 7},
	}

	sort.Sort(SortByMsgCount(sessions))

	got := []string{sessions[0].ID, sessions[1].ID, sessions[2].ID}
	want := []string{"many", "mid", "few"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortByMsgCount order = %v, want %v", got, want)
		}
	}
}

func TestTotalTokens(t *testing.T) {
	sess := Session{TokenUsage: TokenUsage{Input: 1200, Output: 800}}
	if got := sess.TotalTokens(); got != 2000 {
		t.Fatalf("TotalTokens() = %d, want 2000", got)
	}
}

func TestSortByToolCountDescending(t *testing.T) {
	sessions := []Session{
		{ID: "few", ToolCount: 1},
		{ID: "many", ToolCount: 10},
		{ID: "mid", ToolCount: 7},
	}

	sort.Sort(SortByToolCount(sessions))

	got := []string{sessions[0].ID, sessions[1].ID, sessions[2].ID}
	want := []string{"many", "mid", "few"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortByToolCount order = %v, want %v", got, want)
		}
	}
}

func TestSortByTotalTokensDescending(t *testing.T) {
	sessions := []Session{
		{ID: "none", TokenUsage: TokenUsage{}},
		{ID: "high", TokenUsage: TokenUsage{Input: 2000, Output: 3000}},
		{ID: "mid", TokenUsage: TokenUsage{Input: 500, Output: 600}},
	}

	sort.Sort(SortByTotalTokens(sessions))

	got := []string{sessions[0].ID, sessions[1].ID, sessions[2].ID}
	want := []string{"high", "mid", "none"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortByTotalTokens order = %v, want %v", got, want)
		}
	}
}
