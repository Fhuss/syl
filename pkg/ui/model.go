package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/harness"
	"github.com/Fhuss/syl/pkg/multiplexer"
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
	runner        *harness.Runner
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
	showRaw           bool
	showHelp          bool
	sessionFilePath   string
	lastTurnProcessed int
	cardListOffset    int

	// Interactive Actionable Block / Step Picker
	showBlockPicker  bool
	activeBlocks     []card.ActionBlock
	selectedBlockIdx int

	// Quick Search Across Cards
	searchMode     bool
	searchQuery    string
	searchSavedIdx int

	// In-TUI Session Switcher
	showSessionSwitcher bool
	sessionList         []watcher.SessionInfo
	selectedSessionIdx  int
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

	var lastTurn int
	var sessionFilePath string
	if !isDemo && sessionLabel != "" {
		if path, err := card.SessionFilePath(sessionLabel); err == nil {
			sessionFilePath = path
			if len(store.List()) == 0 {
				if snap, err := store.LoadSnapshot(path); err == nil {
					lastTurn = snap.LastTurnProcessed
				}
			}
		}
	}

	var rnr *harness.Runner
	if h != nil {
		rnr = harness.NewRunner(h, 1)
	}

	m := &Model{
		store:             store,
		reconciler:        rec,
		harness:           h,
		runner:            rnr,
		turnCh:            turnCh,
		activityCh:        activityCh,
		selectedIndex:     0,
		viewMode:          ModeSplit,
		isDemo:            isDemo,
		demoStep:          0,
		renderer:          r,
		viewport:          vp,
		buttons:           make([]Button, 0),
		sessionLabel:      sessionLabel,
		sessionFilePath:   sessionFilePath,
		lastTurnProcessed: lastTurn,
		spinner:           sp,
	}

	if len(store.List()) > 0 {
		m.updateViewportContent()
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
		selectedID := cards[m.selectedIndex].ID
		pinned := m.store.TogglePin(selectedID)
		m.saveSessionState()
		if pinned {
			m.toast = "📌 Pinned card"
		} else {
			m.toast = "Unpinned card"
		}
		newCards := m.store.List()
		for idx, c := range newCards {
			if c.ID == selectedID {
				m.selectedIndex = idx
				break
			}
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
		textToSend := fmt.Sprintf("[Ref: %s (v%d)] ", c.Title, c.Version)
		toast, err := multiplexer.Send(textToSend, "")
		if err != nil {
			m.toast = fmt.Sprintf("Error: %v", err)
		} else {
			m.toast = toast
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
		m.saveSessionState()
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

func (m *Model) saveSessionState() {
	if m.isDemo || m.sessionFilePath == "" {
		return
	}
	_ = m.store.SaveSnapshot(m.sessionFilePath, m.sessionLabel, m.lastTurnProcessed)
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

func openURL(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	return cmd.Start()
}

func (m *Model) actionToggleBlockPicker() {
	if m.showBlockPicker {
		m.showBlockPicker = false
		return
	}
	cards := m.store.List()
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		m.toast = "No card selected"
		return
	}
	blocks := card.ExtractActionBlocks(cards[m.selectedIndex].Content)
	if len(blocks) == 0 {
		m.toast = "No code blocks, steps, or URLs found in this card"
		return
	}
	m.activeBlocks = blocks
	m.selectedBlockIdx = 0
	m.showBlockPicker = true
}

func (m *Model) actionCopyBlock(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.activeBlocks) {
		return nil
	}
	blk := m.activeBlocks[idx]
	if err := clipboard.WriteAll(blk.Content); err == nil {
		m.toast = fmt.Sprintf("✓ Copied [%s] to clipboard!", blk.Badge)
	} else {
		m.toast = "Failed to copy block"
	}
	m.showBlockPicker = false
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ClearToastMsg{}
	})
}

