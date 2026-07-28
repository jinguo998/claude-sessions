package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	session "github.com/jinguo998/claude-sessions/internal/app/model"
	appstats "github.com/jinguo998/claude-sessions/internal/app/stats"
)

type statsProjectRow = appstats.ProjectRow
type statsModelRow = appstats.ModelRow
type statsBucket = appstats.Bucket
type statsSessionRank = appstats.SessionRank
type sessionStats = appstats.Dashboard

type statsRange int

const (
	statsRangeAll statsRange = iota
	statsRange30Days
	statsRange7Days
	statsRange24Hours
)

var statsRangeOrder = []statsRange{
	statsRangeAll,
	statsRange30Days,
	statsRange7Days,
	statsRange24Hours,
}

func (r statsRange) label() string {
	switch r {
	case statsRange24Hours:
		return "24h"
	case statsRange7Days:
		return "7d"
	case statsRange30Days:
		return "30d"
	default:
		return "all"
	}
}

func (r statsRange) duration() time.Duration {
	switch r {
	case statsRange24Hours:
		return 24 * time.Hour
	case statsRange7Days:
		return 7 * 24 * time.Hour
	case statsRange30Days:
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}

type statsQueueRender struct {
	panel        string
	rowStartLine int
	rowCount     int
	width        int
}

type statsRenderResult struct {
	content       string
	queueRowStart int
	queueRowCount int
	queueColWidth int
}

type StatsModel struct {
	sessions      []session.Session
	totalCount    int
	scopeSummary  string
	sources       session.SourceRegistry
	width         int
	height        int
	cursor        int
	lastClickTime time.Time
	lastClickIdx  int
	rangeMode     statsRange
}

func NewStatsModel(sources ...session.SourceRegistry) StatsModel {
	registry := session.SourceRegistry{}
	if len(sources) > 0 {
		registry = sources[0]
	}
	return StatsModel{lastClickIdx: -1, sources: registry}
}

func (s StatsModel) Open(sessions []session.Session, totalCount int, scopeSummary string) StatsModel {
	s.sessions = sessions
	s.totalCount = totalCount
	s.scopeSummary = scopeSummary
	s.cursor = 0
	s.lastClickIdx = -1
	return s.clampCursor()
}

func (s StatsModel) SetSize(w, h int) StatsModel {
	s.width = w
	s.height = h
	return s
}

func (s StatsModel) SetSessions(sessions []session.Session, totalCount int, scopeSummary string) StatsModel {
	s.sessions = sessions
	s.totalCount = totalCount
	s.scopeSummary = scopeSummary
	return s.clampCursor()
}

func (s StatsModel) Cursor() int {
	return s.cursor
}

func (s StatsModel) SelectedSession() (session.Session, bool) {
	now := time.Now()
	stats := calculateSessionStats(s.rangeSessions(now), now)
	if s.cursor < 0 || s.cursor >= len(stats.ResumeQueue) {
		return session.Session{}, false
	}
	return stats.ResumeQueue[s.cursor].Session, true
}

func (s StatsModel) Update(msg tea.Msg) (StatsModel, tea.Cmd) {
	now := time.Now()
	rangeSessions := s.rangeSessions(now)
	stats := calculateSessionStats(rangeSessions, now)
	s = s.clampForQueue(len(stats.ResumeQueue))

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return s.updateKey(msg, stats)
	case tea.MouseMsg:
		return s.updateMouse(msg, stats)
	default:
		return s, nil
	}
}

func (s StatsModel) View() string {
	now := time.Now()
	rangeSessions := s.rangeSessions(now)
	stats := calculateSessionStats(rangeSessions, now)
	rendered := renderStatsDashboardResult(stats, len(rangeSessions), s.totalCount, s.width, s.scopeSummaryWithRange(), s.cursor, s.sources)
	return rendered.content
}

func (s StatsModel) RenderResultForTest(now time.Time) statsRenderResult {
	rangeSessions := s.rangeSessions(now)
	return renderStatsDashboardResult(calculateSessionStats(rangeSessions, now), len(rangeSessions), s.totalCount, s.width, s.scopeSummaryWithRange(), s.cursor, s.sources)
}

