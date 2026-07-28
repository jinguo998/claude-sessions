package tui

import "github.com/charmbracelet/lipgloss"

// adaptive returns a color that works on both light and dark backgrounds.
func adaptive(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

var (
	accentColor    = adaptive("128", "170")
	mutedColor     = adaptive("242", "241")
	borderColor    = adaptive("249", "238")
	selectionColor = adaptive("57", "57")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor).
			Padding(0, 1)

	titleBarStyle = lipgloss.NewStyle().
			Background(adaptive("236", "236")).
			Foreground(adaptive("255", "255")).
			Bold(true).
			Padding(0, 1)

	normalStyle = lipgloss.NewStyle()

	dimStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(1, 0, 0, 1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	helpSepStyle = lipgloss.NewStyle().
			Foreground(borderColor)

	confirmStyle = lipgloss.NewStyle().
			Foreground(adaptive("196", "196")).
			Bold(true)

	userStyle = lipgloss.NewStyle().
			Foreground(adaptive("236", "250")).
			Bold(true).
			Padding(0, 1)

	assistantStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			Padding(0, 1)

	userTextStyle = lipgloss.NewStyle().
			Foreground(adaptive("234", "252"))

	assistantTextStyle = lipgloss.NewStyle().
				Foreground(adaptive("234", "252"))

	toolStyle = lipgloss.NewStyle().
			Foreground(adaptive("243", "243")).
			Italic(true)

	approvalStyle = lipgloss.NewStyle().
			Foreground(adaptive("242", "242"))

	toolResultStyle = lipgloss.NewStyle().
			Foreground(adaptive("240", "240"))

	highlightStyle = lipgloss.NewStyle().
			Reverse(true).
			Bold(true)

	loadingStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Padding(2, 2)

	emptyStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Padding(1, 2)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(accentColor)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(adaptive("234", "252"))

	pathStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)

	searchStatusStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	menuStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	menuItemStyle = lipgloss.NewStyle().
			Foreground(adaptive("234", "252"))

	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(adaptive("255", "255")).
				Background(selectionColor).
				Bold(true)

	selectedTrashStyle = lipgloss.NewStyle().
				Foreground(adaptive("255", "255")).
				Background(selectionColor).
				Bold(true)

	statsQueueSelectedStyle = lipgloss.NewStyle().
				Foreground(adaptive("255", "255")).
				Background(selectionColor).
				Bold(true)

	statsPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	statsSectionTitleStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	statsMutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	statsTitleBarStyle = lipgloss.NewStyle().
				Background(adaptive("236", "236")).
				Foreground(adaptive("255", "255")).
				Bold(true).
				Padding(0, 1)

	menuTitleStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	menuDividerStyle = lipgloss.NewStyle().
				Foreground(borderColor)

	borderStyle = lipgloss.NewStyle().Foreground(borderColor)

	claudeBadgeStyle = lipgloss.NewStyle().
				Foreground(adaptive("27", "75")). // strong blue on light, bright blue on dark
				Bold(true)

	codexBadgeStyle = lipgloss.NewStyle().
			Foreground(adaptive("22", "40")). // dark green on light, bright green on dark
			Bold(true)

	flashStyle = lipgloss.NewStyle().
			Foreground(adaptive("255", "255")).
			Background(selectionColor).
			Bold(true).
			Padding(0, 1)
)
