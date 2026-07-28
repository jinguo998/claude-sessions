package stats

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
)

type ProjectRow struct {
	Label       string
	Sessions    int
	Active7Days int
	Tokens      int
}

type ModelRow struct {
	Label    string
	Sessions int
	Tokens   int
}

type Bucket struct {
	Label string
	Count int
}

type SessionRank struct {
	Session  session.Session
	Label    string
	Project  string
	Source   session.Source
	LastTime time.Time
	Tokens   int
	Tools    int
	Turns    int
	Score    int
	Why      string
}

type Dashboard struct {
	TotalSessions   int
	TotalProjects   int
	Active24Hours   int
	Active7Days     int
	Active30Days    int
	OlderSessions   int
	AverageTurns    float64
	AverageTools    float64
	AverageTokens   float64
	TotalTools      int
	TotalTokens     int
	TotalTokensIn   int
	TotalTokensOut  int
	ActivityBuckets []Bucket
	SourceBuckets   []Bucket
	TokenBuckets    []Bucket
	TopProjects     []ProjectRow
	TopModels       []ModelRow
	ResumeQueue     []SessionRank
	Insights        []string
}

func FilterByDuration(sessions []session.Session, now time.Time, dur time.Duration) []session.Session {
	if dur <= 0 {
		return sessions
	}
	filtered := make([]session.Session, 0, len(sessions))
	for _, sess := range sessions {
		if sess.LastTime.IsZero() {
			continue
		}
		if now.Sub(sess.LastTime) <= dur {
			filtered = append(filtered, sess)
		}
	}
	return filtered
}

func Calculate(sessions []session.Session, now time.Time) Dashboard {
	stats := Dashboard{TotalSessions: len(sessions)}
	if len(sessions) == 0 {
		return stats
	}

	projects := make(map[string]*ProjectRow)
	models := make(map[string]*ModelRow)
	sourceCounts := make(map[session.Source]int)
	totalTurns := 0

	var dayBucket, weekBucket, monthBucket int

	for _, sess := range sessions {
		project := sess.ProjectShortName()
		if project == "" {
			project = sess.ProjectDir
		}
		if project != "" {
			row := projects[project]
			if row == nil {
				row = &ProjectRow{Label: project}
				projects[project] = row
			}
			row.Sessions++
			row.Tokens += sess.TotalTokens()
		}

		if sess.Model != "" {
			row := models[sess.Model]
			if row == nil {
				row = &ModelRow{Label: sess.Model}
				models[sess.Model] = row
			}
			row.Sessions++
			row.Tokens += sess.TotalTokens()
		}

		sourceCounts[sess.Source]++

		age := time.Duration(1<<63 - 1)
		if !sess.LastTime.IsZero() {
			age = now.Sub(sess.LastTime)
		}
		switch {
		case age <= 24*time.Hour:
			dayBucket++
			stats.Active24Hours++
			stats.Active7Days++
			stats.Active30Days++
			if project != "" {
				projects[project].Active7Days++
			}
		case age <= 7*24*time.Hour:
			weekBucket++
			stats.Active7Days++
			stats.Active30Days++
			if project != "" {
				projects[project].Active7Days++
			}
		case age <= 30*24*time.Hour:
			monthBucket++
			stats.Active30Days++
		default:
			stats.OlderSessions++
		}

		totalTurns += sess.MsgCount
		stats.TotalTools += sess.ToolCount
		stats.TotalTokens += sess.TotalTokens()
		stats.TotalTokensIn += sess.TokenUsage.Input
		stats.TotalTokensOut += sess.TokenUsage.Output

		stats.ResumeQueue = append(stats.ResumeQueue, SessionRank{
			Session:  sess,
			Label:    truncateLabel(sessionLabel(sess)),
			Project:  project,
			Source:   sess.Source,
			LastTime: sess.LastTime,
			Tokens:   sess.TotalTokens(),
			Tools:    sess.ToolCount,
			Turns:    sess.MsgCount,
			Score:    resumeScore(sess, age),
			Why:      resumeReason(sess, age),
		})
	}

	stats.TotalProjects = len(projects)
	stats.AverageTurns = float64(totalTurns) / float64(len(sessions))
	stats.AverageTools = float64(stats.TotalTools) / float64(len(sessions))
	stats.AverageTokens = float64(stats.TotalTokens) / float64(len(sessions))
	stats.ActivityBuckets = []Bucket{
		{Label: "24h", Count: dayBucket},
		{Label: "2-7d", Count: weekBucket},
		{Label: "8-30d", Count: monthBucket},
		{Label: "30d+", Count: stats.OlderSessions},
	}
	stats.SourceBuckets = sourceBuckets(sourceCounts)
	stats.TokenBuckets = []Bucket{
		{Label: "Input", Count: stats.TotalTokensIn},
		{Label: "Output", Count: stats.TotalTokensOut},
	}
	stats.TopProjects = topProjectRows(projects, 5)
	stats.TopModels = topModelRows(models, 4)
	stats.ResumeQueue = topResumeQueue(stats.ResumeQueue, 5)
	stats.Insights = buildInsights(stats)
	return stats
}