func (s StatsModel) updateKey(msg tea.KeyMsg, stats sessionStats) (StatsModel, tea.Cmd) {
	maxIdx := len(stats.ResumeQueue) - 1
	switch msg.String() {
	case "]", "right":
		s = s.nextRange()
		return s.clampCursor(), nil
	case "[", "left":
		s = s.prevRange()
		return s.clampCursor(), nil
	case "1":
		s.rangeMode = statsRange24Hours
		return s.clampCursor(), nil
	case "2":
		s.rangeMode = statsRange7Days
		return s.clampCursor(), nil
	case "3":
		s.rangeMode = statsRange30Days
		return s.clampCursor(), nil
	case "4", "0", "a":
		s.rangeMode = statsRangeAll
		return s.clampCursor(), nil
	case "esc", "q", "t":
		return s, func() tea.Msg { return StatsCloseMsg{} }
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
		return s, nil
	case "down", "j":
		if s.cursor < maxIdx {
			s.cursor++
		}
		return s, nil
	case "enter", "p":
		if maxIdx >= 0 {
			return s, func() tea.Msg { return StatsPreviewMsg{Session: stats.ResumeQueue[s.cursor].Session} }
		}
		return s, nil
	case "f":
		if maxIdx >= 0 {
			project := stats.ResumeQueue[s.cursor].Session.ProjectPath
			if project != "" {
				return s, func() tea.Msg { return StatsProjectFilterMsg{ProjectPath: project} }
			}
		}
		return s, nil
	case "y":
		if maxIdx >= 0 {
			id := stats.ResumeQueue[s.cursor].Session.ID
			return s, func() tea.Msg { return CopyIDMsg{ID: id} }
		}
		return s, nil
	case "o":
		if maxIdx >= 0 {
			filePath := stats.ResumeQueue[s.cursor].Session.FilePath
			if filePath != "" {
				return s, func() tea.Msg { return OpenEditorMsg{FilePath: filePath} }
			}
		}
		return s, nil
	case "l":
		if maxIdx >= 0 {
			return s, func() tea.Msg { return StatsListFocusMsg{Session: stats.ResumeQueue[s.cursor].Session} }
		}
		return s, nil
	case "r":
		if maxIdx >= 0 {
			return s, func() tea.Msg { return defaultResumeSelection(s.sources, stats.ResumeQueue[s.cursor].Session) }
		}
		return s, nil
	case "R":
		if maxIdx >= 0 {
			return s, func() tea.Msg { return safeResumeSelection(stats.ResumeQueue[s.cursor].Session) }
		}
		return s, nil
	default:
		return s, nil
	}
}

func (s StatsModel) updateMouse(msg tea.MouseMsg, stats sessionStats) (StatsModel, tea.Cmd) {
	rendered := renderStatsDashboardResult(stats, stats.TotalSessions, s.totalCount, s.width, s.scopeSummaryWithRange(), s.cursor, s.sources)

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if s.cursor > 0 {
			s.cursor--
		}
		return s, nil
	case tea.MouseButtonWheelDown:
		if s.cursor < rendered.queueRowCount-1 {
			s.cursor++
		}
		return s, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return s, nil
	}
	if rendered.queueColWidth > 0 && msg.X >= rendered.queueColWidth {
		return s, nil
	}
	idx := msg.Y - rendered.queueRowStart
	if idx < 0 || idx >= rendered.queueRowCount {
		return s, nil
	}

	now := time.Now()
	if idx == s.lastClickIdx && now.Sub(s.lastClickTime) < doubleClickThreshold {
		s.cursor = idx
		s.lastClickIdx = -1
		return s, func() tea.Msg { return StatsPreviewMsg{Session: stats.ResumeQueue[idx].Session} }
	}

	s.cursor = idx
	s.lastClickIdx = idx
	s.lastClickTime = now
	return s, nil
}

func (s StatsModel) clampCursor() StatsModel {
	now := time.Now()
	stats := calculateSessionStats(s.rangeSessions(now), now)
	return s.clampForQueue(len(stats.ResumeQueue))
}

func (s StatsModel) clampForQueue(queueLen int) StatsModel {
	if s.cursor < 0 {
		s.cursor = 0
	}
	if queueLen == 0 {
		s.cursor = 0
		return s
	}
	if s.cursor >= queueLen {
		s.cursor = queueLen - 1
	}
	return s
}

func (s StatsModel) nextRange() StatsModel {
	idx := statsRangeIndex(s.rangeMode)
	s.rangeMode = statsRangeOrder[(idx+1)%len(statsRangeOrder)]
	return s
}

func (s StatsModel) prevRange() StatsModel {
	idx := statsRangeIndex(s.rangeMode)
	s.rangeMode = statsRangeOrder[(idx+len(statsRangeOrder)-1)%len(statsRangeOrder)]
	return s
}

