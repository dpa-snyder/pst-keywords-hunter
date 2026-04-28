package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("#7C3AED") // Purple
	colorSecondary = lipgloss.Color("#06B6D4") // Cyan
	colorSuccess   = lipgloss.Color("#22C55E") // Green
	colorWarning   = lipgloss.Color("#F59E0B") // Amber
	colorDanger    = lipgloss.Color("#EF4444") // Red
	colorMuted     = lipgloss.Color("#6B7280") // Gray
	colorBg        = lipgloss.Color("#1E1B2E") // Dark bg
	colorFg        = lipgloss.Color("#E2E8F0") // Light fg
	colorAccent    = lipgloss.Color("#A78BFA") // Light purple

	// Title
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	// Subtitle / section headers
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary).
			MarginTop(1)

	// Normal text
	normalStyle = lipgloss.NewStyle().
			Foreground(colorFg)

	// Muted text
	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Success text
	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	// Warning text
	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	// Error text
	errorStyle = lipgloss.NewStyle().
			Foreground(colorDanger)

	// Active/selected item
	activeStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	// Checkbox styles
	checkboxChecked   = lipgloss.NewStyle().Foreground(colorSuccess).SetString("[✓]")
	checkboxUnchecked = lipgloss.NewStyle().Foreground(colorMuted).SetString("[ ]")

	// Cursor
	cursorStyle   = lipgloss.NewStyle().Foreground(colorPrimary).SetString("▸ ")
	noCursorStyle = lipgloss.NewStyle().Foreground(colorMuted).SetString("  ")

	// Box for summary/info panels
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	// Badge styles for file types
	pstBadge  = lipgloss.NewStyle().Background(lipgloss.Color("#7C3AED")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).SetString(" PST ")
	ostBadge  = lipgloss.NewStyle().Background(lipgloss.Color("#6D28D9")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).SetString(" OST ")
	emlBadge  = lipgloss.NewStyle().Background(lipgloss.Color("#0891B2")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).SetString(" EML ")
	msgBadge  = lipgloss.NewStyle().Background(lipgloss.Color("#D97706")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).SetString(" MSG ")
	mboxBadge = lipgloss.NewStyle().Background(lipgloss.Color("#059669")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).SetString("MBOX ")

	// Help bar
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	// Progress label
	progressLabelStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true)

	// Match highlight
	matchStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	// Flagged warning
	flaggedStyle = lipgloss.NewStyle().
			Foreground(colorWarning).
			Bold(true)
)