func sourceBuckets(values map[session.Source]int) []Bucket {
	rows := make([]Bucket, 0, len(values))
	for source, count := range values {
		rows = append(rows, Bucket{Label: string(source), Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Label < rows[j].Label
	})
	return rows
}

func resumeScore(sess session.Session, age time.Duration) int {
	recency := 1
	switch {
	case age <= 24*time.Hour:
		recency = 4
	case age <= 7*24*time.Hour:
		recency = 3
	case age <= 30*24*time.Hour:
		recency = 2
	}
	return recency*100000 + sess.TotalTokens()*10 + sess.ToolCount*250 + sess.MsgCount*25
}

func resumeReason(sess session.Session, age time.Duration) string {
	switch {
	case age <= 24*time.Hour && sess.TotalTokens() >= 1_000_000:
		return "today, >1M tok"
	case age <= 24*time.Hour:
		return "used today"
	case sess.ToolCount >= 100:
		return fmt.Sprintf("%d tools", sess.ToolCount)
	case sess.TotalTokens() >= 1_000_000:
		return fmt.Sprintf("%.1fM tokens", float64(sess.TotalTokens())/1_000_000)
	default:
		return fmt.Sprintf("%d turns", sess.MsgCount)
	}
}

func topProjectRows(values map[string]*ProjectRow, limit int) []ProjectRow {
	rows := make([]ProjectRow, 0, len(values))
	for _, row := range values {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Sessions != rows[j].Sessions {
			return rows[i].Sessions > rows[j].Sessions
		}
		if rows[i].Active7Days != rows[j].Active7Days {
			return rows[i].Active7Days > rows[j].Active7Days
		}
		if rows[i].Tokens != rows[j].Tokens {
			return rows[i].Tokens > rows[j].Tokens
		}
		return rows[i].Label < rows[j].Label
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func topModelRows(values map[string]*ModelRow, limit int) []ModelRow {
	rows := make([]ModelRow, 0, len(values))
	for _, row := range values {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Sessions != rows[j].Sessions {
			return rows[i].Sessions > rows[j].Sessions
		}
		if rows[i].Tokens != rows[j].Tokens {
			return rows[i].Tokens > rows[j].Tokens
		}
		return rows[i].Label < rows[j].Label
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func topResumeQueue(rows []SessionRank, limit int) []SessionRank {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		return rows[i].LastTime.After(rows[j].LastTime)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func buildInsights(stats Dashboard) []string {
	var insights []string
	if len(stats.TopProjects) > 0 && stats.TotalSessions > 0 {
		top := stats.TopProjects[0]
		pct := int(float64(top.Sessions) / float64(stats.TotalSessions) * 100)
		insights = append(insights, fmt.Sprintf("%s: %d of %d sessions (%d%%).", top.Label, top.Sessions, stats.TotalSessions, pct))
	}
	switch {
	case stats.Active7Days == stats.TotalSessions && stats.TotalSessions > 0:
		insights = append(insights, fmt.Sprintf("All %d sessions were used in the last 7 days.", stats.TotalSessions))
	case stats.OlderSessions > stats.TotalSessions/2:
		insights = append(insights, fmt.Sprintf("%d of %d sessions are older than 30 days.", stats.OlderSessions, stats.TotalSessions))
	default:
		insights = append(insights, fmt.Sprintf("Used in the last 7 days: %d. Older than 30 days: %d.", stats.Active7Days, stats.OlderSessions))
	}
	if len(stats.ResumeQueue) > 0 {
		insights = append(insights, "Resume candidates are ordered by recency, tokens, tool calls, and turns.")
	}
	return insights
}

func sessionLabel(sess session.Session) string {
	switch {
	case sess.Title != "":
		return sess.Title
	case sess.FirstMsg != "":
		return sess.FirstMsg
	default:
		return sess.ID
	}
}

func truncateLabel(s string) string {
	s = strings.TrimSpace(s)
	if runewidth.StringWidth(s) <= 42 {
		return s
	}
	var b strings.Builder
	width := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if width+rw > 42 {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String()
}