func (m *Model) actionSendBlock(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.activeBlocks) {
		return nil
	}
	blk := m.activeBlocks[idx]
	res, err := multiplexer.Send(blk.Content, "")
	if err != nil {
		m.toast = fmt.Sprintf("Send error: %v", err)
	} else {
		m.toast = res
	}
	m.showBlockPicker = false
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ClearToastMsg{}
	})
}

func (m *Model) actionOpenBlockURL(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.activeBlocks) {
		return nil
	}
	blk := m.activeBlocks[idx]
	if blk.Type != card.BlockURL {
		return nil
	}
	if err := openURL(blk.Content); err == nil {
		m.toast = fmt.Sprintf("✓ Opened %s in browser!", blk.Preview)
	} else {
		m.toast = fmt.Sprintf("Failed to open URL: %v", err)
	}
	m.showBlockPicker = false
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ClearToastMsg{}
	})
}

func (m *Model) actionOpenURL() tea.Cmd {
	cards := m.store.List()
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		m.toast = "No card selected"
		return nil
	}
	blocks := card.ExtractActionBlocks(cards[m.selectedIndex].Content)
	var urlBlocks []card.ActionBlock
	for _, b := range blocks {
		if b.Type == card.BlockURL {
			urlBlocks = append(urlBlocks, b)
		}
	}
	if len(urlBlocks) == 0 {
		m.toast = "No URLs found in this card"
		return nil
	}
	if len(urlBlocks) == 1 {
		if err := openURL(urlBlocks[0].Content); err == nil {
			m.toast = fmt.Sprintf("✓ Opened %s in browser!", urlBlocks[0].Preview)
		} else {
			m.toast = fmt.Sprintf("Failed to open URL: %v", err)
		}
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}
	m.activeBlocks = urlBlocks
	m.selectedBlockIdx = 0
	m.showBlockPicker = true
	return nil
}

func (m *Model) applySearchQuery() {
	cards := m.store.List()
	if len(cards) == 0 {
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.searchQuery))
	if q == "" {
		return
	}

	for i, c := range cards {
		if strings.Contains(strings.ToLower(c.Title), q) ||
			strings.Contains(strings.ToLower(c.Content), q) ||
			strings.Contains(fmt.Sprintf("%d", c.Number), q) {
			m.selectedIndex = i
			m.updateViewportContent()
			return
		}
	}
}

func (m *Model) jumpSearchNext() {
	cards := m.store.List()
	if len(cards) == 0 {
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.searchQuery))
	if q == "" {
		return
	}
	start := (m.selectedIndex + 1) % len(cards)
	for i := 0; i < len(cards); i++ {
		idx := (start + i) % len(cards)
		c := cards[idx]
		if strings.Contains(strings.ToLower(c.Title), q) ||
			strings.Contains(strings.ToLower(c.Content), q) ||
			strings.Contains(fmt.Sprintf("%d", c.Number), q) {
			m.selectedIndex = idx
			m.updateViewportContent()
			return
		}
	}
}

func (m *Model) jumpSearchPrev() {
	cards := m.store.List()
	if len(cards) == 0 {
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.searchQuery))
	if q == "" {
		return
	}
	start := (m.selectedIndex - 1 + len(cards)) % len(cards)
	for i := 0; i < len(cards); i++ {
		idx := (start - i + len(cards)) % len(cards)
		c := cards[idx]
		if strings.Contains(strings.ToLower(c.Title), q) ||
			strings.Contains(strings.ToLower(c.Content), q) ||
			strings.Contains(fmt.Sprintf("%d", c.Number), q) {
			m.selectedIndex = idx
			m.updateViewportContent()
			return
		}
	}
}

func (m *Model) countSearchMatches() int {
	cards := m.store.List()
	q := strings.ToLower(strings.TrimSpace(m.searchQuery))
	if q == "" {
		return len(cards)
	}
	count := 0
	for _, c := range cards {
		if strings.Contains(strings.ToLower(c.Title), q) ||
			strings.Contains(strings.ToLower(c.Content), q) ||
			strings.Contains(fmt.Sprintf("%d", c.Number), q) {
			count++
		}
	}
	return count
}

