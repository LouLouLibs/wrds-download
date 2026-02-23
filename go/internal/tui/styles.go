package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("#7C3AED") // purple
	colorSecondary = lipgloss.Color("#06B6D4") // cyan
	colorMuted     = lipgloss.Color("#6B7280")
	colorSuccess   = lipgloss.Color("#10B981")
	colorError     = lipgloss.Color("#EF4444")
	colorFocus     = lipgloss.Color("#F59E0B") // amber border when focused

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	stylePanelFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFocus)

	stylePanelBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorMuted)

	stylePanelHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSecondary).
				Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	styleSuccess = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError   = lipgloss.NewStyle().Foreground(colorError)

	styleCellHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	styleCellNormal = lipgloss.NewStyle().Padding(0, 1)

	styleRowCount = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)
)
