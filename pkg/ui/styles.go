package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Brand colors
	ColorPrimary   = lipgloss.Color("#00D7D7") // Cyan/Spren
	ColorSecondary = lipgloss.Color("#7D56F4") // Deep Purple
	ColorSuccess   = lipgloss.Color("#04B575") // Green
	ColorWarning   = lipgloss.Color("#FFAE00") // Amber
	ColorDanger    = lipgloss.Color("#FF4F64") // Red
	ColorMuted     = lipgloss.Color("#626262") // Dark Gray
	ColorHighlight = lipgloss.Color("#2E3440") // Dark Nordic BG

	// Base styles
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Padding(0, 1)

	StatusLive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSuccess).
			SetString("● LIVE")

	StatusDemo = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWarning).
			SetString("● DEMO")

	StatusReconciling = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorWarning).
				SetString("⟳ RECONCILING")

	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1)

	// Card item styles
	CardItemStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, false, false)

	CardSelectedStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Background(ColorHighlight).
				Bold(true)

	// Badge styles
	BadgeProc = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorSecondary).
			Padding(0, 1).
			SetString("PROC")

	BadgeCmd = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#000000")).
			Background(ColorSuccess).
			Padding(0, 1).
			SetString("CMD")

	BadgeDec = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#000000")).
			Background(ColorWarning).
			Padding(0, 1).
			SetString("DEC")

	BadgeInfo = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorMuted).
			Padding(0, 1).
			SetString("INFO")

	BadgeInvalid = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorDanger).
			Padding(0, 1).
			SetString("INVALID")

	ToastStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSuccess).
			Padding(0, 1)
)