func (m *Model) actionSwitchSession(sess watcher.SessionInfo) tea.Cmd {
	m.saveSessionState()
	m.sessionLabel = sess.ShortID
	m.store = card.NewStore()
	m.selectedIndex = 0
	m.lastTurnProcessed = 0
	m.showSessionSwitcher = false

	if path, err := card.SessionFilePath(sess.ShortID); err == nil {
		m.sessionFilePath = path
		if snap, err := m.store.LoadSnapshot(path); err == nil {
			m.lastTurnProcessed = snap.LastTurnProcessed
		}
	}

	harnessChoice := "auto"
	switch strings.ToLower(sess.Agent) {
	case "antigravity", "agy":
		harnessChoice = "agy"
	case "claude":
		harnessChoice = "claude"
	case "pi":
		harnessChoice = "pi"
	}
	m.harness = harness.NewAutoHarness(harnessChoice, "")
	m.runner = harness.NewRunner(m.harness, 1)

	m.turnCh = make(chan *watcher.Turn, 100)
	m.activityCh = make(chan watcher.AgentActivity, 20)
	fw := watcher.NewFileWatcher(sess.Path)
	go func() {
		_ = fw.Watch(context.Background(), m.turnCh, m.activityCh)
	}()

	m.toast = fmt.Sprintf("✓ Switched to session [%s] (%s)", sess.ShortID, sess.Agent)
	m.updateViewportContent()

	return tea.Batch(
		m.listenForTurns(),
		m.listenForActivity(),
		tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		}),
	)
}

