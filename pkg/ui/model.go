package ui

import (
	"context"
	"fmt"
	"os/exec"
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
}

func NewModel(store *card.Store, rec reconciler.Reconciler, turnCh chan *watcher.Turn, activityCh chan watcher.AgentActivity, isDemo bool, sessionLabel string) *Model {
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
		return MutationsAppliedMsg{Mutations: muts}
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
	} else {
		rendered, err := m.renderer.Render(selected.Content)
		if err != nil {
			rendered = selected.Content
		}
		m.viewport.SetContent(rendered)
	}
	m.viewport.GotoTop()
}

func (m *Model) actionCopy() tea.Cmd {
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		c := cards[m.selectedIndex]
		snippet := c.Content
		if matches := codeFenceRegex.FindStringSubmatch(c.Content); len(matches) > 1 {
			snippet = strings.TrimSpace(matches[1])
		}
		if err := clipboard.WriteAll(snippet); err == nil {
			m.toast = "✓ Copied to clipboard!"
		} else {
			m.toast = "Failed to copy to clipboard"
		}
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}
	return nil
}

func (m *Model) actionPin() {
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		pinned := m.store.TogglePin(cards[m.selectedIndex].ID)
		if pinned {
			m.toast = "📌 Pinned card"
		} else {
			m.toast = "Unpinned card"
		}
		m.updateViewportContent()
	}
}

func (m *Model) actionToggleHistory() {
	if m.viewMode == ModeHistory {
		m.viewMode = ModeSplit
	} else {
		m.viewMode = ModeHistory
	}
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

func (m *Model) handleButtonAction(action string) tea.Cmd {
	switch action {
	case "copy":
		return m.actionCopy()
	case "pin":
		m.actionPin()
	case "history":
		m.actionToggleHistory()
	case "send":
		return m.actionSend()
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

		wrapWidth := msg.Width - 40
		if wrapWidth < 40 {
			wrapWidth = 40
		}
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(wrapWidth),
		)

		vpHeight := msg.Height - 4
		if vpHeight < 5 {
			vpHeight = 5
		}
		m.viewport.Width = wrapWidth
		m.viewport.Height = vpHeight
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
				m.store.Add(&card.Card{
					ID:      mut.CardID,
					Type:    mut.Type,
					Title:   mut.Title,
					Content: mut.Content,
				})
				m.toast = fmt.Sprintf("Added: %s", mut.Title)
			case reconciler.OpUpdate:
				m.store.Update(mut.CardID, mut.Content, mut.Reason)
				m.toast = fmt.Sprintf("Updated: %s", mut.Reason)
			case reconciler.OpInvalidate:
				m.store.Invalidate(mut.CardID, mut.Reason)
				m.toast = fmt.Sprintf("Invalidated: %s", mut.Reason)
			}
		}
		m.updateViewportContent()
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})

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

				// 2. Check Card List clicks (Row >= 2, X <= 36)
				if msg.Y >= 2 && msg.X <= 36 {
					cardIdx := msg.Y - 2
					cards := m.store.List()
					if cardIdx >= 0 && cardIdx < len(cards) {
						if m.selectedIndex == cardIdx {
							// Toggle view mode on repeat click
							if m.viewMode == ModeDetail {
								m.viewMode = ModeSplit
							} else {
								m.viewMode = ModeDetail
							}
						} else {
							m.selectedIndex = cardIdx
						}
						m.updateViewportContent()
					}
				}
			} else if msg.Button == tea.MouseButtonWheelUp {
				if msg.X <= 36 {
					if m.selectedIndex > 0 {
						m.selectedIndex--
						m.updateViewportContent()
					}
				} else {
					m.viewport.LineUp(3)
				}
			} else if msg.Button == tea.MouseButtonWheelDown {
				if msg.X <= 36 {
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

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m.updateViewportContent()
			}

		case "down", "j":
			cards := m.store.List()
			if m.selectedIndex < len(cards)-1 {
				m.selectedIndex++
				m.updateViewportContent()
			}

		case "enter":
			if m.viewMode == ModeDetail {
				m.viewMode = ModeSplit
			} else {
				m.viewMode = ModeDetail
			}

		case "h":
			m.actionToggleHistory()

		case "p":
			m.actionPin()

		case "c", "y":
			return m, m.actionCopy()

		case "s":
			return m, m.actionSend()

		case "d", " ":
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

	addButton("📋 Copy (c)", "copy", BtnStyle)
	addButton("📌 Pin (p)", "pin", BtnStyle)
	if m.viewMode == ModeHistory {
		addButton("📜 History (h)", "history", BtnHoverStyle)
	} else {
		addButton("📜 History (h)", "history", BtnStyle)
	}
	addButton("🚀 Send (s)", "send", BtnStyle)
	if m.isDemo {
		addButton("⏩ Next Turn (Space)", "demo_next", BtnHoverStyle)
	}
	addButton("✕ Quit (q)", "quit", BtnDangerStyle)

	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}

func (m *Model) View() string {
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
	} else {
		body = m.renderCardsAndDetail(cards)
	}

	// 4. Footer
	hints := FooterStyle.Render("💡 Click any card or action button • Scroll with mouse wheel or trackpad")

	return lipgloss.JoinVertical(lipgloss.Left, header, actionBar, body, hints)
}

func (m *Model) renderCardsAndDetail(cards []*card.Card) string {
	// Left column: list of cards
	leftLines := make([]string, 0, len(cards)+1)
	for i, c := range cards {
		var badge string
		switch c.Type {
		case card.TypeProcedure:
			badge = BadgeProc.Render()
		case card.TypeCommand:
			badge = BadgeCmd.Render()
		case card.TypeDecision:
			badge = BadgeDec.Render()
		default:
			badge = BadgeInfo.Render()
		}

		if c.Status == card.StatusInvalidated {
			badge = BadgeInvalid.Render()
		}

		pinIcon := ""
		if c.Pinned {
			pinIcon = "📌 "
		}

		title := c.Title
		if len(title) > 24 {
			title = title[:21] + "..."
		}

		ver := fmt.Sprintf("v%d", c.Version)
		line := fmt.Sprintf("%s %s%s (%s)", badge, pinIcon, title, ver)

		if i == m.selectedIndex {
			line = CardSelectedStyle.Render("▸ " + line)
		} else {
			line = CardItemStyle.Render("  " + line)
		}
		leftLines = append(leftLines, line)
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
		if len(pendingTitle) > 18 {
			pendingTitle = pendingTitle[:15] + "..."
		}
		if pendingTitle == "" {
			pendingTitle = "Thinking..."
		}

		pendingLine := lipgloss.NewStyle().
			Foreground(ColorWarning).
			Render(fmt.Sprintf("  %s %s %s", execBadge, m.spinner.View(), pendingTitle))
		leftLines = append(leftLines, pendingLine)
	}
	leftPane := lipgloss.JoinVertical(lipgloss.Left, leftLines...)

	leftStyle := lipgloss.NewStyle().
		Width(36).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(ColorMuted).
		Padding(0, 1)

	rightStyle := lipgloss.NewStyle().
		Padding(0, 1)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyle.Render(leftPane), rightStyle.Render(m.viewport.View()))
}
