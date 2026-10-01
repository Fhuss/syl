package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/harness"
	"github.com/Fhuss/syl/pkg/multiplexer"
	"github.com/Fhuss/syl/pkg/reconciler"
	"github.com/Fhuss/syl/pkg/watcher"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var codeFenceRegex = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9_-]+)?\n(.*?)\n```")

type ViewMode int

const (
	ModeSplit ViewMode = iota
	ModeGlance
	ModeDetail
	ModeHistory
)

type PaneType int

const (
	PaneCards PaneType = iota
	PanePreview
)

type EditorFinishedMsg struct {
	Err        error
	TempFile   string
	CardID     string
	Generation uint64
}

type TurnReceivedMsg struct {
	Turn       *watcher.Turn
	Generation uint64
}

type agentActivityMsg struct {
	Activity   watcher.AgentActivity
	Generation uint64
}

type MutationsAppliedMsg struct {
	Mutations  []reconciler.Mutation
	Turn       *watcher.Turn
	Generation uint64
}

type TitleSummarizedMsg struct {
	CardID     string
	Title      string
	Turn       *watcher.Turn
	Generation uint64
}

type ReconcileFailedMsg struct {
	Err        error
	Turn       *watcher.Turn
	Generation uint64
}

type retryReconcileMsg struct {
	TurnID     int
	Attempt    int
	Generation uint64
}

type ClearToastMsg struct{}

type checkNewSessionMsg struct{}
type pollAgainMsg struct{}
type newSessionDetectedMsg struct {
	Session watcher.SessionInfo
}

type Button struct {
	Label  string
	Action string
	Row    int
	X1     int
	X2     int
}

type Model struct {
	store             *card.Store
	reconciler        reconciler.Reconciler
	harness           harness.Harness
	runner            *harness.Runner
	turnCh            chan *watcher.Turn
	activityCh        chan watcher.AgentActivity
	watchCtx          context.Context
	watchCancel       context.CancelFunc
	reconcileMu       sync.Mutex
	generation        uint64
	pendingTurns      []*watcher.Turn
	failedTurn        *watcher.Turn
	reconcileAttempts int
	saveError         string
	loadError         error
	selectedIndex     int
	historyIndex      int
	historyCardID     string
	viewMode          ViewMode
	isDemo            bool
	demoStep          int
	width             int
	height            int
	toast             string
	reconciling       bool
	renderer          *glamour.TermRenderer
	viewport          viewport.Model
	buttons           []Button
	sessionLabel      string
	spinner           spinner.Model
	isAgentBusy       bool
	agentTool         string
	agentPrompt       string
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

	// Recoverable drops / selected trash purge.
	showTrash         bool
	selectedTrashIdx  int
	confirmTrashPurge bool

	// Vim Enhancements
	activePane     PaneType
	lastGKeyTime   time.Time
	lastZKeyTime   time.Time
	isFolded       bool
	searchMatches  []int
	searchMatchIdx int
	currentContent string

	// Waiting for conversation mode
	waitingForSession bool
	waitingSince      time.Time
	peerCWD           string
	peerAgent         string
}

func (m *Model) SetWaitingForSession(waiting bool, peerCWD, peerAgent string) {
	m.waitingForSession = waiting
	if waiting {
		m.waitingSince = time.Now().Add(-2 * time.Second)
		m.sessionLabel = "waiting"
	}
	m.peerCWD = peerCWD
	m.peerAgent = peerAgent
}

func snapshotLoadWarning(err error) string {
	return fmt.Sprintf("Snapshot unreadable: %v. Original preserved; quit and repair it.", err)
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
	var sessionFilePath, saveError string
	var loadError error
	if !isDemo && sessionLabel != "" {
		if path, err := card.SessionFilePath(sessionLabel); err == nil {
			sessionFilePath = path
			loadInto := store
			if len(store.List()) > 0 {
				loadInto = card.NewStore() // Validate without replacing caller-supplied cards.
			}
			if snap, err := loadInto.LoadSnapshot(path); err == nil {
				if loadInto == store {
					lastTurn = snap.LastTurnProcessed
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				loadError = fmt.Errorf("%s: %w", path, err)
			}
		} else {
			saveError = fmt.Sprintf("Save failed: %v", err)
		}
	}

	var rnr *harness.Runner
	if h != nil {
		rnr = harness.NewRunner(h, 1)
	}

	watchCtx, watchCancel := context.WithCancel(context.Background())
	m := &Model{
		watchCtx:          watchCtx,
		watchCancel:       watchCancel,
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
		saveError:         saveError,
		loadError:         loadError,
		toast:             saveError,
		spinner:           sp,
	}
	if loadError != nil {
		m.toast = snapshotLoadWarning(loadError)
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
		if m.waitingForSession {
			cmds = append(cmds, m.pollNewSessionCmd())
		}
		if m.turnCh != nil {
			cmds = append(cmds, m.listenForTurns())
		}
		if m.activityCh != nil {
			cmds = append(cmds, m.listenForActivity())
		}
	}

	return tea.Batch(cmds...)
}

func (m *Model) pollNewSessionCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return checkNewSessionMsg{}
	})
}

// StopWatching cancels the active file tail and pending channel listeners.
func (m *Model) StopWatching() {
	if m.watchCancel != nil {
		m.watchCancel()
	}
}

// StartWatching owns the watcher and its listeners so switching sessions can
// cancel the previous file tail instead of mixing two conversations.
func (m *Model) StartWatching(path string) {
	m.StopWatching()
	if m.loadError != nil {
		return // Never process turns from an unreadable snapshot.
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.watchCtx, m.watchCancel = ctx, cancel
	m.generation++
	m.turnCh = make(chan *watcher.Turn, 100)
	m.activityCh = make(chan watcher.AgentActivity, 20)
	turns, activity := m.turnCh, m.activityCh
	go func() {
		if err := watcher.NewFileWatcher(path).Watch(ctx, turns, activity); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
		}
	}()
}

func (m *Model) listenForActivity() tea.Cmd {
	ch, ctx, generation := m.activityCh, m.watchCtx, m.generation
	return func() tea.Msg {
		select {
		case act, ok := <-ch:
			if ok {
				return agentActivityMsg{Activity: act, Generation: generation}
			}
		case <-ctx.Done():
		}
		return nil
	}
}

func (m *Model) listenForTurns() tea.Cmd {
	ch, ctx, generation := m.turnCh, m.watchCtx, m.generation
	return func() tea.Msg {
		select {
		case turn, ok := <-ch:
			if ok {
				return TurnReceivedMsg{Turn: turn, Generation: generation}
			}
		case <-ctx.Done():
		}
		return nil
	}
}

func (m *Model) applyReconcile(turn *watcher.Turn) tea.Cmd {
	// Snapshot inputs before starting a Bubble Tea command on another goroutine.
	// Reconciler jobs are serialized to preserve transcript order.
	cards := m.store.List()
	copies := make([]*card.Card, len(cards))
	for i, c := range cards {
		copyCard := *c
		copyCard.History = append([]card.CardVersion(nil), c.History...)
		copies[i] = &copyCard
	}
	rec, generation, sessionCtx := m.reconciler, m.generation, m.watchCtx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(sessionCtx, 20*time.Second)
		defer cancel()
		m.reconcileMu.Lock()
		defer m.reconcileMu.Unlock()
		if err := ctx.Err(); err != nil {
			return ReconcileFailedMsg{Err: err, Turn: turn, Generation: generation}
		}
		muts, err := rec.Reconcile(ctx, copies, turn)
		if err != nil {
			return ReconcileFailedMsg{Err: err, Turn: turn, Generation: generation}
		}
		return MutationsAppliedMsg{Mutations: muts, Turn: turn, Generation: generation}
	}
}

func (m *Model) retryFailedTurn() tea.Cmd {
	if m.failedTurn == nil || m.reconciling {
		return nil
	}
	m.reconciling = true
	m.toast = "Retrying card capture..."
	return m.applyReconcile(m.failedTurn)
}

func (m *Model) nextPendingTurn() tea.Cmd {
	if len(m.pendingTurns) == 0 {
		m.reconciling = false
		return nil
	}
	turn := m.pendingTurns[0]
	m.pendingTurns = m.pendingTurns[1:]
	m.reconciling = true
	return m.applyReconcile(turn)
}

func (m *Model) advanceDemo() tea.Cmd {
	m.demoStep++
	turn := &watcher.Turn{
		ID:        m.demoStep,
		Timestamp: time.Now(),
	}
	switch m.demoStep {
	case 1:
		turn.User = "Welcome to Syl! Show me how this works and give me a full tour of features and shortcuts."
		turn.Assistant = "Welcome to Syl! Here is your quickstart tour with tips, tricks, and interactive cards for testing."
	case 2:
		turn.User = "Can you update the quickstart guide with companion skill installation instructions?"
		turn.Assistant = "Updated the quickstart guide to v2 and tracked the change in revision history."
	case 3:
		turn.User = "Let's record the architecture decision to support both Ghostty cmux and tmux natively"
		turn.Assistant = "Recorded the multiplexer architecture decision and added discovery commands."
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

	barLines := m.getActionBarLines()

	var wrapWidth int
	var vpHeight int

	if m.viewMode == ModeDetail {
		wrapWidth = m.width - 4
		if wrapWidth < 25 {
			wrapWidth = 25
		}
		// Accounting for header, focus banner, action bar lines, hints, and borders
		vpHeight = m.height - 5 - barLines
		if vpHeight < 5 {
			vpHeight = 5
		}
	} else {
		leftColWidth := m.getLeftColWidth()
		wrapWidth = m.width - leftColWidth - 4
		if wrapWidth < 25 {
			wrapWidth = 25
		}
		// Accounting for header, action bar lines, hints, and borders
		vpHeight = m.height - 4 - barLines
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
	m.updateViewportContentPreserve(false)
}

func (m *Model) updateViewportContentPreserve(preserveOffset bool) {
	prevOffset := m.viewport.YOffset
	cards := m.store.List()
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		m.viewport.SetContent("No card selected.")
		return
	}

	selected := cards[m.selectedIndex]
	var finalContent string
	if m.viewMode == ModeHistory {
		if m.historyCardID != selected.ID {
			m.historyCardID = selected.ID
			m.historyIndex = len(selected.History) - 1
		}
		if len(selected.History) == 0 {
			finalContent = "No revision history available."
		} else {
			if m.historyIndex < 0 || m.historyIndex >= len(selected.History) {
				m.historyIndex = len(selected.History) - 1
			}
			revision := selected.History[m.historyIndex]
			header := fmt.Sprintf("Revision History: %s · entry %d/%d · v%d\n%s — %s\n\n",
				selected.Title, m.historyIndex+1, len(selected.History), revision.Version,
				revision.UpdatedAt.Format("2006-01-02 15:04"), revision.Reason)
			content, err := m.renderer.Render(revision.Content)
			if err != nil {
				content = revision.Content
			}
			finalContent = lipgloss.NewStyle().Bold(true).Render(header) + content
		}

	} else if m.showRaw {
		rawContent := selected.Content
		if m.isFolded {
			rawContent = card.FoldCodeBlocks(rawContent)
		}
		finalContent = rawContent
	} else {
		contentToRender := selected.Content
		if m.isFolded {
			contentToRender = card.FoldCodeBlocks(contentToRender)
		}
		rendered, err := m.renderer.Render(contentToRender)
		if err != nil {
			rendered = contentToRender
		}
		finalContent = rendered
	}

	m.currentContent = finalContent
	m.viewport.SetContent(finalContent)

	if preserveOffset {
		m.viewport.SetYOffset(prevOffset)
	} else {
		m.viewport.GotoTop()
	}
	m.computeSearchMatches()
}

func (m *Model) computeSearchMatches() {
	m.searchMatches = nil
	m.searchMatchIdx = 0
	if m.searchQuery == "" {
		return
	}
	if m.currentContent == "" {
		m.updateViewportContentPreserve(true)
	}
	lines := strings.Split(m.currentContent, "\n")
	queryLower := strings.ToLower(m.searchQuery)
	for i, l := range lines {
		stripped := strings.ToLower(ansi.Strip(l))
		if strings.Contains(stripped, queryLower) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
}

func (m *Model) jumpSearchMatch(next bool) {
	if m.searchQuery == "" {
		m.toast = "No search pattern (use '/' to search)"
		return
	}
	if len(m.searchMatches) == 0 {
		m.computeSearchMatches()
		if len(m.searchMatches) == 0 {
			m.toast = fmt.Sprintf("Pattern not found in card: %s", m.searchQuery)
			return
		}
	}

	if next {
		m.searchMatchIdx = (m.searchMatchIdx + 1) % len(m.searchMatches)
	} else {
		m.searchMatchIdx = (m.searchMatchIdx - 1 + len(m.searchMatches)) % len(m.searchMatches)
		if m.searchMatchIdx < 0 {
			m.searchMatchIdx += len(m.searchMatches)
		}
	}

	targetLine := m.searchMatches[m.searchMatchIdx]
	m.viewport.SetYOffset(targetLine)
	m.toast = fmt.Sprintf("[%d/%d] Match: %s", m.searchMatchIdx+1, len(m.searchMatches), m.searchQuery)
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

func (m *Model) actionTogglePaneFocus() {
	if m.viewMode == ModeDetail {
		return
	}
	if m.activePane == PaneCards {
		m.activePane = PanePreview
		m.toast = "Focus: Preview buffer (j/k scroll, 'w' switch back)"
	} else {
		m.activePane = PaneCards
		m.toast = "Focus: Card turns list (j/k select, 'w' switch back)"
	}
}

func (m *Model) actionToggleFold() {
	m.isFolded = !m.isFolded
	if m.isFolded {
		m.toast = "Folded code blocks ('za' to expand)"
	} else {
		m.toast = "Expanded code blocks ('za' to fold)"
	}
	m.updateViewportContentPreserve(true)
}

func (m *Model) actionToggleChecklist() tea.Cmd {
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return nil
	}
	cards := m.store.List()
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		return nil
	}
	c := cards[m.selectedIndex]
	newContent, toggled, toastMsg := card.ToggleTaskCheckbox(c.Content)
	if !toggled {
		m.toast = "No checklist items (- [ ]) in card"
	} else {
		m.store.Update(c.ID, newContent, "Task toggled")
		if err := m.saveSessionState(); err == nil {
			m.toast = "✓ " + toastMsg
		}
		m.updateViewportContentPreserve(true)
	}
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ClearToastMsg{}
	})
}

func (m *Model) actionOpenInEditor() tea.Cmd {
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return nil
	}
	cards := m.store.List()
	if len(cards) == 0 || m.selectedIndex >= len(cards) {
		m.toast = "No card selected"
		return nil
	}
	c := cards[m.selectedIndex]

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		for _, e := range []string{"nvim", "vim", "nano", "vi"} {
			if p, err := exec.LookPath(e); err == nil {
				editor = p
				break
			}
		}
	}
	if editor == "" {
		m.toast = "No editor found ($EDITOR not set, nvim/vim/nano not found)"
		return nil
	}

	file, err := os.CreateTemp("", "syl-*.md")
	if err != nil {
		m.toast = fmt.Sprintf("Error creating temp file: %v", err)
		return nil
	}
	tempFile := file.Name()
	if _, err = file.WriteString(c.Content); err != nil {
		file.Close()
		os.Remove(tempFile)
		m.toast = fmt.Sprintf("Error writing temp file: %v", err)
		return nil
	}
	if err = file.Close(); err != nil {
		os.Remove(tempFile)
		m.toast = fmt.Sprintf("Error closing temp file: %v", err)
		return nil
	}

	// $EDITOR commonly includes flags and can contain a quoted executable path.
	args, err := parseEditorCommand(editor)
	if err != nil {
		os.Remove(tempFile)
		m.toast = fmt.Sprintf("Invalid editor command: %v", err)
		return nil
	}
	cmd := exec.Command(args[0], append(args[1:], tempFile)...)
	generation := m.generation
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err, TempFile: tempFile, CardID: c.ID, Generation: generation}
	})
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
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return nil
	}
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		selectedID := cards[m.selectedIndex].ID
		pinned := m.store.TogglePin(selectedID)
		if err := m.saveSessionState(); err == nil {
			if pinned {
				m.toast = "📌 Pinned card"
			} else {
				m.toast = "Unpinned card"
			}
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
		m.historyCardID = ""
	} else {
		m.viewMode = ModeHistory
		cards := m.store.List()
		if m.selectedIndex < len(cards) {
			m.historyCardID = cards[m.selectedIndex].ID
			m.historyIndex = len(cards[m.selectedIndex].History) - 1
		}
	}
	m.updateViewportSize()
	m.updateViewportContent()
}

func (m *Model) actionRestoreHistoryRevision() tea.Cmd {
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return nil
	}
	cards := m.store.List()
	if m.selectedIndex >= len(cards) || m.historyIndex < 0 || m.historyIndex >= len(cards[m.selectedIndex].History) {
		return nil
	}
	c := cards[m.selectedIndex]
	if m.historyIndex == len(c.History)-1 {
		m.toast = "This is already the current revision"
		return nil
	}
	previousVersion := c.History[m.historyIndex].Version
	restored, ok := m.store.RestoreRevision(c.ID, m.historyIndex)
	if !ok {
		return nil
	}
	m.historyIndex = len(restored.History) - 1
	m.updateViewportContent()
	if err := m.saveSessionState(); err == nil {
		m.toast = fmt.Sprintf("Restored content from v%d as v%d", previousVersion, restored.Version)
	}
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return ClearToastMsg{} })
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
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return nil
	}
	cards := m.store.List()
	if len(cards) > 0 && m.selectedIndex < len(cards) {
		c := cards[m.selectedIndex]
		m.store.Remove(c.ID)
		if err := m.saveSessionState(); err == nil {
			m.toast = fmt.Sprintf("Dropped card #%d · U to undo", c.Number)
		}
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

func (m *Model) actionOpenTrash() {
	if !m.store.CanUndoDrop() {
		m.toast = "No dropped cards in trash"
		return
	}
	m.showTrash = true
	m.confirmTrashPurge = false
	m.selectedTrashIdx = 0
}

func (m *Model) actionUndoDrop() tea.Cmd {
	return m.actionRestoreDropped(0)
}

func (m *Model) actionRestoreDropped(index int) tea.Cmd {
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return nil
	}
	c, ok := m.store.RestoreDropped(index)
	if !ok {
		m.toast = "No dropped card to restore"
	} else {
		m.showTrash = false
		m.confirmTrashPurge = false
		for idx, candidate := range m.store.List() {
			if candidate.ID == c.ID {
				m.selectedIndex = idx
				break
			}
		}
		m.updateViewportContent()
		if err := m.saveSessionState(); err == nil {
			m.toast = fmt.Sprintf("Restored card #%d: %s", c.Number, c.Title)
		}
	}
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return ClearToastMsg{} })
}

func (m *Model) actionPurgeDropped(index int) tea.Cmd {
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return nil
	}
	c, ok := m.store.PurgeDropped(index)
	if !ok {
		return nil
	}
	m.confirmTrashPurge = false
	if remaining := len(m.store.DroppedCards()); remaining == 0 {
		m.showTrash = false
	} else if m.selectedTrashIdx >= remaining {
		m.selectedTrashIdx = remaining - 1
	}
	if err := m.saveSessionState(); err == nil {
		m.toast = fmt.Sprintf("Purged card #%d from Syl trash (backups may retain it)", c.Number)
	}
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return ClearToastMsg{} })
}

func (m *Model) saveSessionState() error {
	if m.loadError != nil {
		m.toast = snapshotLoadWarning(m.loadError)
		return m.loadError // Never overwrite an unreadable snapshot.
	}
	if m.isDemo || m.sessionLabel == "" {
		return nil
	}
	var err error
	if m.sessionFilePath == "" {
		err = fmt.Errorf("session snapshot path is unavailable")
	} else {
		err = m.store.SaveSnapshot(m.sessionFilePath, m.sessionLabel, m.lastTurnProcessed)
	}
	if err != nil {
		m.saveError = fmt.Sprintf("Save failed: %v", err)
		m.toast = m.saveError
		return err
	}
	if m.toast == m.saveError {
		m.toast = ""
	}
	m.saveError = ""
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
		m.toast = "No code blocks, steps, URLs, or files found in this card"
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
	switch blk.Type {
	case card.BlockURL:
		if err := openURL(blk.Content); err == nil {
			m.toast = fmt.Sprintf("✓ Opened %s in browser!", blk.Preview)
		} else {
			m.toast = fmt.Sprintf("Failed to open URL: %v", err)
		}
	case card.BlockFile:
		return m.actionOpenFile(blk.Content)
	default:
		return nil
	}
	m.showBlockPicker = false
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ClearToastMsg{}
	})
}

// actionOpenFile opens or previews a file path using the system's default handler.
func (m *Model) actionOpenFile(path string) tea.Cmd {
	// Check if the file exists
	info, err := os.Stat(path)
	if err != nil {
		m.toast = fmt.Sprintf("⚠ File not found: %s", filepath.Base(path))
		m.showBlockPicker = false
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})
	}

	// Use system open to preview/open the file
	if err := openURL(path); err == nil {
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		m.toast = fmt.Sprintf("✓ Opened %s (%s)", filepath.Base(path), kind)
	} else {
		m.toast = fmt.Sprintf("Failed to open file: %v", err)
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
	if m.loadError != nil && len(m.store.List()) > 0 {
		m.toast = "Snapshot unreadable; cannot switch away from unsaved cards"
		return nil
	}
	if m.loadError == nil {
		if err := m.saveSessionState(); err != nil {
			return nil // Keep unsaved cards in this session until storage recovers.
		}
	}
	path, err := card.SessionFilePath(sess.ShortID)
	if err != nil {
		m.toast = fmt.Sprintf("Could not open session: %v", err)
		return nil
	}
	nextStore := card.NewStore()
	var lastTurn int
	if snap, err := nextStore.LoadSnapshot(path); err == nil {
		lastTurn = snap.LastTurnProcessed
	} else if !errors.Is(err, os.ErrNotExist) {
		m.toast = snapshotLoadWarning(fmt.Errorf("%s: %w", path, err))
		return nil // Keep the current session and target snapshot untouched.
	}
	m.loadError = nil
	m.saveError = ""
	m.sessionFilePath = path
	m.sessionLabel = sess.ShortID
	m.store = nextStore
	m.selectedIndex = 0
	m.historyIndex = 0
	m.historyCardID = ""
	m.lastTurnProcessed = lastTurn
	m.pendingTurns = nil
	m.failedTurn = nil
	m.reconcileAttempts = 0
	m.reconciling = false
	m.isAgentBusy = false
	m.showSessionSwitcher = false
	m.showTrash = false
	m.confirmTrashPurge = false
	m.selectedTrashIdx = 0

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

	m.waitingForSession = false
	m.StartWatching(sess.Path)

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

func (m *Model) actionNewSession() tea.Cmd {
	if m.isDemo {
		m.toast = "Demo mode: session switching disabled"
		return nil
	}

	if m.loadError == nil && len(m.store.List()) > 0 {
		_ = m.saveSessionState()
	}

	m.StopWatching()
	m.store = card.NewStore()
	m.selectedIndex = 0
	m.historyIndex = 0
	m.historyCardID = ""
	m.lastTurnProcessed = 0
	m.pendingTurns = nil
	m.failedTurn = nil
	m.reconcileAttempts = 0
	m.reconciling = false
	m.isAgentBusy = false
	m.showSessionSwitcher = false
	m.showBlockPicker = false
	m.showTrash = false
	m.confirmTrashPurge = false
	m.selectedTrashIdx = 0
	m.sessionLabel = "waiting"
	m.sessionFilePath = ""
	m.searchMatches = nil
	m.waitingForSession = true
	m.waitingSince = time.Now().Add(-2 * time.Second)
	if peerCWD := multiplexer.DetectPeerCWD(); peerCWD != "" {
		m.peerCWD = peerCWD
	} else if m.peerCWD == "" {
		m.peerCWD, _ = os.Getwd()
	}
	if peerAgent := multiplexer.DetectPeerAgent(); peerAgent != "" {
		m.peerAgent = peerAgent
	}

	m.toast = "✓ Pane cleared. Waiting for conversation in adjacent terminal..."
	m.updateViewportContent()

	return tea.Batch(
		m.pollNewSessionCmd(),
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
		Render("Press 1-9 to yank instantly, Enter to copy, s to send, o to open URL/file")

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
			case card.BlockFile:
				badgeBg = ColorDanger
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
				b.WriteString(lipgloss.NewStyle().Background(ColorHighlight).Bold(true).Width(boxWidth-4).Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	}

	b.WriteString("\n")
	footer := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("[1-9] Quick Yank • [Enter] Yank • [s] Send to Peer • [o] Open URL/File • [Esc] Close")
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

	// Option 0: Start New Session
	isNewSelected := m.selectedSessionIdx == 0
	newPrefix := "  "
	if isNewSelected {
		newPrefix = "❯ "
	}
	newSessionLine := fmt.Sprintf("%s✨ %s  %s", newPrefix,
		lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Start New Session"),
		lipgloss.NewStyle().Foreground(ColorMuted).Render("(Clear pane & wait for prompt)"))
	if isNewSelected {
		b.WriteString(lipgloss.NewStyle().Background(ColorHighlight).Bold(true).Width(boxWidth-4).Render(newSessionLine) + "\n")
	} else {
		b.WriteString(newSessionLine + "\n")
	}
	divLen := boxWidth - 8
	if divLen < 10 {
		divLen = 10
	}
	b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("  " + strings.Repeat("─", divLen) + "\n"))

	if len(m.sessionList) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("  No prior conversation sessions found.\n"))
	} else {
		for i, s := range m.sessionList {
			isSelected := (i + 1) == m.selectedSessionIdx
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
				b.WriteString(lipgloss.NewStyle().Background(ColorHighlight).Bold(true).Width(boxWidth-4).Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	}

	b.WriteString("\n")
	footer := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("[j/k, ↑/↓] Navigate • [Enter] Select • [N] New Session • [Esc] Cancel")
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

func (m *Model) buildTrashModal() string {
	boxWidth := 72
	if m.width > 20 && m.width < boxWidth+4 {
		boxWidth = m.width - 4
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render("DROPPED CARDS") + "\n\n")
	drops := m.store.DroppedCards()
	for idx, c := range drops {
		line := fmt.Sprintf("  #%d  %s (v%d)", c.Number, c.Title, c.Version)
		if idx == m.selectedTrashIdx {
			line = "❯" + line[1:]
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n")
	if m.confirmTrashPurge && m.selectedTrashIdx < len(drops) {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render(
			fmt.Sprintf("Purge #%d from Syl trash? [y] Confirm  [n/Esc] Cancel", drops[m.selectedTrashIdx].Number)) + "\n")
		b.WriteString("Backups and filesystem history may still contain copies.")
	} else {
		b.WriteString("[j/k] Select • [Enter] Restore • [x] Purge • [Esc] Close")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorWarning).Background(lipgloss.Color("#1E222A")).
		Padding(1, 2).Width(boxWidth).Render(b.String())
}

func (m *Model) buildHelpModal() string {
	boxWidth := 76
	if m.width > 20 && m.width < 80 {
		boxWidth = m.width - 4
	}

	compact := m.height < 32

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render("✨ SYL COMMAND REFERENCE & SHORTCUTS")

	subtitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Actionable Command Badges • Press any shortcut key to trigger directly")

	secHeader := func(emoji, name string) string {
		return lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render(emoji + " " + name)
	}

	renderBadgeRow := func(key string, keyStyle lipgloss.Style, actionTitle, desc string) string {
		keyPill := keyStyle.Render(fmt.Sprintf("%-5s", key))
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ECEFF4"))
		titleStr := titleStyle.Render(fmt.Sprintf("%-16s", actionTitle))

		availDesc := boxWidth - lipgloss.Width(keyPill) - lipgloss.Width(titleStr) - 7
		if availDesc > 4 && len(desc) > availDesc {
			desc = desc[:availDesc-1] + "…"
		}
		descStr := lipgloss.NewStyle().Foreground(lipgloss.Color("#9AA4B2")).Render(desc)

		if boxWidth < 55 {
			return keyPill + " " + titleStr
		}
		return keyPill + " " + titleStr + " " + descStr
	}

	var b strings.Builder
	b.WriteString(title + "\n")
	b.WriteString(subtitle + "\n")
	if !compact {
		b.WriteString("\n")
	}

	// 1. Actions & Workflow
	b.WriteString(secHeader("🚀", "Workflow & Actions") + "\n")
	b.WriteString(renderBadgeRow("s", BadgeKeyPrimary, "Send to Surface", "Send [Ref: ...] reference to active cmux/tmux pane") + "\n")
	b.WriteString(renderBadgeRow("b", BadgeKeyPrimary, "Step/File Picker", "Surgical block picker (yank 1-9, run, open URL/file)") + "\n")
	b.WriteString(renderBadgeRow("v", BadgeKeyPrimary, "Edit in $EDITOR", "Open active card in nvim/vim and save edits back") + "\n")
	b.WriteString(renderBadgeRow("␣", BadgeKeyPrimary, "Check Task", "Toggle next task checkbox (- [ ] -> - [x])") + "\n")
	b.WriteString(renderBadgeRow("c", BadgeKeySuccess, "Copy Code", "Copy extracted code block(s) to clipboard") + "\n")
	b.WriteString(renderBadgeRow("C / y", BadgeKeySuccess, "Copy All", "Copy full card markdown content to clipboard") + "\n")
	b.WriteString(renderBadgeRow("u", BadgeKeySuccess, "Open URL", "Open primary card documentation link in browser") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	// 2. View & Display Modes
	b.WriteString(secHeader("👁", "View & Display Modes") + "\n")
	b.WriteString(renderBadgeRow("Tab", BadgeKeySecondary, "Focus / Split", "Toggle Focus View (hide/show turns column)") + "\n")
	b.WriteString(renderBadgeRow("w", BadgeKeySecondary, "Switch Pane", "Switch focus between card list and preview buffer") + "\n")
	b.WriteString(renderBadgeRow("za", BadgeKeySecondary, "Fold Code", "Collapse or expand fenced code blocks in card") + "\n")
	b.WriteString(renderBadgeRow("r", BadgeKeySecondary, "Raw / Rendered", "Toggle Raw text mode (for clean mouse select)") + "\n")
	b.WriteString(renderBadgeRow("h", BadgeKeySecondary, "Version History", "Preview prior content with ←/→; Enter restores as new version") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	// 3. Session & Card Management
	b.WriteString(secHeader("🗂", "Session & Organization") + "\n")
	b.WriteString(renderBadgeRow("N", BadgeKeyWarning, "New Session", "Clear reference pane and wait for conversation in adjacent terminal") + "\n")
	b.WriteString(renderBadgeRow("S", BadgeKeyWarning, "Switch Session", "Browse and switch conversation sessions (agy, claude, pi)") + "\n")
	b.WriteString(renderBadgeRow("/", BadgeKeyWarning, "Find Cards", "Search & filter reference cards interactively") + "\n")
	b.WriteString(renderBadgeRow("n / N", BadgeKeyWarning, "Next / Prev Match", "Jump to next or previous in-card search match") + "\n")
	b.WriteString(renderBadgeRow("R", BadgeKeyWarning, "Retry Capture", "Retry a failed card turn before later turns are processed") + "\n")
	b.WriteString(renderBadgeRow("p", BadgeKeyWarning, "Pin Card", "Pin / unpin card (protect from pruning)") + "\n")
	b.WriteString(renderBadgeRow("e", BadgeKeyWarning, "Export Cards", "Export session reference cards to markdown") + "\n")
	b.WriteString(renderBadgeRow("x / d", BadgeKeyWarning, "Drop Card", "Drop card (last 10 drops remain recoverable)") + "\n")
	b.WriteString(renderBadgeRow("U", BadgeKeyWarning, "Undo Drop", "Restore most recently dropped card, even after restart") + "\n")
	b.WriteString(renderBadgeRow("T", BadgeKeyWarning, "Trash", "Browse dropped cards; restore or confirm purge") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	// 4. Navigation & Control (Vim Motions)
	b.WriteString(secHeader("🧭", "Vim Navigation & Control") + "\n")
	b.WriteString(renderBadgeRow("j / k", BadgeKeyMuted, "Down / Up", "Scroll buffer (preview focused) or navigate cards") + "\n")
	b.WriteString(renderBadgeRow("Ctrl-e/y", BadgeKeyMuted, "Nudge Scroll", "Scroll preview buffer down or up 1 line") + "\n")
	b.WriteString(renderBadgeRow("Ctrl-d/u", BadgeKeyMuted, "Half Page", "Scroll preview buffer half page down or up") + "\n")
	b.WriteString(renderBadgeRow("gg / G", BadgeKeyMuted, "Top / Bottom", "Jump directly to top (gg) or bottom (G) of card") + "\n")
	b.WriteString(renderBadgeRow("? / F1", BadgeKeyMuted, "Help Palette", "Toggle this command reference glossary") + "\n")
	b.WriteString(renderBadgeRow("q / Esc", BadgeKeyMuted, "Close / Quit", "Close modal / Quit syl") + "\n")
	if !compact {
		b.WriteString("\n")
	}

	footer := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("Press any command key to run • [?], [Esc], [Enter], or click to close")
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
	runner, label, generation := m.runner, m.sessionLabel, m.generation
	return func() tea.Msg {
		if runner == nil || turn == nil {
			return nil
		}
		title, err := runner.SummarizeTurn(context.Background(), label, turn)
		if err != nil || title == "" {
			return nil
		}
		return TitleSummarizedMsg{CardID: cardID, Title: title, Turn: turn, Generation: generation}
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
	case "undo_drop":
		return m.actionUndoDrop()
	case "trash":
		m.actionOpenTrash()
	case "export":
		return m.actionExport()
	case "history":
		m.actionToggleHistory()
	case "send":
		return m.actionSend()
	case "block_picker":
		m.actionToggleBlockPicker()
	case "edit_card":
		return m.actionOpenInEditor()
	case "toggle_pane":
		m.actionTogglePaneFocus()
	case "toggle_fold":
		m.actionToggleFold()
	case "checklist":
		return m.actionToggleChecklist()
	case "search":
		m.searchMode = true
		m.searchQuery = ""
		m.searchSavedIdx = m.selectedIndex
	case "sessions":
		if !m.isDemo {
			sessions, _ := watcher.ListSessions(12)
			m.sessionList = sessions
			m.selectedSessionIdx = 0
			m.showSessionSwitcher = true
		}
	case "new_session":
		return m.actionNewSession()
	case "help":
		m.showHelp = !m.showHelp
	case "demo_next":
		if m.isDemo && m.demoStep < 3 {
			return m.advanceDemo()
		}
		m.toast = "Demo finished! All turns demonstrated."
	case "quit":
		if m.loadError != nil && len(m.store.List()) > 0 {
			m.toast = "Snapshot unreadable; export unsaved cards before quitting"
			return nil
		}
		if m.loadError == nil {
			if err := m.saveSessionState(); err != nil {
				return nil
			}
		}
		m.StopWatching()
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

	case EditorFinishedMsg:
		defer os.Remove(msg.TempFile)
		if msg.Generation != m.generation || m.loadError != nil {
			return m, nil
		}
		if msg.Err != nil {
			m.toast = fmt.Sprintf("Editor exited: %v", msg.Err)
		} else if data, err := os.ReadFile(msg.TempFile); err == nil {
			newContent := string(data)
			if c, ok := m.store.Get(msg.CardID); ok {
				if strings.TrimSpace(newContent) != strings.TrimSpace(c.Content) && strings.TrimSpace(newContent) != "" {
					m.store.Update(c.ID, newContent, "Edited in editor")
					if err := m.saveSessionState(); err == nil {
						m.toast = "✓ Card updated from editor"
					}
					m.updateViewportContentPreserve(true)
				} else {
					m.toast = "No changes made in editor"
				}
			} else {
				m.toast = "Edited card no longer exists"
			}
		} else {
			m.toast = fmt.Sprintf("Error reading edited card: %v", err)
		}
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return ClearToastMsg{}
		})

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case checkNewSessionMsg:
		if !m.waitingForSession {
			return m, nil
		}
		peerCWD := m.peerCWD
		waitingSince := m.waitingSince
		excludeID := ""
		return m, func() tea.Msg {
			sess, err := watcher.FindNewerSession(waitingSince, peerCWD, excludeID)
			if err == nil && sess != nil {
				return newSessionDetectedMsg{Session: *sess}
			}
			return pollAgainMsg{}
		}

	case pollAgainMsg:
		if m.waitingForSession {
			return m, m.pollNewSessionCmd()
		}
		return m, nil

	case newSessionDetectedMsg:
		if !m.waitingForSession {
			return m, nil
		}
		m.waitingForSession = false
		return m, m.actionSwitchSession(msg.Session)

	case agentActivityMsg:
		if msg.Generation != m.generation {
			return m, nil
		}
		m.isAgentBusy = msg.Activity.IsBusy
		m.agentTool = msg.Activity.Tool
		m.agentPrompt = msg.Activity.Prompt
		return m, m.listenForActivity()

	case watcher.AgentActivity: // Keep direct activity messages usable by embedders.
		m.isAgentBusy = msg.IsBusy
		m.agentTool = msg.Tool
		m.agentPrompt = msg.Prompt
		return m, m.listenForActivity()

	case TurnReceivedMsg:
		if msg.Generation != m.generation || m.loadError != nil {
			return m, nil
		}
		m.isAgentBusy = false
		if msg.Turn != nil && msg.Turn.ID <= m.lastTurnProcessed {
			// Turn was already processed in a saved snapshot.
			// Upgrade card title if a definitive title is now in cache (disabled in demo mode).
			if !m.isDemo {
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
			}
			return m, m.listenForTurns()
		}

		if m.reconciling || m.failedTurn != nil {
			m.pendingTurns = append(m.pendingTurns, msg.Turn)
			return m, m.listenForTurns()
		}
		m.reconciling = true
		return m, tea.Batch(m.applyReconcile(msg.Turn), m.listenForTurns())

	case MutationsAppliedMsg:
		if msg.Generation != m.generation || m.loadError != nil {
			return m, nil
		}
		m.failedTurn = nil
		m.reconcileAttempts = 0
		if msg.Turn != nil && msg.Turn.ID > m.lastTurnProcessed {
			m.lastTurnProcessed = msg.Turn.ID
		}

		for _, mut := range msg.Mutations {
			switch mut.Op {
			case reconciler.OpAdd:
				// 1. Check title cache so restarted syl retains all past AI titles (disabled in demo mode)
				if !m.isDemo && msg.Turn != nil {
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

				// 2. Queue AI summarization if not definitive in cache (disabled in demo mode)
				if !m.isDemo && msg.Turn != nil && m.runner != nil {
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
				if !m.isDemo && msg.Turn != nil && m.runner != nil {
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
		cmds = append(cmds, m.nextPendingTurn())
		return m, tea.Batch(cmds...)

	case TitleSummarizedMsg:
		if m.isDemo || msg.Generation != m.generation || m.loadError != nil {
			return m, nil
		}
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
		if msg.Generation != m.generation {
			return m, nil
		}
		m.reconciling = false
		if msg.Turn == nil {
			m.toast = fmt.Sprintf("Card capture failed: %v", msg.Err)
			return m, nil
		}
		m.failedTurn = msg.Turn
		m.reconcileAttempts++
		m.toast = fmt.Sprintf("Capture paused on turn %d: %v", msg.Turn.ID, msg.Err)
		// Never checkpoint later turns past this failure. Retry twice, then
		// keep the backlog until the user presses R or restarts Syl.
		if m.reconcileAttempts < 3 {
			generation, id, attempt := m.generation, msg.Turn.ID, m.reconcileAttempts
			delay := time.Duration(attempt) * time.Second
			return m, tea.Tick(delay, func(time.Time) tea.Msg {
				return retryReconcileMsg{TurnID: id, Attempt: attempt, Generation: generation}
			})
		}
		return m, nil

	case retryReconcileMsg:
		if msg.Generation != m.generation || m.failedTurn == nil || m.failedTurn.ID != msg.TurnID || msg.Attempt != m.reconcileAttempts {
			return m, nil
		}
		return m, m.retryFailedTurn()

	case ClearToastMsg:
		m.toast = m.saveError // Keep persistence errors visible.
		if m.loadError != nil {
			m.toast = snapshotLoadWarning(m.loadError)
		}
		return m, nil

	// Mouse Event Handling (Clicks, Scroll wheel, Drag)
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			if m.showTrash {
				m.showTrash = false
				m.confirmTrashPurge = false
				return m, nil
			}
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
				barLines := m.getActionBarLines()
				// 1. Check Action Bar clicks (Rows 1 .. barLines)
				if msg.Y >= 1 && msg.Y <= barLines {
					targetRow := msg.Y - 1
					for _, btn := range m.buttons {
						if btn.Row == targetRow && msg.X >= btn.X1 && msg.X <= btn.X2 {
							cmd := m.handleButtonAction(btn.Action)
							return m, cmd
						}
					}
				}

				// 2. In Split mode: Check Card List clicks
				cardListStartY := 1 + barLines
				if m.viewMode != ModeDetail {
					leftColWidth := m.getLeftColWidth()
					if msg.Y >= cardListStartY && msg.X <= leftColWidth {
						m.activePane = PaneCards
						cardIdx := m.cardListOffset + (msg.Y - cardListStartY)
						cards := m.store.List()
						if cardIdx >= 0 && cardIdx < len(cards) {
							if m.selectedIndex == cardIdx {
								m.actionToggleFocus()
							} else {
								m.selectedIndex = cardIdx
								m.updateViewportContent()
							}
						}
					} else if msg.Y >= cardListStartY && msg.X > leftColWidth {
						m.activePane = PanePreview
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
			case "?", "f1", "esc", "q", "enter", " ":
				m.showHelp = false
				return m, nil
			case "s":
				m.showHelp = false
				return m, m.actionSend()
			case "b":
				m.showHelp = false
				m.actionToggleBlockPicker()
				return m, nil
			case "c":
				m.showHelp = false
				return m, m.actionCopy()
			case "C", "y":
				m.showHelp = false
				return m, m.actionCopyAll()
			case "p":
				m.showHelp = false
				return m, m.actionPin()
			case "/":
				m.showHelp = false
				m.searchMode = true
				m.searchQuery = ""
				m.searchSavedIdx = m.selectedIndex
				return m, nil
			case "S":
				m.showHelp = false
				if !m.isDemo {
					sessions, _ := watcher.ListSessions(12)
					m.sessionList = sessions
					m.selectedSessionIdx = 0
					m.showSessionSwitcher = true
				}
				return m, nil
			case "N", "ctrl+n":
				m.showHelp = false
				if !m.isDemo {
					return m, m.actionNewSession()
				}
				return m, nil
			case "r":
				m.showHelp = false
				m.actionToggleRaw()
				return m, nil
			case "R":
				m.showHelp = false
				return m, m.retryFailedTurn()
			case "e":
				m.showHelp = false
				return m, m.actionExport()
			case "x", "d":
				m.showHelp = false
				return m, m.actionDrop()
			case "U":
				m.showHelp = false
				return m, m.actionUndoDrop()
			case "T":
				m.showHelp = false
				m.actionOpenTrash()
				return m, nil
			case "h":
				m.showHelp = false
				m.actionToggleHistory()
				return m, nil
			case "tab":
				m.showHelp = false
				m.actionToggleFocus()
				return m, nil
			case "u":
				m.showHelp = false
				return m, m.actionOpenURL()
			case "v":
				m.showHelp = false
				return m, m.actionOpenInEditor()
			case "w":
				m.showHelp = false
				m.actionTogglePaneFocus()
				return m, nil
			case "z", "Z":
				m.showHelp = false
				m.actionToggleFold()
				return m, nil
			case "j", "down", "right":
				m.showHelp = false
				cards := m.store.List()
				if m.selectedIndex < len(cards)-1 {
					m.selectedIndex++
					m.updateViewportContent()
				}
				return m, nil
			case "k", "up", "left":
				m.showHelp = false
				if m.selectedIndex > 0 {
					m.selectedIndex--
					m.updateViewportContent()
				}
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
				if m.selectedSessionIdx < len(m.sessionList) {
					m.selectedSessionIdx++
				}
				return m, nil
			case "n", "N":
				return m, m.actionNewSession()
			case "enter":
				if m.selectedSessionIdx == 0 {
					return m, m.actionNewSession()
				}
				targetIdx := m.selectedSessionIdx - 1
				if len(m.sessionList) > 0 && targetIdx < len(m.sessionList) {
					return m, m.actionSwitchSession(m.sessionList[targetIdx])
				}
				return m, nil
			default:
				return m, nil
			}
		}

		if m.showTrash {
			if m.confirmTrashPurge {
				switch msg.String() {
				case "y":
					return m, m.actionPurgeDropped(m.selectedTrashIdx)
				case "n", "esc":
					m.confirmTrashPurge = false
				}
				return m, nil
			}
			switch msg.String() {
			case "esc", "q", "T":
				m.showTrash = false
			case "up", "k":
				if m.selectedTrashIdx > 0 {
					m.selectedTrashIdx--
				}
			case "down", "j":
				if m.selectedTrashIdx < len(m.store.DroppedCards())-1 {
					m.selectedTrashIdx++
				}
			case "enter":
				return m, m.actionRestoreDropped(m.selectedTrashIdx)
			case "x", "d":
				m.confirmTrashPurge = true
			}
			return m, nil
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

		if m.viewMode == ModeHistory {
			switch msg.String() {
			case "h", "esc", "q":
				m.actionToggleHistory()
				return m, nil
			case "left", "[":
				if m.historyIndex > 0 {
					m.historyIndex--
					m.updateViewportContent()
				}
				return m, nil
			case "right", "]":
				cards := m.store.List()
				if m.selectedIndex < len(cards) && m.historyIndex < len(cards[m.selectedIndex].History)-1 {
					m.historyIndex++
					m.updateViewportContent()
				}
				return m, nil
			case "up", "k":
				m.viewport.LineUp(1)
				return m, nil
			case "down", "j":
				m.viewport.LineDown(1)
				return m, nil
			case "enter":
				return m, m.actionRestoreHistoryRevision()
			default:
				if msg.String() != "ctrl+c" {
					return m, nil
				}
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			if m.loadError != nil && len(m.store.List()) > 0 && msg.String() == "q" {
				m.toast = "Snapshot unreadable; export unsaved cards before quitting"
				return m, nil
			}
			if m.loadError == nil {
				if err := m.saveSessionState(); err != nil && msg.String() == "q" {
					return m, nil // Ctrl+C is still an explicit unsaved exit.
				}
			}
			m.StopWatching()
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
			sessions, _ := watcher.ListSessions(12)
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

		// Vim Mode & Pane Controls
		case "w", "ctrl+w":
			m.actionTogglePaneFocus()
			return m, nil

		case "v", "E":
			return m, m.actionOpenInEditor()

		case "z":
			m.lastZKeyTime = time.Now()
			return m, nil

		case "a":
			if time.Since(m.lastZKeyTime) < 600*time.Millisecond {
				m.lastZKeyTime = time.Time{}
				m.actionToggleFold()
				return m, nil
			}
			return m, nil

		case "Z":
			m.actionToggleFold()
			return m, nil

		case " ":
			if m.isDemo {
				cards := m.store.List()
				if len(cards) > 0 && m.selectedIndex < len(cards) {
					c := cards[m.selectedIndex]
					if strings.Contains(c.Content, "- [ ]") || strings.Contains(c.Content, "- [x]") {
						return m, m.actionToggleChecklist()
					}
				}
				if m.demoStep < 3 {
					return m, m.advanceDemo()
				}
				m.toast = "Demo finished! All 3 scenario turns demonstrated."
				return m, nil
			}
			return m, m.actionToggleChecklist()

		case "n":
			if len(m.searchMatches) > 0 {
				m.jumpSearchMatch(true)
				return m, nil
			}
			return m, nil

		case "N", "ctrl+n":
			if msg.String() == "N" && len(m.searchMatches) > 0 {
				m.jumpSearchMatch(false)
				return m, nil
			}
			if !m.isDemo {
				return m, m.actionNewSession()
			}
			return m, nil

		// Scrolling preview viewport content
		case "up":
			m.viewport.LineUp(2)
			return m, nil

		case "down":
			m.viewport.LineDown(2)
			return m, nil

		case "ctrl+e":
			m.viewport.LineDown(1)
			return m, nil

		case "ctrl+y":
			m.viewport.LineUp(1)
			return m, nil

		case "pgup", "ctrl+u":
			m.viewport.HalfViewUp()
			return m, nil

		case "pgdown", "ctrl+d":
			m.viewport.HalfViewDown()
			return m, nil

		case "ctrl+f":
			m.viewport.ViewDown()
			return m, nil

		case "ctrl+b":
			m.viewport.ViewUp()
			return m, nil

		case "{":
			m.viewport.LineUp(5)
			return m, nil

		case "}":
			m.viewport.LineDown(5)
			return m, nil

		case "g":
			if time.Since(m.lastGKeyTime) < 500*time.Millisecond {
				m.viewport.GotoTop()
				m.lastGKeyTime = time.Time{}
			} else {
				m.lastGKeyTime = time.Now()
			}
			return m, nil

		case "home":
			m.viewport.GotoTop()
			return m, nil

		case "end", "G":
			m.viewport.GotoBottom()
			return m, nil

		// Navigating reference turns / cards
		case "k", "left", "[":
			if m.viewMode == ModeDetail || m.activePane == PanePreview {
				m.viewport.LineUp(1)
			} else if m.selectedIndex > 0 {
				m.selectedIndex--
				m.updateViewportContent()
			}
			return m, nil

		case "j", "right", "]":
			if m.viewMode == ModeDetail || m.activePane == PanePreview {
				m.viewport.LineDown(1)
			} else {
				cards := m.store.List()
				if m.selectedIndex < len(cards)-1 {
					m.selectedIndex++
					m.updateViewportContent()
				}
			}
			return m, nil

		case "tab", "enter", "f", "t":
			m.actionToggleFocus()

		case "r":
			m.actionToggleRaw()

		case "R":
			return m, m.retryFailedTurn()

		case "U":
			return m, m.actionUndoDrop()

		case "T":
			m.actionOpenTrash()
			return m, nil

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
		}
	}

	// Update viewport with key/wheel events
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

type actionBadge struct {
	labelCompact string
	labelFull    string
	action       string
	style        lipgloss.Style
}

func (m *Model) getActionBarBadges() []actionBadge {
	var badges []actionBadge

	// 1. Focus / Split toggle
	if m.viewMode == ModeDetail {
		badges = append(badges, actionBadge{
			labelCompact: "Tab Split",
			labelFull:    "☰ Show Turns (Tab)",
			action:       "toggle_focus",
			style:        BtnHoverStyle,
		})
	} else {
		badges = append(badges, actionBadge{
			labelCompact: "Tab Focus",
			labelFull:    "👁 Hide Turns (Tab)",
			action:       "toggle_focus",
			style:        BtnStyle,
		})
	}

	// 1b. Pane Switch (w) in split view
	if m.viewMode == ModeSplit {
		paneLabelCompact := "w Pane"
		paneLabelFull := "⇋ Preview (w)"
		paneStyle := BtnStyle
		if m.activePane == PanePreview {
			paneLabelFull = "⇋ Cards (w)"
			paneStyle = BtnHoverStyle
		}
		badges = append(badges, actionBadge{
			labelCompact: paneLabelCompact,
			labelFull:    paneLabelFull,
			action:       "toggle_pane",
			style:        paneStyle,
		})
	}

	// 2. Steps (b)
	badges = append(badges, actionBadge{
		labelCompact: "b Steps",
		labelFull:    "📦 Steps (b)",
		action:       "block_picker",
		style:        BtnStyle,
	})

	// 3. Send (s) - Primary workflow command
	badges = append(badges, actionBadge{
		labelCompact: "s Send",
		labelFull:    "🚀 Send (s)",
		action:       "send",
		style:        BtnStyle,
	})

	// 4. Copy Code (c)
	badges = append(badges, actionBadge{
		labelCompact: "c Copy",
		labelFull:    "📋 Copy (c)",
		action:       "copy",
		style:        BtnStyle,
	})

	// 5. Pin (p)
	badges = append(badges, actionBadge{
		labelCompact: "p Pin",
		labelFull:    "📌 Pin (p)",
		action:       "pin",
		style:        BtnStyle,
	})

	// 6. Search (/)
	badges = append(badges, actionBadge{
		labelCompact: "/ Find",
		labelFull:    "🔍 Find (/)",
		action:       "search",
		style:        BtnStyle,
	})

	// 7. Sessions (S)
	if !m.isDemo {
		badges = append(badges, actionBadge{
			labelCompact: "S Switch",
			labelFull:    "🔄 Switch (S)",
			action:       "sessions",
			style:        BtnStyle,
		})
	}

	// 8. Raw toggle (r)
	if m.showRaw {
		badges = append(badges, actionBadge{
			labelCompact: "r View",
			labelFull:    "🎨 Rendered (r)",
			action:       "toggle_raw",
			style:        BtnHoverStyle,
		})
	} else {
		badges = append(badges, actionBadge{
			labelCompact: "r Raw",
			labelFull:    "📝 Raw (r)",
			action:       "toggle_raw",
			style:        BtnStyle,
		})
	}

	// 8b. Fold (za)
	foldLabelCompact := "za Fold"
	foldLabelFull := "▾ Fold (za)"
	foldStyle := BtnStyle
	if m.isFolded {
		foldLabelCompact = "za Expand"
		foldLabelFull = "▸ Expand (za)"
		foldStyle = BtnHoverStyle
	}
	badges = append(badges, actionBadge{
		labelCompact: foldLabelCompact,
		labelFull:    foldLabelFull,
		action:       "toggle_fold",
		style:        foldStyle,
	})

	// 8c. Edit in $EDITOR (v)
	badges = append(badges, actionBadge{
		labelCompact: "v Edit",
		labelFull:    "✏ Edit (v)",
		action:       "edit_card",
		style:        BtnStyle,
	})

	// 8d. Checklist toggle (Space)
	cards := m.store.List()
	if !m.isDemo && len(cards) > 0 && m.selectedIndex < len(cards) {
		if strings.Contains(cards[m.selectedIndex].Content, "- [ ]") || strings.Contains(cards[m.selectedIndex].Content, "- [x]") {
			badges = append(badges, actionBadge{
				labelCompact: "␣ Task",
				labelFull:    "✓ Task (␣)",
				action:       "checklist",
				style:        BtnStyle,
			})
		}
	}

	// 9. History (h)
	histStyle := BtnStyle
	if m.viewMode == ModeHistory {
		histStyle = BtnHoverStyle
	}
	badges = append(badges, actionBadge{
		labelCompact: "h Hist",
		labelFull:    "📜 History (h)",
		action:       "history",
		style:        histStyle,
	})

	// 10. Export (e)
	badges = append(badges, actionBadge{
		labelCompact: "e Export",
		labelFull:    "💾 Export (e)",
		action:       "export",
		style:        BtnStyle,
	})

	// 11. Drop (x), then offer undo while a dropped card exists.
	badges = append(badges, actionBadge{
		labelCompact: "x Drop",
		labelFull:    "🗑 Drop (x)",
		action:       "drop",
		style:        BtnStyle,
	})
	if m.store.CanUndoDrop() {
		badges = append(badges, actionBadge{
			labelCompact: "U Undo",
			labelFull:    "↶ Undo Drop (U)",
			action:       "undo_drop",
			style:        BtnHoverStyle,
		}, actionBadge{
			labelCompact: "T Trash",
			labelFull:    "🗑 Trash (T)",
			action:       "trash",
			style:        BtnStyle,
		})
	}

	// 12. Help / Command Palette (?)
	helpStyle := BtnStyle
	if m.showHelp {
		helpStyle = BtnHoverStyle
	}
	badges = append(badges, actionBadge{
		labelCompact: "? Help",
		labelFull:    "❓ Help (?)",
		action:       "help",
		style:        helpStyle,
	})

	// 13. Demo next (d or Space)
	if m.isDemo {
		badges = append(badges, actionBadge{
			labelCompact: "d Next",
			labelFull:    "⏩ Next Turn (d)",
			action:       "demo_next",
			style:        BtnHoverStyle,
		})
	}

	// 14. Quit (q)
	badges = append(badges, actionBadge{
		labelCompact: "q Quit",
		labelFull:    "✕ Quit (q)",
		action:       "quit",
		style:        BtnDangerStyle,
	})

	return badges
}

func (m *Model) getActionBarLines() int {
	w := m.width
	if w <= 0 {
		return 1
	}
	availWidth := w - 2
	if availWidth < 20 {
		availWidth = 20
	}

	badges := m.getActionBarBadges()
	useCompact := w < 130

	lines := 1
	currX := 1
	for _, b := range badges {
		label := b.labelFull
		if useCompact {
			label = b.labelCompact
		}
		btnW := lipgloss.Width(b.style.Render(label))
		if currX+btnW > availWidth && currX > 1 {
			lines++
			currX = 1
		}
		currX += btnW + 1
	}
	return lines
}

func (m *Model) renderActionBar() string {
	m.buttons = make([]Button, 0)
	w := m.width
	if w <= 0 {
		w = 80
	}
	availWidth := w - 2
	if availWidth < 20 {
		availWidth = 20
	}

	badges := m.getActionBarBadges()
	useCompact := w < 130

	var rows []string
	var curRow strings.Builder
	currX := 1
	rowIdx := 0

	for _, b := range badges {
		label := b.labelFull
		if useCompact {
			label = b.labelCompact
		}
		rendered := b.style.Render(label)
		btnW := lipgloss.Width(rendered)

		if currX+btnW > availWidth && currX > 1 {
			rows = append(rows, curRow.String())
			curRow.Reset()
			rowIdx++
			currX = 1
		}

		m.buttons = append(m.buttons, Button{
			Label:  label,
			Action: b.action,
			Row:    rowIdx,
			X1:     currX,
			X2:     currX + btnW,
		})

		curRow.WriteString(rendered)
		curRow.WriteString(" ")
		currX += btnW + 1
	}

	if curRow.Len() > 0 {
		rows = append(rows, curRow.String())
	}

	paddedRows := make([]string, len(rows))
	for i, r := range rows {
		paddedRows[i] = lipgloss.NewStyle().Padding(0, 1).Render(r)
	}

	return strings.Join(paddedRows, "\n")
}

func (m *Model) renderBaseView() string {
	cards := m.store.List()

	// 1. Header
	var statusBadge string
	if m.loadError != nil {
		label := "SNAPSHOT UNREADABLE · QUIT & REPAIR"
		if len(cards) > 0 {
			label = "SNAPSHOT UNREADABLE · EXPORT UNSAVED CARDS"
		}
		statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render(label)
	} else if m.saveError != "" {
		statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render("SAVE FAILED · Q TO RETRY")
	} else if m.failedTurn != nil && !m.reconciling {
		statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render("CAPTURE PAUSED · R TO RETRY")
	} else if m.waitingForSession {
		statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(m.spinner.View() + " WAITING FOR CONVERSATION")
	} else if m.isAgentBusy {
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
	if m.waitingForSession {
		sessionBadge = lipgloss.NewStyle().Foreground(ColorWarning).Render("[NEW]") + " "
	} else if m.sessionLabel != "" {
		sessionBadge = lipgloss.NewStyle().Foreground(ColorMuted).Render("["+m.sessionLabel+"]") + " "
	}

	focusBadge := ""
	if m.viewMode == ModeSplit {
		if m.activePane == PanePreview {
			focusBadge = " " + lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("[PREVIEW]")
		} else {
			focusBadge = " " + lipgloss.NewStyle().Foreground(ColorMuted).Render("[CARDS]")
		}
	} else if m.viewMode == ModeDetail {
		focusBadge = " " + lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("[FOCUS]")
	}

	headerLeft := HeaderStyle.Render("syl") + " " + sessionBadge + statusBadge + focusBadge
	var headerRight string
	if m.toast != "" {
		headerRight = ToastStyle.Render(m.toast)
	} else if m.waitingForSession {
		headerRight = FooterStyle.Render("Listening...")
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
		if m.waitingForSession {
			workspaceInfo := m.peerCWD
			if workspaceInfo == "" {
				workspaceInfo, _ = os.Getwd()
			}
			agentInfo := m.peerAgent
			if agentInfo == "" {
				agentInfo = "AI Assistant"
			} else {
				agentInfo = strings.ToUpper(agentInfo)
			}

			title := lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPrimary).
				Render(fmt.Sprintf("%s WAITING FOR CONVERSATION", m.spinner.View()))

			subtitle := lipgloss.NewStyle().
				Foreground(ColorMuted).
				Render("Syl is active and ready to stream reference cards from your adjacent terminal.")

			details := lipgloss.NewStyle().Render(fmt.Sprintf(
				"• Target Workspace: %s\n• Companion Agent:  %s\n• Live Status:      Listening for initial prompt...",
				lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(workspaceInfo),
				lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render(agentInfo),
			))

			hints := lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render(
				"💡 Ready when you are:\n   1. Type your prompt in the adjacent terminal to begin.\n   2. Reference cards will appear automatically as the assistant responds.\n   • Press [S] to browse and attach to an earlier session.\n   • Press [?] for keyboard shortcuts.",
			)

			emptyMsg = fmt.Sprintf("%s\n%s\n\n%s\n\n%s", title, subtitle, details, hints)
		} else if m.isAgentBusy {
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
	} else if m.viewMode == ModeHistory {
		hintText = "💡 ←/→: Prior/Next Revision • ↑/↓: Scroll • Enter: Restore Content as New Version • h/Esc: Close"
	} else if m.width < 90 {
		if m.viewMode == ModeDetail {
			hintText = "💡 Tab: Show • b: Steps • s: Send • c: Copy • /: Search • ?: Palette • q: Quit"
		} else {
			hintText = "💡 Tab: Hide • b: Steps • s: Send • c: Copy • /: Search • ?: Palette • q: Quit"
		}
	} else {
		if m.viewMode == ModeDetail {
			hintText = "💡 ↑/↓: Scroll • j/k: Turn • b: Steps • s: Send • c: Copy • /: Search • S: Sessions • ?: Palette"
		} else {
			hintText = "💡 ↑/↓: Scroll • j/k: Turn • Tab: Focus • b: Steps • s: Send • c: Copy • /: Search • S: Sessions • ?: Palette"
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
	if m.showTrash {
		return m.overlayModalOnBlurredBackdrop(base, m.buildTrashModal())
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
		Width(m.width-2).
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

	renderedLeft := leftStyle.Render(leftPane)
	renderedRight := rightStyle.Render(m.viewport.View())

	if m.viewMode == ModeSplit {
		if m.activePane == PaneCards {
			renderedRight = DimANSI(renderedRight)
		} else if m.activePane == PanePreview {
			renderedLeft = DimANSI(renderedLeft)
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, renderedLeft, renderedRight)
}
