package ui

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/jakobhuss/syl/pkg/card"
	"github.com/jakobhuss/syl/pkg/reconciler"
	"github.com/jakobhuss/syl/pkg/watcher"
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

type Model struct {
	store         *card.Store
	reconciler    reconciler.Reconciler
	turnCh        chan *watcher.Turn
	selectedIndex int
	viewMode      ViewMode
	isDemo        bool
	demoStep      int
	width         int
	height        int
	toast         string
	reconciling   bool
	renderer      *glamour.TermRenderer
}

func NewModel(store *card.Store, rec reconciler.Reconciler, turnCh chan *watcher.Turn, isDemo bool) *Model {
	r, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)

	return &Model{
		store:         store,
		reconciler:    rec,
		turnCh:        turnCh,
		selectedIndex: 0,
		viewMode:      ModeSplit,
		isDemo:        isDemo,
		demoStep:      0,
		renderer:      r,
	}
}

func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.isDemo {
		// Auto-advance first step in demo
		cmds = append(cmds, m.advanceDemo())
	} else if m.turnCh != nil {
		cmds = append(cmds, m.listenForTurns())
	}

	return tea.Batch(cmds...)
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

	m.reconciling = true
	return m.applyReconcile(turn)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		wrapWidth := msg.Width / 2
		if wrapWidth < 40 {
			wrapWidth = 40
		}
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(wrapWidth),
		)
		return m, nil

	case TurnReceivedMsg:
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

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}

		case "down", "j":
			cards := m.store.List()
			if m.selectedIndex < len(cards)-1 {
				m.selectedIndex++
			}

		case "enter":
			if m.viewMode == ModeDetail {
				m.viewMode = ModeSplit
			} else {
				m.viewMode = ModeDetail
			}

		case "h":
			if m.viewMode == ModeHistory {
				m.viewMode = ModeSplit
			} else {
				m.viewMode = ModeHistory
			}

		case "p":
			cards := m.store.List()
			if len(cards) > 0 && m.selectedIndex < len(cards) {
				pinned := m.store.TogglePin(cards[m.selectedIndex].ID)
				if pinned {
					m.toast = "📌 Pinned card"
				} else {
					m.toast = "Unpinned card"
				}
			}

		case "y":
			// Yank active card content or primary code block to clipboard
			cards := m.store.List()
			if len(cards) > 0 && m.selectedIndex < len(cards) {
				c := cards[m.selectedIndex]
				snippet := c.Content
				if matches := codeFenceRegex.FindStringSubmatch(c.Content); len(matches) > 1 {
					snippet = strings.TrimSpace(matches[1])
				}
				if err := clipboard.WriteAll(snippet); err == nil {
					m.toast = "✓ Snippet yanked to clipboard!"
				} else {
					m.toast = "Failed to copy to clipboard"
				}
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
					return ClearToastMsg{}
				})
			}

		case "s":
			// Send to chat via cmux if available, else copy to clipboard
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
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
					return ClearToastMsg{}
				})
			}

		case "d", " ":
			if m.isDemo {
				if m.demoStep < 3 {
					return m, m.advanceDemo()
				}
				m.toast = "Demo finished! All 3 scenario turns demonstrated."
			}
		}
	}

	return m, nil
}

func (m *Model) View() string {
	cards := m.store.List()

	// 1. Header
	var statusBadge string
	if m.reconciling {
		statusBadge = StatusReconciling.Render()
	} else if m.isDemo {
		statusBadge = StatusDemo.Render()
	} else {
		statusBadge = StatusLive.Render()
	}

	headerLeft := HeaderStyle.Render("syl") + " " + statusBadge
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

	// 2. Body
	var body string
	if len(cards) == 0 {
		emptyMsg := "No reference cards captured yet.\nStart chatting in your AI assistant or run 'syl demo' to preview."
		body = lipgloss.NewStyle().
			Padding(2, 2).
			Foreground(ColorMuted).
			Render(emptyMsg)
	} else {
		body = m.renderCardsAndDetail(cards)
	}

	// 3. Footer
	var hints string
	if m.isDemo {
		hints = "[Space/d] Next Turn  [Enter] Fullscreen  [y] Yank  [p] Pin  [h] History  [s] Send  [q] Quit"
	} else {
		hints = "[↑/↓ j/k] Navigate  [Enter] Fullscreen  [y] Yank  [p] Pin  [h] History  [s] Send  [q] Quit"
	}
	footer := FooterStyle.Render(hints)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *Model) renderCardsAndDetail(cards []*card.Card) string {
	// Left column: list of cards
	leftLines := make([]string, 0, len(cards))
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
		if len(title) > 28 {
			title = title[:25] + "..."
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
	leftPane := lipgloss.JoinVertical(lipgloss.Left, leftLines...)

	// Right column: Selected Card Detail or History
	var rightPane string
	if m.selectedIndex < len(cards) {
		selected := cards[m.selectedIndex]

		if m.viewMode == ModeHistory {
			// Show revision history
			var historyLines []string
			historyLines = append(historyLines, lipgloss.NewStyle().Bold(true).Render("Revision History: "+selected.Title))
			for _, h := range selected.History {
				entry := fmt.Sprintf("v%d: %s\n(%s)", h.Version, h.Reason, h.UpdatedAt.Format("15:04:05"))
				historyLines = append(historyLines, lipgloss.NewStyle().Foreground(ColorPrimary).Render(entry))
			}
			rightPane = lipgloss.JoinVertical(lipgloss.Left, historyLines...)
		} else {
			// Show rendered markdown content
			rendered, err := m.renderer.Render(selected.Content)
			if err != nil {
				rendered = selected.Content
			}
			rightPane = rendered
		}
	}

	leftStyle := lipgloss.NewStyle().
		Width(36).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(ColorMuted).
		Padding(0, 1)

	rightStyle := lipgloss.NewStyle().
		Padding(0, 1)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyle.Render(leftPane), rightStyle.Render(rightPane))
}
