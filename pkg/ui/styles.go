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
			Padding(0, 1)

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

	// Clickable buttons
	BtnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ECEFF4")).
			Background(lipgloss.Color("#3B4252")).
			Padding(0, 1)

	BtnHoverStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(ColorPrimary).
			Bold(true).
			Padding(0, 1)

	BtnDangerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#BF616A")).
			Padding(0, 1)

	BadgeNumberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88C0D0")).
			Bold(true)

	BadgeVersionStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Command Badge Palette Key Caps
	BadgeKeyPrimary = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#16191E")).
			Background(ColorPrimary).
			Padding(0, 1)

	BadgeKeySuccess = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#16191E")).
			Background(ColorSuccess).
			Padding(0, 1)

	BadgeKeyWarning = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#16191E")).
			Background(ColorWarning).
			Padding(0, 1)

	BadgeKeySecondary = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorSecondary).
			Padding(0, 1)

	BadgeKeyMuted = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ECEFF4")).
			Background(lipgloss.Color("#4C566A")).
			Padding(0, 1)
)