func statsRangeIndex(mode statsRange) int {
	for i, candidate := range statsRangeOrder {
		if candidate == mode {
			return i
		}
	}
	return 0
}

func (s StatsModel) scopeSummaryWithRange() string {
	rangePart := "range=" + s.rangeMode.label()
	if strings.TrimSpace(s.scopeSummary) == "" {
		return "scope: " + rangePart
	}
	if strings.HasPrefix(s.scopeSummary, "scope: ") {
		return s.scopeSummary + "  |  " + rangePart
	}
	return s.scopeSummary + "  |  " + rangePart
}

func (s StatsModel) rangeSessions(now time.Time) []session.Session {
	return filterSessionsByStatsRange(s.sessions, now, s.rangeMode)
}

func filterSessionsByStatsRange(sessions []session.Session, now time.Time, mode statsRange) []session.Session {
	return appstats.FilterByDuration(sessions, now, mode.duration())
}

func calculateSessionStats(sessions []session.Session, now time.Time) sessionStats {
	return appstats.Calculate(sessions, now)
}

func formatCompactNumber(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func averageInt(total, count int) int {
	if count <= 0 {
		return 0
	}
	return total / count
}

func statsSessionLabel(sess session.Session) string {
	switch {
	case sess.Title != "":
		return sess.Title
	case sess.FirstMsg != "":
		return sess.FirstMsg
	default:
		return sess.ID
	}
}

func truncateStatLabel(s string) string {
	s = strings.TrimSpace(s)
	if displayWidth(s) > 42 {
		return truncateToWidth(s, 42)
	}
	return s
}

func bar(count, maxCount, width int) string {
	if width < 1 {
		width = 1
	}
	if maxCount <= 0 || count <= 0 {
		return strings.Repeat(".", width)
	}
	filled := count * width / maxCount
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled) + strings.Repeat(".", width-filled)
}

func maxBucketCount(items []statsBucket) int {
	maxCount := 0
	for _, item := range items {
		if item.Count > maxCount {
			maxCount = item.Count
		}
	}
	return maxCount
}

func renderStatsDashboard(stats sessionStats, shownCount, totalCount, width int, scopeSummary string, selectedQueue int) string {
	return renderStatsDashboardResult(stats, shownCount, totalCount, width, scopeSummary, selectedQueue, session.SourceRegistry{}).content
}

func renderStatsDashboardResult(stats sessionStats, shownCount, totalCount, width int, scopeSummary string, selectedQueue int, sources session.SourceRegistry) statsRenderResult {
	scopeLabel := fmt.Sprintf("  Session statistics (%d shown", shownCount)
	if totalCount != shownCount {
		scopeLabel += fmt.Sprintf(" / %d total", totalCount)
	}
	scopeLabel += ")"
	title := statsTitleBarStyle.Render(scopeLabel + strings.Repeat(" ", max(width-lipgloss.Width(scopeLabel)-4, 1)))

	sections := []string{
		statsMutedStyle.Render("  " + scopeSummary),
		renderStatsOverview(stats, width),
		renderStatsMiddle(stats, width, sources),
		renderStatsTables(stats, width),
	}

	var b strings.Builder
	line := 0
	b.WriteString(title)
	line += countRenderedLines(title)
	b.WriteString("\n\n")
	line += 2

	for _, section := range sections {
		b.WriteString(section)
		line += countRenderedLines(section)
		b.WriteString("\n\n")
		line += 2
	}

	queueContentWidth := max(width-2, 38)
	detailWidth := max(width, 34)
	sideBySide := selectedQueue >= 0 && selectedQueue < len(stats.ResumeQueue) && width >= 160
	if sideBySide {
		detailWidth = max(width/3, 44)
		queueOuterWidth := width - detailWidth - 2
		queueContentWidth = max(queueOuterWidth-2, 38)
	}

	queue := renderStatsQueue(stats, queueContentWidth, selectedQueue, sources)
	queueRowStart := line + queue.rowStartLine
	queueSection := queue.panel
	if detail := renderSelectedSession(stats, detailWidth, selectedQueue, sources); detail != "" {
		if sideBySide {
			queueSection = lipgloss.JoinHorizontal(lipgloss.Top, queue.panel, "  ", detail)
		} else {
			queueSection = queue.panel + "\n\n" + detail
		}
	}
	b.WriteString(queueSection)
	line += countRenderedLines(queueSection)
	b.WriteString("\n\n")
	line += 2

	footerText := "q close  [/]/1-4 range  j/k move  Enter/p preview  r/R resume  f project  l list  y copy  o open"
	footerLines := strings.ReplaceAll(wrapText(footerText, max(width-4, 20)), "\n", "\n  ")
	footer := "  " + dimStyle.Render(footerLines)
	b.WriteString(footer)

	return statsRenderResult{
		content:       b.String(),
		queueRowStart: queueRowStart,
		queueRowCount: queue.rowCount,
		queueColWidth: queue.width,
	}
}