func (m *Model) buildBlockPickerModal() string {
	boxWidth := 74
	if m.width > 20 && m.width < 78 {
		boxWidth = m.width - 4
	}

	cards := m.store.List()
	cardNum := 0
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		cardNum = cards[m.selectedIndex].Number
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(fmt.Sprintf("📦 ACTIONABLE STEPS & COMMANDS (CARD #%d)", cardNum))

	sub := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Press 1-9 to yank instantly, Enter to copy, s to send next door, o to open URL")

	var b strings.Builder
	b.WriteString(title + "\n" + sub + "\n\n")

	if len(m.activeBlocks) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("No actionable blocks found in this card.\n"))
	} else {
		for i, blk := range m.activeBlocks {
			isSelected := i == m.selectedBlockIdx
			keyStr := "   "
			if blk.Key != "" {
				keyStr = fmt.Sprintf("[%s]", blk.Key)
			}
			keyStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Width(5)
			if isSelected {
				keyStyle = keyStyle.Foreground(ColorPrimary)
			}

			badgeBg := ColorSecondary
			switch blk.Type {
			case card.BlockCode:
				badgeBg = ColorSuccess
			case card.BlockStep:
				badgeBg = ColorWarning
			case card.BlockURL:
				badgeBg = ColorPrimary
			}
			badgeStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#000000")).
				Background(badgeBg).
				Padding(0, 1)

			badgeRendered := badgeStyle.Render(blk.Badge)
			prefix := "  "
			if isSelected {
				prefix = "❯ "
			}

			availPrev := boxWidth - 22
			prev := blk.Preview
			if len(prev) > availPrev && availPrev > 6 {
				prev = prev[:availPrev-3] + "..."
			}

			line := fmt.Sprintf("%s%s %-10s %s", prefix, keyStyle.Render(keyStr), badgeRendered, prev)
			if isSelected {
				b.WriteString(lipgloss.NewStyle().Background(ColorHighlight).Bold(true).Width(boxWidth - 4).Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	}

	b.WriteString("\n")
	footer := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("[1-9] Quick Yank • [Enter] Yank • [s] Send to Peer • [o] Open URL • [Esc] Close")
	b.WriteString(footer)

	modalBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(lipgloss.Color("#1E222A")).
		Padding(1, 2).
		Width(boxWidth).
		Render(b.String())

	return modalBox
}

func (m *Model) buildSessionSwitcherModal() string {
	boxWidth := 74
	if m.width > 20 && m.width < 78 {
		boxWidth = m.width - 4
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render("🔄 SWITCH ACTIVE CONVERSATION SESSION")

	sub := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Select a recent session to watch (Antigravity, Claude Code, Pi)")

	var b strings.Builder
	b.WriteString(title + "\n" + sub + "\n\n")

	if len(m.sessionList) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("No conversation sessions found.\n"))
	} else {
		for i, s := range m.sessionList {
			isSelected := i == m.selectedSessionIdx
			prefix := "  "
			if isSelected {
				prefix = "❯ "
			}

			agentTag := fmt.Sprintf("[%s]", s.Agent)
			if s.Agent == "" {
				agentTag = "[agy]"
			}
			agentStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Width(15)

			idStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Width(10)
			idRendered := idStyle.Render(s.ShortID)

			timeAgo := watcher.FormatTimeAgo(s.ModTime)
			timeStyle := lipgloss.NewStyle().Foreground(ColorMuted).Width(12)

			availTitle := boxWidth - 44
			titleText := s.Title
			if len(titleText) > availTitle && availTitle > 6 {
				titleText = titleText[:availTitle-3] + "..."
			}

			line := fmt.Sprintf("%s%s %s %s %s", prefix, agentStyle.Render(agentTag), idRendered, lipgloss.NewStyle().Render(titleText), timeStyle.Render(timeAgo))
			if isSelected {
				b.WriteString(lipgloss.NewStyle().Background(ColorHighlight).Bold(true).Width(boxWidth - 4).Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	}

	b.WriteString("\n")
	footer := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("[j/k, ↑/↓] Navigate • [Enter] Switch • [Esc] Cancel")
	b.WriteString(footer)

	modalBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(lipgloss.Color("#1E222A")).
		Padding(1, 2).
		Width(boxWidth).
		Render(b.String())

	return modalBox
}

func (m *Model) buildHelpModal() string {
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
	b.WriteString(colKey.Render("b") + colDesc.Render("Surgical Block/Step picker (yank 1-9, run)") + "\n")
	b.WriteString(colKey.Render("u") + colDesc.Render("Open card URL directly in web browser") + "\n")
	b.WriteString(colKey.Render("/") + colDesc.Render("Search & filter reference cards") + "\n")
	b.WriteString(colKey.Render("S") + colDesc.Render("Switch active conversation session") + "\n")
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
		Background(lipgloss.Color("#16191E")).
		Padding(paddingY, 2)

	return boxStyle.Render(b.String())
}

func (m *Model) overlayModalOnBlurredBackdrop(baseView, modalBox string) string {
	bgLines := strings.Split(baseView, "\n")
	modalLines := strings.Split(modalBox, "\n")

	modalH := len(modalLines)
	modalW := lipgloss.Width(modalBox)

	targetW := m.width
	if targetW <= 0 {
		targetW = 80
	}
	targetH := m.height
	if targetH <= 0 {
		targetH = len(bgLines)
	}
	if targetH < modalH {
		targetH = modalH
	}

	modalX := (targetW - modalW) / 2
	if modalX < 0 {
		modalX = 0
	}
	modalY := (targetH - modalH) / 2
	if modalY < 0 {
		modalY = 0
	}

	// Blur style: faint muted slate gray that pushes background content into defocus
	blurStyle := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#4C566A"))

	outLines := make([]string, targetH)
	for y := 0; y < targetH; y++ {
		rawBg := ""
		if y < len(bgLines) {
			rawBg = bgLines[y]
		}
		strippedBg := ansi.Strip(rawBg)

		if y >= modalY && y < modalY+modalH {
			// Row is partially covered by modal
			modalLine := modalLines[y-modalY]
			curModalW := ansi.StringWidth(modalLine)
			if curModalW < modalW {
				modalLine += strings.Repeat(" ", modalW-curModalW)
			}

			// Left side of backdrop
			leftPlain := ""
			if modalX > 0 {
				leftPlain = ansi.CutWc(strippedBg, 0, modalX)
				lw := ansi.StringWidth(leftPlain)
				if lw < modalX {
					leftPlain += strings.Repeat(" ", modalX-lw)
				}
			}

			// Right side of backdrop
			rightStart := modalX + modalW
			rightAvail := targetW - rightStart
			rightPlain := ""
			if rightAvail > 0 {
				rightPlain = ansi.CutWc(strippedBg, rightStart, targetW)
				rw := ansi.StringWidth(rightPlain)
				if rw < rightAvail {
					rightPlain += strings.Repeat(" ", rightAvail-rw)
				}
			}

			line := ""
			if modalX > 0 {
				line += blurStyle.Render(leftPlain)
			}
			line += modalLine
			if rightAvail > 0 {
				line += blurStyle.Render(rightPlain)
			}
			outLines[y] = line
		} else {
			// Entire row is backdrop
			lineContent := strippedBg
			cw := ansi.StringWidth(lineContent)
			if cw < targetW {
				lineContent += strings.Repeat(" ", targetW-cw)
			} else if cw > targetW {
				lineContent = ansi.CutWc(lineContent, 0, targetW)
			}
			outLines[y] = blurStyle.Render(lineContent)
		}
	}

	return strings.Join(outLines, "\n")
}

func (m *Model) renderHelpModal() string {
	return m.overlayModalOnBlurredBackdrop(m.renderBaseView(), m.buildHelpModal())
}

func (m *Model) summarizeCardTitle(cardID string, turn *watcher.Turn) tea.Cmd {
	return func() tea.Msg {
		if m.runner == nil || turn == nil {
			return nil
		}
		title, err := m.runner.SummarizeTurn(context.Background(), m.sessionLabel, turn)
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

// cardTypeBadge returns the rendered badge string for a card.
// TypeProcedure is the overwhelming default and adds no signal, so it is hidden.
// Only CMD, DEC, INFO, and INVALID badges are shown.
func cardTypeBadge(c *card.Card) string {
	if c.Status == card.StatusInvalidated {
		return BadgeInvalid.Render()
	}
	switch c.Type {
	case card.TypeCommand:
		return BadgeCmd.Render()
	case card.TypeDecision:
		return BadgeDec.Render()
	case card.TypeInfo:
		return BadgeInfo.Render()
	default:
		// TypeProcedure and anything else: no badge — silence the noise
		return ""
	}
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
	case "block_picker":
		m.actionToggleBlockPicker()
	case "search":
		m.searchMode = true
		m.searchQuery = ""
		m.searchSavedIdx = m.selectedIndex
	case "sessions":
		if !m.isDemo {
			if sessions, err := watcher.ListSessions(12); err == nil && len(sessions) > 0 {
				m.sessionList = sessions
				m.selectedSessionIdx = 0
				m.showSessionSwitcher = true
			}
		}
	case "help":
		m.showHelp = !m.showHelp
	case "demo_next":
		if m.isDemo && m.demoStep < 3 {
			return m.advanceDemo()
		}
		m.toast = "Demo finished! All turns demonstrated."
	case "quit":
		m.saveSessionState()
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
		if msg.Turn != nil && msg.Turn.ID <= m.lastTurnProcessed {
			// Turn was already processed in a saved snapshot.
			// Upgrade card title if a definitive title is now in cache.
			if cached, ok := harness.GetTitleCache().Get(m.sessionLabel, msg.Turn.ID); ok && harness.IsDefinitiveTitle(cached) {
				for _, c := range m.store.List() {
					if len(c.History) > 0 && strings.Contains(c.History[0].Reason, fmt.Sprintf("Turn %d", msg.Turn.ID)) {
						if !harness.IsDefinitiveTitle(c.Title) {
							m.store.UpdateTitle(c.ID, cached)
							m.updateViewportContent()
							m.saveSessionState()
						}
					}
				}
			}
			return m, m.listenForTurns()
		}

		m.reconciling = true
		return m, tea.Batch(m.applyReconcile(msg.Turn), m.listenForTurns())

	case MutationsAppliedMsg:
		m.reconciling = false
		if msg.Turn != nil && msg.Turn.ID > m.lastTurnProcessed {
			m.lastTurnProcessed = msg.Turn.ID
		}

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

				// 2. Queue AI summarization if not definitive in cache
				if msg.Turn != nil && m.runner != nil {
					hasDefinitive := false
					if cached, ok := harness.GetTitleCache().Get(m.sessionLabel, msg.Turn.ID); ok && harness.IsDefinitiveTitle(cached) {
						hasDefinitive = true
					} else if cached, ok := harness.GetTitleCache().GetByPrompt(msg.Turn.User); ok && harness.IsDefinitiveTitle(cached) {
						hasDefinitive = true
					}

					if !hasDefinitive {
						cmds = append(cmds, m.summarizeCardTitle(mut.CardID, msg.Turn))
					}
				}
			case reconciler.OpUpdate:
				m.store.Update(mut.CardID, mut.Content, mut.Reason)
				m.toast = fmt.Sprintf("Updated: %s", mut.Reason)
				if msg.Turn != nil && m.runner != nil {
					// Check if card ALREADY has a definitive title
					c, ok := m.store.Get(mut.CardID)
					if ok && harness.IsDefinitiveTitle(c.Title) {
						// Preserve established card title! Do not overwrite with follow-up turn prompt!
						continue
					}

					hasDefinitive := false
					if cached, ok := harness.GetTitleCache().Get(m.sessionLabel, msg.Turn.ID); ok && harness.IsDefinitiveTitle(cached) {
						hasDefinitive = true
					} else if cached, ok := harness.GetTitleCache().GetByPrompt(msg.Turn.User); ok && harness.IsDefinitiveTitle(cached) {
						hasDefinitive = true
					}

					if !hasDefinitive {
						cmds = append(cmds, m.summarizeCardTitle(mut.CardID, msg.Turn))
					}
				}
			case reconciler.OpInvalidate:
				m.store.Invalidate(mut.CardID, mut.Reason)
				m.toast = fmt.Sprintf("Invalidated: %s", mut.Reason)
			}
		}
		m.updateViewportContent()
		m.saveSessionState()
		cmds = append(cmds, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		}))
		return m, tea.Batch(cmds...)

	case TitleSummarizedMsg:
		if m.store.UpdateTitle(msg.CardID, msg.Title) {
			m.updateViewportContent()
			m.saveSessionState()
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
			if m.showBlockPicker {
				m.showBlockPicker = false
				return m, nil
			}
			if m.showSessionSwitcher {
				m.showSessionSwitcher = false
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
						cardIdx := m.cardListOffset + (msg.Y - 2)
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

		if m.showBlockPicker {
			switch msg.String() {
			case "esc", "q", "b":
				m.showBlockPicker = false
				return m, nil
			case "up", "k":
				if m.selectedBlockIdx > 0 {
					m.selectedBlockIdx--
				}
				return m, nil
			case "down", "j":
				if m.selectedBlockIdx < len(m.activeBlocks)-1 {
					m.selectedBlockIdx++
				}
				return m, nil
			case "enter", "c":
				return m, m.actionCopyBlock(m.selectedBlockIdx)
			case "s":
				return m, m.actionSendBlock(m.selectedBlockIdx)
			case "o":
				return m, m.actionOpenBlockURL(m.selectedBlockIdx)
			case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
				for idx, blk := range m.activeBlocks {
					if blk.Key == msg.String() {
						return m, m.actionCopyBlock(idx)
					}
				}
				return m, nil
			default:
				return m, nil
			}
		}

		if m.showSessionSwitcher {
			switch msg.String() {
			case "esc", "q", "S":
				m.showSessionSwitcher = false
				return m, nil
			case "up", "k":
				if m.selectedSessionIdx > 0 {
					m.selectedSessionIdx--
				}
				return m, nil
			case "down", "j":
				if m.selectedSessionIdx < len(m.sessionList)-1 {
					m.selectedSessionIdx++
				}
				return m, nil
			case "enter":
				if len(m.sessionList) > 0 && m.selectedSessionIdx < len(m.sessionList) {
					return m, m.actionSwitchSession(m.sessionList[m.selectedSessionIdx])
				}
				return m, nil
			default:
				return m, nil
			}
		}

		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.selectedIndex = m.searchSavedIdx
				m.updateViewportContent()
				return m, nil
			case "enter":
				m.searchMode = false
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.applySearchQuery()
				}
				return m, nil
			case "down", "tab", "ctrl+n":
				m.jumpSearchNext()
				return m, nil
			case "up", "shift+tab", "ctrl+p":
				m.jumpSearchPrev()
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
					m.applySearchQuery()
					return m, nil
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.saveSessionState()
			return m, tea.Quit

		case "?", "f1":
			m.showHelp = true
			return m, nil

		case "b":
			m.actionToggleBlockPicker()
			return m, nil

		case "u":
			return m, m.actionOpenURL()

		case "/":
			m.searchMode = true
			m.searchQuery = ""
			m.searchSavedIdx = m.selectedIndex
			return m, nil

		case "S":
			if m.isDemo {
				m.toast = "Demo mode: session switching disabled"
				return m, nil
			}
			sessions, err := watcher.ListSessions(12)
			if err != nil || len(sessions) == 0 {
				m.toast = "No active or past sessions found"
				return m, nil
			}
			m.sessionList = sessions
			m.selectedSessionIdx = 0
			m.showSessionSwitcher = true
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

	// 2. Copy Code & Surgical Blocks
	addButton("📋 Copy (c)", "copy", BtnStyle)
	addButton("📦 Steps (b)", "block_picker", BtnStyle)

	// 3. Copy All (when width allows)
	if m.width >= 90 {
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

	// 6. Search & Sessions (when width allows)
	if m.width >= 100 {
		addButton("🔍 Find (/)", "search", BtnStyle)
		if !m.isDemo {
			addButton("🔄 Switch (S)", "sessions", BtnStyle)
		}
	}

	// 7. Export (when width allows)
	if m.width >= 115 {
		addButton("💾 Export (e)", "export", BtnStyle)
	}

	// 8. History (when width allows)
	if m.width >= 130 {
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

func (m *Model) renderBaseView() string {
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
			agentName := "AGY"
			if m.runner != nil && m.runner.Name() != "" && m.runner.Name() != "heuristic" {
				agentName = strings.ToUpper(m.runner.Name())
			}
			emptyMsg = fmt.Sprintf("%s Agent is processing your request in %s...\n\nAction: %s\nPrompt: %q\n\nReference cards will appear automatically when the turn completes.",
				m.spinner.View(), agentName, m.agentTool, m.agentPrompt)
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
	if m.searchMode {
		matches := m.countSearchMatches()
		hintText = fmt.Sprintf("🔍 Search cards: %s_  [%d matches]  (Enter: Keep • Esc: Cancel • ↑/↓: Next/Prev)", m.searchQuery, matches)
	} else if m.width < 90 {
		if m.viewMode == ModeDetail {
			hintText = "💡 Tab: Show • b: Steps • /: Search • ?: Help • q: Quit"
		} else {
			hintText = "💡 Tab: Hide • b: Steps • /: Search • ?: Help • q: Quit"
		}
	} else {
		if m.viewMode == ModeDetail {
			hintText = "💡 ↑/↓: Scroll • j/k: Turn • b: Steps • c: Copy • /: Search • S: Sessions • ?: Help"
		} else {
			hintText = "💡 ↑/↓: Scroll • j/k: Turn • Tab: Focus • b: Steps • c: Copy • /: Search • S: Sessions • ?: Help"
		}
	}
	hints := FooterStyle.Render(hintText)

	return lipgloss.JoinVertical(lipgloss.Left, header, actionBar, body, hints)
}

func (m *Model) View() string {
	base := m.renderBaseView()
	if m.showHelp {
		return m.overlayModalOnBlurredBackdrop(base, m.buildHelpModal())
	}
	if m.showBlockPicker {
		return m.overlayModalOnBlurredBackdrop(base, m.buildBlockPickerModal())
	}
	if m.showSessionSwitcher {
		return m.overlayModalOnBlurredBackdrop(base, m.buildSessionSwitcherModal())
	}
	return base
}

func (m *Model) renderDetailFocus(cards []*card.Card) string {
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		return ""
	}
	selected := cards[m.selectedIndex]

	badgeStr := cardTypeBadge(selected)

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

	badgePart := ""
	if badgeStr != "" {
		badgePart = badgeStr + " "
	}
	bannerLeft := fmt.Sprintf("%s  %s %s%s%s %s", modeIndicator, numStr, badgePart, pinIcon, titleStr, verStr)
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

	availHeight := m.viewport.Height
	if availHeight <= 0 {
		availHeight = m.height - 3
	}
	if availHeight <= 0 {
		availHeight = 20
	}

	cardSlots := availHeight
	if m.isAgentBusy {
		cardSlots--
	}
	if cardSlots < 1 {
		cardSlots = 1
	}

	if m.selectedIndex < m.cardListOffset {
		m.cardListOffset = m.selectedIndex
	} else if m.selectedIndex >= m.cardListOffset+cardSlots {
		m.cardListOffset = m.selectedIndex - cardSlots + 1
	}
	if m.cardListOffset < 0 {
		m.cardListOffset = 0
	}
	maxOffset := len(cards) - cardSlots
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.cardListOffset > maxOffset {
		m.cardListOffset = maxOffset
	}

	endIdx := m.cardListOffset + cardSlots
	if endIdx > len(cards) {
		endIdx = len(cards)
	}

	visibleCards := cards[m.cardListOffset:endIdx]
	leftLines := make([]string, 0, len(visibleCards)+1)
	for idx, c := range visibleCards {
		i := m.cardListOffset + idx
		badgeStr := cardTypeBadge(c)

		prefix := "  "
		if i == m.selectedIndex {
			prefix = "▸ "
		}

		numStr := BadgeNumberStyle.Render(fmt.Sprintf("#%-2d", c.Number))
		pinIcon := ""
		if c.Pinned {
			pinIcon = "📌 "
		}
		verStr := BadgeVersionStyle.Render(fmt.Sprintf("v%d", c.Version))

		// Calculate available width for the title so the row NEVER wraps
		badgePadding := 0
		if badgeStr != "" {
			badgePadding = lipgloss.Width(badgeStr) + 1
		}
		fixedWidth := lipgloss.Width(prefix) + lipgloss.Width(numStr) + 1 + badgePadding + lipgloss.Width(pinIcon) + 1 + lipgloss.Width(verStr)
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

		var leftPart string
		if badgeStr != "" {
			leftPart = fmt.Sprintf("%s%s %s %s%s", prefix, numStr, badgeStr, pinIcon, title)
		} else {
			leftPart = fmt.Sprintf("%s%s %s%s", prefix, numStr, pinIcon, title)
		}
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
