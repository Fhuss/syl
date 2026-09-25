package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/harness"
	"github.com/Fhuss/syl/pkg/reconciler"
	"github.com/Fhuss/syl/pkg/watcher"
)

var codeFenceRegex = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9_-]+)?\n(.*?)\n```")

type ViewMode int

const (
	ModeSplit ViewMode = iota
	ModeGlance
	ModeDetail
	ModeHistory
)

type TurnReceivedMsg struct {
	Turn *watcher.Turn
}

type MutationsAppliedMsg struct {
	Mutations []reconciler.Mutation
	Turn      *watcher.Turn
}

type TitleSummarizedMsg struct {
	CardID string
	Title  string
	Turn   *watcher.Turn
}

type ReconcileFailedMsg struct {
	Err error
}

type ClearToastMsg struct{}

type Button struct {
	Label  string
	Action string
	X1     int
	X2     int
}

type Model struct {
	store         *card.Store
	reconciler    reconciler.Reconciler
	harness       harness.Harness
	turnCh        chan *watcher.Turn
	activityCh    chan watcher.AgentActivity
	selectedIndex int
	viewMode      ViewMode
	isDemo        bool
	demoStep      int
	width         int
	height        int
	toast         string
	reconciling   bool
	renderer      *glamour.TermRenderer
	viewport      viewport.Model
	buttons       []Button
	sessionLabel  string
	spinner       spinner.Model
	isAgentBusy   bool
	agentTool     string
	agentPrompt   string
	showRaw       bool
	showHelp      bool
}

func NewModel(store *card.Store, rec reconciler.Reconciler, h harness.Harness, turnCh chan *watcher.Turn, activityCh chan watcher.AgentActivity, isDemo bool, sessionLabel string) *Model {
	r, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)

	vp := viewport.New(80, 20)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorWarning)

	m := &Model{
		store:         store,
		reconciler:    rec,
		harness:       h,
		turnCh:        turnCh,
		activityCh:    activityCh,
		selectedIndex: 0,
		viewMode:      ModeSplit,
		isDemo:        isDemo,
		demoStep:      0,
		renderer:      r,
		viewport:      vp,
		buttons:       make([]Button, 0),
		sessionLabel:  sessionLabel,
		spinner:       sp,
	}

	return m
}

func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.spinner.Tick)

	if m.isDemo {
		cmds = append(cmds, m.advanceDemo())
	} else {
		if m.turnCh != nil {
			cmds = append(cmds, m.listenForTurns())
		}
		if m.activityCh != nil {
			cmds = append(cmds, m.listenForActivity())
		}
	}

	return tea.Batch(cmds...)
}

func (m *Model) listenForActivity() tea.Cmd {
	return func() tea.Msg {
		if m.activityCh == nil {
			return nil
		}
		act, ok := <-m.activityCh
		if !ok {
			return nil
		}
		return act
	}
}

func (m *Model) listenForTurns() tea.Cmd {
	return func() tea.Msg {
		turn, ok := <-m.turnCh
		if !ok {
			return nil
		}
		return TurnReceivedMsg{Turn: turn}
	}
}

func (m *Model) applyReconcile(turn *watcher.Turn) tea.Cmd {
	return func() tea.Msg {
		cards := m.store.List()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		muts, err := m.reconciler.Reconcile(ctx, cards, turn)
		if err != nil {
			return ReconcileFailedMsg{Err: err}
		}
		return MutationsAppliedMsg{Mutations: muts, Turn: turn}
	}
}

func (m *Model) advanceDemo() tea.Cmd {
	m.demoStep++
	turn := &watcher.Turn{
		ID:        m.demoStep,
		Timestamp: time.Now(),
	}
	switch m.demoStep {
	case 1:
		turn.User = "Deploy our service to AWS ECS Fargate using the AWS CLI"
		turn.Assistant = "Here are the steps to deploy to ECS Fargate in us-east-1..."
	case 2:
		turn.User = "Step 2 failed with AccessDenied: not authorized to perform ecr:GetAuthorizationToken"
		turn.Assistant = "You need to attach AmazonEC2ContainerRegistryReadOnly to your task execution role first."
	case 3:
		turn.User = "Actually, let's deploy to region eu-central-1 (Frankfurt) instead of us-east-1"
		turn.Assistant = "Updated all commands to use eu-central-1."
	default:
		return func() tea.Msg {
			return ClearToastMsg{}
		}
	}

	m.isAgentBusy = true
	m.agentPrompt = turn.User
	m.agentTool = "Planning..."

	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg {
		return TurnReceivedMsg{Turn: turn}
	})
}

func (m *Model) updateViewportSize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	var wrapWidth int
	var vpHeight int

	if m.viewMode == ModeDetail {
		wrapWidth = m.width - 4
		if wrapWidth < 25 {
			wrapWidth = 25
		}
		vpHeight = m.height - 6 // accounting for focus banner and hints
		if vpHeight < 5 {
			vpHeight = 5
		}
	} else {
		leftColWidth := m.getLeftColWidth()
		wrapWidth = m.width - leftColWidth - 4
		if wrapWidth < 25 {
			wrapWidth = 25
		}
		vpHeight = m.height - 5
		if vpHeight < 5 {
			vpHeight = 5
		}
	}

	m.renderer, _ = glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(wrapWidth),
	)

	m.viewport.Width = wrapWidth
	m.viewport.Height = vpHeight
}

func (m *Model) updateViewportContent() {
	cards := m.store.List()
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		m.viewport.SetContent("No card selected.")
		return
	}

	selected := cards[m.selectedIndex]
	if m.viewMode == ModeHistory {
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Revision History: " + selected.Title + "\n\n"))
		for _, h := range selected.History {
			entry := fmt.Sprintf("• Version %d [%s]:\n  Reason: %s\n\n",
				h.Version, h.UpdatedAt.Format("15:04:05"), h.Reason)
			b.WriteString(lipgloss.NewStyle().Foreground(ColorPrimary).Render(entry))
		}
		m.viewport.SetContent(b.String())
	} else if m.showRaw {
		// Display raw unformatted markdown text - no ANSI colors, no margins, ideal for mouse selection
		m.viewport.SetContent(selected.Content)
	} else {
		rendered, err := m.renderer.Render(selected.Content)
		if err != nil {
			rendered = selected.Content
		}
		m.viewport.SetContent(rendered)
	}
	m.viewport.GotoTop()
}

func (m *Model) actionToggleFocus() {
	if m.viewMode == ModeDetail {
		m.viewMode = ModeSplit
		m.toast = "Split view: turns list shown"
	} else {
		m.viewMode = ModeDetail
		m.toast = "Focus view: turns list hidden (Tab to restore)"
	}
	m.updateViewportSize()
	m.updateViewportContent()
}

func (m *Model) actionToggleRaw() {
	m.showRaw = !m.showRaw
	if m.showRaw {
		m.toast = "Raw text mode (clean for mouse selection)"
	} else {
		m.toast = "Rendered markdown mode"
	}
	m.updateViewportContent()
}

func (m *Model) actionCopy() tea.Cmd {
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		c := cards[m.selectedIndex]
		snippet := c.Content
		allMatches := codeFenceRegex.FindAllStringSubmatch(c.Content, -1)
		if len(allMatches) > 0 {
			var blocks []string
			for _, match := range allMatches {
				if len(match) > 1 {
					code := strings.TrimSpace(match[1])
					if code != "" {
						blocks = append(blocks, code)
					}
				}
			}
			if len(blocks) > 0 {
				snippet = strings.Join(blocks, "\n\n")
			}
		}
		if err := clipboard.WriteAll(snippet); err == nil {
			if len(allMatches) > 0 {
				m.toast = fmt.Sprintf("✓ Copied %d code block(s) to clipboard!", len(allMatches))
			} else {
				m.toast = "✓ Copied card text to clipboard!"
			}
		} else {
			m.toast = "Failed to copy to clipboard"
		}
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}
	return nil
}

func (m *Model) actionCopyAll() tea.Cmd {
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		c := cards[m.selectedIndex]
		if err := clipboard.WriteAll(c.Content); err == nil {
			m.toast = "✓ Copied full card content to clipboard!"
		} else {
			m.toast = "Failed to copy to clipboard"
		}
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}
	return nil
}

func (m *Model) actionPin() tea.Cmd {
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		pinned := m.store.TogglePin(cards[m.selectedIndex].ID)
		if pinned {
			m.toast = "📌 Pinned card"
		} else {
			m.toast = "Unpinned card"
		}
		m.updateViewportContent()
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}
	return nil
}

func (m *Model) actionToggleHistory() {
	if m.viewMode == ModeHistory {
		m.viewMode = ModeSplit
	} else {
		m.viewMode = ModeHistory
	}
	m.updateViewportSize()
	m.updateViewportContent()
}

func (m *Model) actionSend() tea.Cmd {
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		c := cards[m.selectedIndex]
		textToSend := fmt.Sprintf("[Ref: %s (v%d)]\n%s", c.Title, c.Version, c.Content)
		if err := exec.Command("cmux", "send", textToSend).Run(); err == nil {
			m.toast = "✓ Sent card to active chat surface!"
		} else {
			_ = clipboard.WriteAll(textToSend)
			m.toast = "✓ Copied context to clipboard!"
		}
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}
	return nil
}

func (m *Model) actionDrop() tea.Cmd {
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		c := cards[m.selectedIndex]
		m.store.Remove(c.ID)
		m.toast = fmt.Sprintf("✓ Dropped card #%d", c.Number)
		newCards := m.store.List()
		if len(newCards) == 0 {
			m.selectedIndex = 0
		} else if m.selectedIndex >= len(newCards) {
			m.selectedIndex = len(newCards) - 1
		}
		m.updateViewportContent()
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}
	return nil
}

func (m *Model) actionExport() tea.Cmd {
	cards := m.store.List()
	if len(cards) == 0 {
		m.toast = "No reference cards to export"
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}

	content := m.store.ExportMarkdown(m.sessionLabel)
	targetPath := "./syl-reference.md"

	err := os.WriteFile(targetPath, []byte(content), 0644)
	if err != nil {
		home, hErr := os.UserHomeDir()
		if hErr == nil {
			exportDir := filepath.Join(home, ".config", "syl", "exports")
			_ = os.MkdirAll(exportDir, 0755)
			label := m.sessionLabel
			if label == "" {
				label = "session"
			}
			targetPath = filepath.Join(exportDir, fmt.Sprintf("syl-reference-%s.md", label))
			err = os.WriteFile(targetPath, []byte(content), 0644)
		}
	}

	if err == nil {
		_ = clipboard.WriteAll(targetPath)
		m.toast = fmt.Sprintf("✓ Exported %d cards to %s", len(cards), targetPath)
	} else {
		m.toast = fmt.Sprintf("Failed to export: %v", err)
	}

	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return ClearToastMsg{}
	})
}

func (m *Model) renderHelpModal() string {
	boxWidth := 74
	if m.width > 20 && m.width < 78 {
		boxWidth = m.width - 4
	}

	compact := m.height < 28

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render("✨ SYL COMMAND REFERENCE & SHORTCUTS")

	colKey := lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Width(17)
	colDesc := lipgloss.NewStyle().Foreground(lipgloss.Color("#ECEFF4"))
	secHeader := lipgloss.NewStyle().Bold(true).Foreground(ColorWarning)

	var b strings.Builder
	b.WriteString(title + "\n")
	if !compact {
		b.WriteString("\n")
	}

	b.WriteString(secHeader.Render("Navigation & Preview") + "\n")
	b.WriteString(colKey.Render("j / k, ← / →") + colDesc.Render("Switch active reference turn/card") + "\n")
	b.WriteString(colKey.Render("↑ / ↓") + colDesc.Render("Scroll card preview content smoothly") + "\n")
	b.WriteString(colKey.Render("PgUp / PgDn") + colDesc.Render("Scroll preview half-page up / down") + "\n")
	b.WriteString(colKey.Render("g / G") + colDesc.Render("Jump to top / bottom of card preview") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	b.WriteString(secHeader.Render("View Modes") + "\n")
	b.WriteString(colKey.Render("Tab / Enter") + colDesc.Render("Toggle Focus View (hide/show turns column)") + "\n")
	b.WriteString(colKey.Render("r") + colDesc.Render("Toggle Raw text mode (for clean mouse select)") + "\n")
	b.WriteString(colKey.Render("h") + colDesc.Render("Toggle card version revision history") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	b.WriteString(secHeader.Render("Clipboard & Actions") + "\n")
	b.WriteString(colKey.Render("c") + colDesc.Render("Copy extracted code block(s) to clipboard") + "\n")
	b.WriteString(colKey.Render("C / y") + colDesc.Render("Copy full card markdown content") + "\n")
	b.WriteString(colKey.Render("p") + colDesc.Render("Pin / unpin card (protect from pruning)") + "\n")
	b.WriteString(colKey.Render("x / d") + colDesc.Render("Drop / delete card from reference pane") + "\n")
	b.WriteString(colKey.Render("e") + colDesc.Render("Export session reference cards to markdown") + "\n")
	b.WriteString(colKey.Render("s") + colDesc.Render("Send card context to chat surface (cmux)") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	b.WriteString(secHeader.Render("Mouse & General") + "\n")
	b.WriteString(colKey.Render("Click / Wheel") + colDesc.Render("Click action pills or turns; wheel to scroll") + "\n")
	b.WriteString(colKey.Render("? / F1") + colDesc.Render("Toggle this help reference glossary") + "\n")
	b.WriteString(colKey.Render("q / Esc") + colDesc.Render("Close help modal / Quit syl") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	footer := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("Press [?], [Esc], [Enter], or click anywhere to close")
	b.WriteString(footer)

	paddingY := 1
	if compact {
		paddingY = 0
	}

	boxStyle := lipgloss.NewStyle().
		Width(boxWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(lipgloss.Color("#1E222A")).
		Padding(paddingY, 2)

	modalBox := boxStyle.Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modalBox)
}

func (m *Model) summarizeCardTitle(cardID string, turn *watcher.Turn) tea.Cmd {
	return func() tea.Msg {
		if m.harness == nil || turn == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		title, err := m.harness.Summarize(ctx, turn.User, turn.Assistant)
		if err != nil || title == "" {
			return nil
		}
		return TitleSummarizedMsg{
			CardID: cardID,
			Title:  title,
			Turn:   turn,
		}
	}
}

func (m *Model) getLeftColWidth() int {
	if m.width <= 70 {
		return 32
	}
	if m.width <= 100 {
		return 42
	}
	if m.width <= 140 {
		return 48
	}
	return 54
}

func (m *Model) handleButtonAction(action string) tea.Cmd {
	switch action {
	case "toggle_focus":
		m.actionToggleFocus()
	case "toggle_raw":
		m.actionToggleRaw()
	case "copy":
		return m.actionCopy()
	case "copy_all":
		return m.actionCopyAll()
	case "pin":
		return m.actionPin()
	case "drop":
		return m.actionDrop()
	case "export":
		return m.actionExport()
	case "history":
		m.actionToggleHistory()
	case "send":
		return m.actionSend()
	case "help":
		m.showHelp = !m.showHelp
	case "demo_next":
		if m.isDemo && m.demoStep < 3 {
			return m.advanceDemo()
		}
		m.toast = "Demo finished! All turns demonstrated."
	case "quit":
		return tea.Quit
	}
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSize()
		m.updateViewportContent()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case watcher.AgentActivity:
		m.isAgentBusy = msg.IsBusy
		m.agentTool = msg.Tool
		m.agentPrompt = msg.Prompt
		return m, m.listenForActivity()

	case TurnReceivedMsg:
		m.isAgentBusy = false
		m.reconciling = true
		return m, tea.Batch(m.applyReconcile(msg.Turn), m.listenForTurns())

	case MutationsAppliedMsg:
		m.reconciling = false
		for _, mut := range msg.Mutations {
			switch mut.Op {
			case reconciler.OpAdd:
				// 1. Check title cache so restarted syl retains all past AI titles
				if msg.Turn != nil {
					if cached, ok := harness.GetTitleCache().Get(m.sessionLabel, msg.Turn.ID); ok {
						mut.Title = cached
					} else if cached, ok := harness.GetTitleCache().GetByPrompt(msg.Turn.User); ok {
						mut.Title = cached
					}
				}

				m.store.Add(&card.Card{
					ID:      mut.CardID,
					Type:    mut.Type,
					Title:   mut.Title,
					Content: mut.Content,
				})
				m.toast = fmt.Sprintf("Added: %s", mut.Title)

				// 2. Queue AI summarization if not in cache (live turn or un-cached catch-up)
				if msg.Turn != nil && m.harness != nil {
					hasCache := false
					if _, ok := harness.GetTitleCache().Get(m.sessionLabel, msg.Turn.ID); ok {
						hasCache = true
					} else if _, ok := harness.GetTitleCache().GetByPrompt(msg.Turn.User); ok {
						hasCache = true
					}

					if !msg.Turn.IsCatchUp || !hasCache {
						cmds = append(cmds, m.summarizeCardTitle(mut.CardID, msg.Turn))
					}
				}
			case reconciler.OpUpdate:
				m.store.Update(mut.CardID, mut.Content, mut.Reason)
				m.toast = fmt.Sprintf("Updated: %s", mut.Reason)
				if msg.Turn != nil && !msg.Turn.IsCatchUp && m.harness != nil {
					cmds = append(cmds, m.summarizeCardTitle(mut.CardID, msg.Turn))
				}
			case reconciler.OpInvalidate:
				m.store.Invalidate(mut.CardID, mut.Reason)
				m.toast = fmt.Sprintf("Invalidated: %s", mut.Reason)
			}
		}
		m.updateViewportContent()
		cmds = append(cmds, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		}))
		return m, tea.Batch(cmds...)

	case TitleSummarizedMsg:
		if m.store.UpdateTitle(msg.CardID, msg.Title) {
			m.updateViewportContent()
		}
		if msg.Turn != nil {
			harness.GetTitleCache().Set(m.sessionLabel, msg.Turn.ID, msg.Title)
			harness.GetTitleCache().SetByPrompt(msg.Turn.User, msg.Title)
		}
		return m, nil

	case ReconcileFailedMsg:
		m.reconciling = false
		m.toast = fmt.Sprintf("Error: %v", msg.Err)
		return m, nil

	case ClearToastMsg:
		m.toast = ""
		return m, nil

	// Mouse Event Handling (Clicks, Scroll wheel, Drag)
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}

			if msg.Button == tea.MouseButtonLeft {
				// 1. Check Action Bar clicks (Row 1)
				if msg.Y == 1 {
					for _, btn := range m.buttons {
						if msg.X >= btn.X1 && msg.X <= btn.X2 {
							cmd := m.handleButtonAction(btn.Action)
							return m, cmd
						}
					}
				}

				// 2. In Split mode: Check Card List clicks (Row >= 2, X <= leftColWidth)
				if m.viewMode != ModeDetail {
					leftColWidth := m.getLeftColWidth()
					if msg.Y >= 2 && msg.X <= leftColWidth {
						cardIdx := msg.Y - 2
						cards := m.store.List()
						if cardIdx >= 0 && cardIdx < len(cards) {
							if m.selectedIndex == cardIdx {
								m.actionToggleFocus()
							} else {
								m.selectedIndex = cardIdx
								m.updateViewportContent()
							}
						}
					}
				}
			} else if msg.Button == tea.MouseButtonWheelUp {
				if m.viewMode == ModeDetail {
					m.viewport.LineUp(3)
				} else {
					leftColWidth := m.getLeftColWidth()
					if msg.X <= leftColWidth {
						if m.selectedIndex > 0 {
							m.selectedIndex--
							m.updateViewportContent()
						}
					} else {
						m.viewport.LineUp(3)
					}
				}
			} else if msg.Button == tea.MouseButtonWheelDown {
				if m.viewMode == ModeDetail {
					m.viewport.LineDown(3)
				} else {
					leftColWidth := m.getLeftColWidth()
					if msg.X <= leftColWidth {
						cards := m.store.List()
						if m.selectedIndex < len(cards)-1 {
							m.selectedIndex++
							m.updateViewportContent()
						}
					} else {
						m.viewport.LineDown(3)
					}
				}
			}
		}

	case tea.KeyMsg:
		if m.showHelp {
			switch msg.String() {
			case "?", "f1", "esc", "q", "enter", " ", "tab":
				m.showHelp = false
				return m, nil
			default:
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "?", "f1":
			m.showHelp = true
			return m, nil

		case "e":
			return m, m.actionExport()

		case "esc":
			if m.viewMode == ModeDetail || m.viewMode == ModeHistory {
				m.viewMode = ModeSplit
				m.updateViewportSize()
				m.updateViewportContent()
				return m, nil
			}

		// Scrolling preview viewport content
		case "up":
			m.viewport.LineUp(2)
			return m, nil

		case "down":
			m.viewport.LineDown(2)
			return m, nil

		case "pgup", "ctrl+u":
			m.viewport.HalfViewUp()
			return m, nil

		case "pgdown", "ctrl+d":
			m.viewport.HalfViewDown()
			return m, nil

		case "home", "g":
			m.viewport.GotoTop()
			return m, nil

		case "end", "G":
			m.viewport.GotoBottom()
			return m, nil

		// Navigating reference turns / cards
		case "k", "left", "[":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m.updateViewportContent()
			}

		case "j", "right", "]":
			cards := m.store.List()
			if m.selectedIndex < len(cards)-1 {
				m.selectedIndex++
				m.updateViewportContent()
			}

		case "tab", "enter", "f", "t":
			m.actionToggleFocus()

		case "r":
			m.actionToggleRaw()

		case "c":
			return m, m.actionCopy()

		case "C", "y":
			return m, m.actionCopyAll()

		case "h":
			m.actionToggleHistory()

		case "p":
			return m, m.actionPin()

		case "s":
			return m, m.actionSend()

		case "x", "backspace", "delete":
			return m, m.actionDrop()

		case "d":
			if m.isDemo {
				if m.demoStep < 3 {
					return m, m.advanceDemo()
				}
				m.toast = "Demo finished! All 3 scenario turns demonstrated."
			} else {
				return m, m.actionDrop()
			}

		case " ":
			if m.isDemo {
				if m.demoStep < 3 {
					return m, m.advanceDemo()
				}
				m.toast = "Demo finished! All 3 scenario turns demonstrated."
			}
		}
	}

	// Update viewport with key/wheel events
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) renderActionBar() string {
	var b strings.Builder
	m.buttons = make([]Button, 0)
	currX := 1

	addButton := func(label, action string, style lipgloss.Style) {
		rendered := style.Render(label)
		width := lipgloss.Width(rendered)
		m.buttons = append(m.buttons, Button{
			Label:  label,
			Action: action,
			X1:     currX,
			X2:     currX + width,
		})
		b.WriteString(rendered)
		b.WriteString(" ")
		currX += width + 1
	}

	// 1. Focus / Split toggle
	if m.viewMode == ModeDetail {
		addButton("☰ Show Turns (Tab)", "toggle_focus", BtnHoverStyle)
	} else {
		addButton("👁 Hide Turns (Tab)", "toggle_focus", BtnStyle)
	}

	// 2. Copy Code
	addButton("📋 Copy (c)", "copy", BtnStyle)

	// 3. Copy All (when width allows)
	if m.width >= 85 {
		addButton("📄 All (C)", "copy_all", BtnStyle)
	}

	// 4. Raw text toggle
	if m.showRaw {
		addButton("🎨 Rendered (r)", "toggle_raw", BtnHoverStyle)
	} else {
		addButton("📝 Raw (r)", "toggle_raw", BtnStyle)
	}

	// 5. Pin & Drop
	addButton("📌 Pin (p)", "pin", BtnStyle)
	addButton("🗑 Drop (x)", "drop", BtnStyle)

	// 6. Export (when width allows)
	if m.width >= 90 {
		addButton("💾 Export (e)", "export", BtnStyle)
	}

	// 7. History (when width allows)
	if m.width >= 105 {
		if m.viewMode == ModeHistory {
			addButton("📜 History (h)", "history", BtnHoverStyle)
		} else {
			addButton("📜 History (h)", "history", BtnStyle)
		}
	}

	// 8. Send (when width allows)
	if m.width >= 120 {
		addButton("🚀 Send (s)", "send", BtnStyle)
	}

	// 9. Help modal
	if m.showHelp {
		addButton("❓ Help (?)", "help", BtnHoverStyle)
	} else {
		addButton("❓ Help (?)", "help", BtnStyle)
	}

	// 10. Demo next turn
	if m.isDemo {
		addButton("⏩ Next (Space)", "demo_next", BtnHoverStyle)
	}

	// 11. Quit
	addButton("✕ Quit (q)", "quit", BtnDangerStyle)

	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}

func (m *Model) View() string {
	if m.showHelp {
		return m.renderHelpModal()
	}

	cards := m.store.List()

	// 1. Header
	var statusBadge string
	if m.isAgentBusy {
		toolName := m.agentTool
		if toolName == "" {
			toolName = "BUSY"
		}
		statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render(m.spinner.View() + " " + strings.ToUpper(toolName))
	} else if m.reconciling {
		statusBadge = StatusReconciling.Render()
	} else if m.isDemo {
		statusBadge = StatusDemo.Render()
	} else {
		statusBadge = StatusLive.Render()
	}

	sessionBadge := ""
	if m.sessionLabel != "" {
		sessionBadge = lipgloss.NewStyle().Foreground(ColorMuted).Render("["+m.sessionLabel+"]") + " "
	}

	headerLeft := HeaderStyle.Render("syl") + " " + sessionBadge + statusBadge
	var headerRight string
	if m.toast != "" {
		headerRight = ToastStyle.Render(m.toast)
	} else {
		headerRight = FooterStyle.Render(fmt.Sprintf("%d cards", len(cards)))
	}

	gap := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
	if gap < 2 {
		gap = 2
	}
	header := headerLeft + strings.Repeat(" ", gap) + headerRight

	// 2. Action Bar (Clickable pills)
	actionBar := m.renderActionBar()

	// 3. Body
	var body string
	if len(cards) == 0 {
		var emptyMsg string
		if m.isAgentBusy {
			emptyMsg = fmt.Sprintf("%s Agent is processing your request in AGY...\n\nAction: %s\nPrompt: %q\n\nReference cards will appear automatically when the turn completes.",
				m.spinner.View(), m.agentTool, m.agentPrompt)
		} else {
			emptyMsg = "No reference cards captured yet.\nStart chatting in your AI assistant or click [Next Turn] to preview."
		}
		body = lipgloss.NewStyle().
			Padding(2, 2).
			Foreground(ColorMuted).
			Render(emptyMsg)
	} else if m.viewMode == ModeDetail {
		body = m.renderDetailFocus(cards)
	} else {
		body = m.renderCardsAndDetail(cards)
	}

	// 4. Footer
	var hintText string
	if m.width < 90 {
		if m.viewMode == ModeDetail {
			hintText = "💡 Tab: Show • ↑/↓: Scroll • j/k: Turn • c: Copy • ?: Help • e: Export"
		} else {
			hintText = "💡 Tab: Hide • ↑/↓: Scroll • j/k: Turn • c: Copy • ?: Help • e: Export"
		}
	} else {
		if m.viewMode == ModeDetail {
			hintText = "💡 ↑/↓: Scroll preview • j/k (or ←/→): Switch turn • Tab: Show turns • c: Copy • r: Raw • e: Export • ?: Help Glossary"
		} else {
			hintText = "💡 ↑/↓: Scroll preview • j/k (or ←/→): Switch turn • Tab: Hide turns • c: Copy • r: Raw • e: Export • ?: Help Glossary"
		}
	}
	hints := FooterStyle.Render(hintText)

	return lipgloss.JoinVertical(lipgloss.Left, header, actionBar, body, hints)
}

func (m *Model) renderDetailFocus(cards []*card.Card) string {
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		return ""
	}
	selected := cards[m.selectedIndex]

	var badge lipgloss.Style
	switch selected.Type {
	case card.TypeProcedure:
		badge = BadgeProc
	case card.TypeCommand:
		badge = BadgeCmd
	case card.TypeDecision:
		badge = BadgeDec
	default:
		badge = BadgeInfo
	}
	if selected.Status == card.StatusInvalidated {
		badge = BadgeInvalid
	}

	numStr := BadgeNumberStyle.Render(fmt.Sprintf("#%d", selected.Number))
	verStr := BadgeVersionStyle.Render(fmt.Sprintf("v%d", selected.Version))
	pinIcon := ""
	if selected.Pinned {
		pinIcon = "📌 "
	}

	modeIndicator := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render("FOCUS VIEW")

	turnNavHint := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(fmt.Sprintf("[%d/%d]  [↑/↓: scroll • j/k: switch turn • Tab: show turns]", m.selectedIndex+1, len(cards)))

	titleStr := lipgloss.NewStyle().Bold(true).Render(selected.Title)

	bannerLeft := fmt.Sprintf("%s  %s %s %s%s %s", modeIndicator, numStr, badge.Render(), pinIcon, titleStr, verStr)
	gap := m.width - lipgloss.Width(bannerLeft) - lipgloss.Width(turnNavHint) - 4
	if gap < 2 {
		gap = 2
	}
	bannerLine := bannerLeft + strings.Repeat(" ", gap) + turnNavHint

	banner := lipgloss.NewStyle().
		Background(ColorHighlight).
		Width(m.width - 2).
		Padding(0, 1).
		Render(bannerLine)

	vpContent := lipgloss.NewStyle().
		Padding(0, 1).
		Render(m.viewport.View())

	return lipgloss.JoinVertical(lipgloss.Left, banner, vpContent)
}

func (m *Model) renderCardsAndDetail(cards []*card.Card) string {
	leftColWidth := m.getLeftColWidth()
	rowWidth := leftColWidth - 1 // Leave 1 col for right vertical border
	if rowWidth < 20 {
		rowWidth = 20
	}
	innerWidth := rowWidth - 2 // CardItemStyle has Padding(0, 1)
	if innerWidth < 16 {
		innerWidth = 16
	}

	leftLines := make([]string, 0, len(cards)+1)
	for i, c := range cards {
		var badge lipgloss.Style
		switch c.Type {
		case card.TypeProcedure:
			badge = BadgeProc
		case card.TypeCommand:
			badge = BadgeCmd
		case card.TypeDecision:
			badge = BadgeDec
		default:
			badge = BadgeInfo
		}

		if c.Status == card.StatusInvalidated {
			badge = BadgeInvalid
		}

		prefix := "  "
		if i == m.selectedIndex {
			prefix = "▸ "
		}

		numStr := BadgeNumberStyle.Render(fmt.Sprintf("#%-2d", c.Number))
		badgeStr := badge.Render()
		pinIcon := ""
		if c.Pinned {
			pinIcon = "📌 "
		}
		verStr := BadgeVersionStyle.Render(fmt.Sprintf("v%d", c.Version))

		// Calculate available width for the title so the row NEVER wraps
		fixedWidth := lipgloss.Width(prefix) + lipgloss.Width(numStr) + 1 + lipgloss.Width(badgeStr) + 1 + lipgloss.Width(pinIcon) + 1 + lipgloss.Width(verStr)
		availTitle := innerWidth - fixedWidth
		if availTitle < 6 {
			availTitle = 6
		}

		title := c.Title
		runes := []rune(title)
		if len(runes) > availTitle {
			if availTitle > 3 {
				title = string(runes[:availTitle-3]) + "..."
			} else {
				title = string(runes[:availTitle])
			}
		}

		leftPart := fmt.Sprintf("%s%s %s %s%s", prefix, numStr, badgeStr, pinIcon, title)
		gap := innerWidth - lipgloss.Width(leftPart) - lipgloss.Width(verStr)
		if gap < 1 {
			gap = 1
		}
		lineContent := leftPart + strings.Repeat(" ", gap) + verStr

		var renderedRow string
		if i == m.selectedIndex {
			renderedRow = CardSelectedStyle.Width(rowWidth).Render(lineContent)
		} else {
			renderedRow = CardItemStyle.Width(rowWidth).Render(lineContent)
		}
		leftLines = append(leftLines, renderedRow)
	}

	// Add dynamic loading state card when agent is busy
	if m.isAgentBusy {
		execBadge := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#000000")).
			Background(ColorWarning).
			Padding(0, 1).
			SetString("EXEC")

		pendingTitle := m.agentPrompt
		if pendingTitle == "" {
			pendingTitle = "Thinking..."
		}

		prefix := "  "
		fixedWidth := lipgloss.Width(prefix) + lipgloss.Width(execBadge.Render()) + 1 + lipgloss.Width(m.spinner.View()) + 1
		availTitle := innerWidth - fixedWidth
		if availTitle < 6 {
			availTitle = 6
		}

		pRunes := []rune(pendingTitle)
		if len(pRunes) > availTitle {
			if availTitle > 3 {
				pendingTitle = string(pRunes[:availTitle-3]) + "..."
			} else {
				pendingTitle = string(pRunes[:availTitle])
			}
		}

		pendingLine := fmt.Sprintf("%s%s %s %s", prefix, execBadge.Render(), m.spinner.View(), pendingTitle)
		renderedPending := lipgloss.NewStyle().
			Foreground(ColorWarning).
			Width(rowWidth).
			Padding(0, 1).
			Render(pendingLine)
		leftLines = append(leftLines, renderedPending)
	}

	leftPane := lipgloss.JoinVertical(lipgloss.Left, leftLines...)

	leftStyle := lipgloss.NewStyle().
		Width(leftColWidth).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(ColorMuted)

	if m.viewport.Height > 0 {
		leftStyle = leftStyle.Height(m.viewport.Height)
	}

	rightStyle := lipgloss.NewStyle().
		Padding(0, 1)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyle.Render(leftPane), rightStyle.Render(m.viewport.View()))
}