func renderStatsOverview(stats sessionStats, width int) string {
	lines := []string{
		fmt.Sprintf("Sessions %d   Projects %d   Active in 7d %d   Active in 24h %d", stats.TotalSessions, stats.TotalProjects, stats.Active7Days, stats.Active24Hours),
		fmt.Sprintf("Tokens %s   Input %s   Output %s", formatCompactNumber(stats.TotalTokens), formatCompactNumber(stats.TotalTokensIn), formatCompactNumber(stats.TotalTokensOut)),
		fmt.Sprintf("Average per session %.1f turns   %.1f tool calls", stats.AverageTurns, stats.AverageTools),
	}
	return renderStatsSection("Overview", lines, width)
}

func renderStatsMiddle(stats sessionStats, width int, sources session.SourceRegistry) string {
	sectionWidth := statsColumnWidth(width, 2)
	if width < 100 {
		sectionWidth = width
	}
	left := renderStatsSection("Session summary", prefixLines(stats.Insights, "- "), sectionWidth)
	rightLines := append(renderBucketLines("Sessions by age", stats.ActivityBuckets, 14), "")
	rightLines = append(rightLines, renderBucketLines("Sessions by source", displaySourceBuckets(stats.SourceBuckets, sources), 14)...)
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, renderBucketLines("Tokens by direction", stats.TokenBuckets, 14)...)
	right := renderStatsSection("Distribution", rightLines, sectionWidth)
	if width < 100 {
		return strings.Join([]string{left, right}, "\n\n")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func renderStatsTables(stats sessionStats, width int) string {
	sectionWidth := statsColumnWidth(width, 2)
	if width < 100 {
		sectionWidth = width
	}
	projectLines := []string{"Proj                Ses  7d     Avg  Tokens"}
	for _, row := range stats.TopProjects {
		projectLines = append(projectLines, fmt.Sprintf("%-18s %3d %3d %7s %7s",
			truncateToWidth(row.Label, 18), row.Sessions, row.Active7Days, formatCompactNumber(averageInt(row.Tokens, row.Sessions)), formatCompactNumber(row.Tokens)))
	}
	if len(stats.TopProjects) == 0 {
		projectLines = append(projectLines, "None")
	}

	modelLines := []string{"Model               Ses     Avg  Tokens"}
	for _, row := range stats.TopModels {
		modelLines = append(modelLines, fmt.Sprintf("%-18s %3d %7s %7s",
			truncateToWidth(row.Label, 18), row.Sessions, formatCompactNumber(averageInt(row.Tokens, row.Sessions)), formatCompactNumber(row.Tokens)))
	}
	if len(stats.TopModels) == 0 {
		modelLines = append(modelLines, "None")
	}

	left := renderStatsSection("Projects", projectLines, sectionWidth)
	right := renderStatsSection("Models", modelLines, sectionWidth)
	if width < 100 {
		return strings.Join([]string{left, right}, "\n\n")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func renderStatsQueue(stats sessionStats, width int, selected int, sources session.SourceRegistry) statsQueueRender {
	lines := []string{}
	compact := width < 112
	if compact {
		lines = append(lines, "Src  Last seen   Reason          Project         Session")
	} else {
		lines = append(lines, "Src  Last seen   Reason          Project            Session                      Tokens Tools Turns")
	}
	for i, row := range stats.ResumeQueue {
		var line string
		if compact {
			sessionWidth := max(width-52, 12)
			line = fmt.Sprintf("%-3s  %-10s  %-15s %-14s %s",
				queueSource(row.Source, sources),
				relativeTime(row.LastTime),
				truncateToWidth(row.Why, 15),
				truncateToWidth(row.Project, 14),
				truncateToWidth(row.Label, sessionWidth),
			)
		} else {
			line = fmt.Sprintf("%-3s  %-10s  %-15s %-16s %-28s %6s %5d %5d",
				queueSource(row.Source, sources),
				relativeTime(row.LastTime),
				truncateToWidth(row.Why, 15),
				truncateToWidth(row.Project, 16),
				truncateToWidth(row.Label, 28),
				formatCompactNumber(row.Tokens),
				row.Tools,
				row.Turns,
			)
		}
		if i == selected {
			line = statsQueueSelectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if len(stats.ResumeQueue) == 0 {
		lines = append(lines, "No sessions to rank")
	}
	return statsQueueRender{
		panel:        renderStatsPanel("Resume candidates", lines, width),
		rowStartLine: 4,
		rowCount:     len(stats.ResumeQueue),
		width:        width + 2,
	}
}

func renderSelectedSession(stats sessionStats, width int, selected int, sources session.SourceRegistry) string {
	if len(stats.ResumeQueue) == 0 || selected < 0 || selected >= len(stats.ResumeQueue) {
		return ""
	}
	row := stats.ResumeQueue[selected]
	sess := row.Session

	title := statsSessionLabel(sess)
	valueWidth := max(width-12, 12)
	lines := []string{
		detailValueStyle.Render(truncateToWidth(title, max(width-2, 12))),
		"",
		fmt.Sprintf("%s%s  %s%s",
			detailLabelStyle.Render("Why: "), detailValueStyle.Render(row.Why),
			detailLabelStyle.Render("Last seen: "), detailValueStyle.Render(relativeTime(row.LastTime)),
		),
		fmt.Sprintf("%s%s  %s%s",
			detailLabelStyle.Render("Source: "), detailValueStyle.Render(sources.Info(sess.Source).Label),
			detailLabelStyle.Render("Model: "), detailValueStyle.Render(orValue(sess.Model, "unknown")),
		),
		fmt.Sprintf("%s%s  %s%d  %s%d",
			detailLabelStyle.Render("Tokens: "), detailValueStyle.Render(formatCompactNumber(row.Tokens)),
			detailLabelStyle.Render("Tools: "), row.Tools,
			detailLabelStyle.Render("Turns: "), row.Turns,
		),
		fmt.Sprintf("%s%s  %s%s",
			detailLabelStyle.Render("Token in: "), detailValueStyle.Render(formatCompactNumber(sess.TokenUsage.Input)),
			detailLabelStyle.Render("out: "), detailValueStyle.Render(formatCompactNumber(sess.TokenUsage.Output)),
		),
		fmt.Sprintf("%s%s", detailLabelStyle.Render("Project: "), detailValueStyle.Render(truncateToWidth(orValue(sess.ProjectPath, row.Project), valueWidth))),
	}
	if sess.LastMsg != "" && sess.LastMsg != sess.FirstMsg {
		lines = append(lines, fmt.Sprintf("%s%s", detailLabelStyle.Render("Last msg: "), detailValueStyle.Render(truncateToWidth(sess.LastMsg, valueWidth))))
	}
	return renderStatsSection("Selected session", lines, width)
}

func orValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func renderBucketLines(title string, buckets []statsBucket, barWidth int) []string {
	lines := []string{title}
	maxCount := maxBucketCount(buckets)
	for _, bucket := range buckets {
		lines = append(lines, fmt.Sprintf("%-6s [%s] %d", bucket.Label, bar(bucket.Count, maxCount, barWidth), bucket.Count))
	}
	return lines
}

func renderStatsPanel(title string, lines []string, width int) string {
	if width < 24 {
		width = 24
	}
	content := []string{statsSectionTitleStyle.Render(title), ""}
	for _, line := range lines {
		content = append(content, line)
	}
	return statsPanelStyle.Width(width).Render(strings.Join(content, "\n"))
}

func renderStatsSection(title string, lines []string, width int) string {
	if width < 24 {
		width = 24
	}
	content := []string{
		"  " + statsSectionTitleStyle.Render(title),
		"  " + borderStyle.Render(strings.Repeat("─", max(width-2, 1))),
	}
	for _, line := range lines {
		content = append(content, "  "+line)
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(content, "\n"))
}

func prefixLines(lines []string, prefix string) []string {
	if len(lines) == 0 {
		return []string{"None"}
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, prefix+line)
	}
	return out
}

func statsColumnWidth(totalWidth, cols int) int {
	gaps := (cols - 1) * 2
	return max((totalWidth-gaps)/cols, 24)
}

func displaySourceBuckets(buckets []statsBucket, sources session.SourceRegistry) []statsBucket {
	out := make([]statsBucket, len(buckets))
	for i, bucket := range buckets {
		out[i] = bucket
		out[i].Label = sources.Info(session.Source(bucket.Label)).Label
	}
	return out
}

func queueSource(src session.Source, sources session.SourceRegistry) string {
	return sources.Info(src).Badge
}

func countRenderedLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
