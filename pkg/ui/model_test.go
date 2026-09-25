package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/harness"
	"github.com/Fhuss/syl/pkg/reconciler"
	"github.com/Fhuss/syl/pkg/watcher"
)

func TestMain(m *testing.M) {
	os.Setenv("SYL_MULTIPLEXER_DISABLED", "1")
	tmpDir, err := os.MkdirTemp("", "syl-test-*")
	if err == nil {
		os.Setenv("SYL_CONFIG_DIR", tmpDir)
		defer os.RemoveAll(tmpDir)
	}
	os.Exit(m.Run())
}

func TestModelDropAndPin(t *testing.T) {
	store := card.NewStore()
	c1 := store.Add(&card.Card{
		ID:      "card-1",
		Type:    card.TypeProcedure,
		Title:   "Step 1: Setup AWS Infrastructure",
		Content: "aws ec2 create-vpc",
	})
	c2 := store.Add(&card.Card{
		ID:      "card-2",
		Type:    card.TypeCommand,
		Title:   "Step 2: Deploy Container",
		Content: "docker run -d app",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 120
	m.height = 30

	// Select first card and drop it
	m.selectedIndex = 0
	_ = m.actionDrop()

	cards := store.List()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card after drop, got %d", len(cards))
	}
	if cards[0].ID != c2.ID {
		t.Fatalf("expected card-2 to remain, got %s", cards[0].ID)
	}

	// Pin remaining card
	_ = m.actionPin()
	if !cards[0].Pinned {
		t.Fatalf("expected remaining card to be pinned")
	}

	_ = c1
}

func TestModelKeyDrop(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:    "card-1",
		Type:  card.TypeProcedure,
		Title: "To be dropped",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30

	// Send "x" key to drop
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if len(store.List()) != 0 {
		t.Fatalf("expected card to be dropped via 'x' key")
	}
}

func TestModelLeftColWidth(t *testing.T) {
	m := &Model{}

	m.width = 60
	if w := m.getLeftColWidth(); w != 32 {
		t.Errorf("expected width 32 for screen width 60, got %d", w)
	}

	m.width = 90
	if w := m.getLeftColWidth(); w != 42 {
		t.Errorf("expected width 42 for screen width 90, got %d", w)
	}

	m.width = 120
	if w := m.getLeftColWidth(); w != 48 {
		t.Errorf("expected width 48 for screen width 120, got %d", w)
	}

	m.width = 180
	if w := m.getLeftColWidth(); w != 54 {
		t.Errorf("expected width 54 for screen width 180, got %d", w)
	}
}

func TestModelTitleSummarized(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:    "card-1",
		Type:  card.TypeProcedure,
		Title: "Old Raw Prompt Title",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30

	// Send TitleSummarizedMsg
	_, _ = m.Update(TitleSummarizedMsg{
		CardID: "card-1",
		Title:  "Succinct AI Topic",
	})

	cards := store.List()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	if cards[0].Title != "Succinct AI Topic" {
		t.Errorf("expected updated title 'Succinct AI Topic', got %q", cards[0].Title)
	}
}

func TestModelToggleFocus(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "card-1",
		Type:    card.TypeProcedure,
		Title:   "Step 1: Setup AWS Infrastructure",
		Content: "aws ec2 create-vpc\naws ec2 create-subnet",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 120
	m.height = 30
	m.updateViewportSize()

	if m.viewMode != ModeSplit {
		t.Fatalf("expected initial viewMode ModeSplit, got %v", m.viewMode)
	}
	expectedSplitWrap := 120 - m.getLeftColWidth() - 4
	if m.viewport.Width != expectedSplitWrap {
		t.Errorf("expected split viewport width %d, got %d", expectedSplitWrap, m.viewport.Width)
	}

	// Press Tab to toggle focus
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.viewMode != ModeDetail {
		t.Fatalf("expected viewMode ModeDetail after Tab, got %v", m.viewMode)
	}
	expectedDetailWrap := 120 - 4
	if m.viewport.Width != expectedDetailWrap {
		t.Errorf("expected detail viewport width %d, got %d", expectedDetailWrap, m.viewport.Width)
	}

	// Verify focus view banner contains FOCUS VIEW
	viewOutput := m.View()
	if !containsString(viewOutput, "FOCUS VIEW") {
		t.Errorf("expected View() output to contain 'FOCUS VIEW', got:\n%s", viewOutput)
	}

	// Press Tab again to return to split
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.viewMode != ModeSplit {
		t.Fatalf("expected viewMode ModeSplit after second Tab, got %v", m.viewMode)
	}
	if m.viewport.Width != expectedSplitWrap {
		t.Errorf("expected restored split viewport width %d, got %d", expectedSplitWrap, m.viewport.Width)
	}
}

func TestModelToggleRaw(t *testing.T) {
	store := card.NewStore()
	rawMarkdown := "### Title\n\n```bash\necho hello\n```"
	store.Add(&card.Card{
		ID:      "card-1",
		Type:    card.TypeCommand,
		Title:   "Echo Command",
		Content: rawMarkdown,
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30
	m.updateViewportSize()
	m.updateViewportContent()

	if m.showRaw {
		t.Fatalf("expected showRaw to be false initially")
	}

	// Press 'r' to toggle raw
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !m.showRaw {
		t.Fatalf("expected showRaw to be true after pressing 'r'")
	}
	if m.viewport.View() == "" {
		t.Errorf("expected non-empty viewport view in raw mode")
	}

	// Press 'r' again to return to rendered
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.showRaw {
		t.Fatalf("expected showRaw to be false after second 'r'")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && hasSubstr(s, substr)))
}

func hasSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestModelArrowScrollVsTurnNav(t *testing.T) {
	store := card.NewStore()
	longContent := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20"
	store.Add(&card.Card{
		ID:      "card-1",
		Type:    card.TypeProcedure,
		Title:   "Turn 1",
		Content: longContent,
	})
	store.Add(&card.Card{
		ID:      "card-2",
		Type:    card.TypeProcedure,
		Title:   "Turn 2",
		Content: "Second card content",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 10
	m.updateViewportSize()
	m.updateViewportContent()

	// Initial state: selected card is 0 (Turn 1), viewport YOffset is 0
	if m.selectedIndex != 0 {
		t.Fatalf("expected initial selectedIndex 0, got %d", m.selectedIndex)
	}
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected initial YOffset 0, got %d", m.viewport.YOffset)
	}

	// Press Down arrow: should scroll viewport down, NOT switch cards!
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selectedIndex != 0 {
		t.Errorf("Down arrow should NOT switch cards, got selectedIndex %d", m.selectedIndex)
	}
	if m.viewport.YOffset <= 0 {
		t.Errorf("Down arrow should scroll viewport down, got YOffset %d", m.viewport.YOffset)
	}

	// Press Up arrow: should scroll viewport back up
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selectedIndex != 0 {
		t.Errorf("Up arrow should NOT switch cards, got selectedIndex %d", m.selectedIndex)
	}

	// Press 'j': should switch to next card
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.selectedIndex != 1 {
		t.Errorf("'j' should switch to next card (index 1), got %d", m.selectedIndex)
	}

	// Press 'k': should switch back to previous card
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.selectedIndex != 0 {
		t.Errorf("'k' should switch to previous card (index 0), got %d", m.selectedIndex)
	}

	// Press Right arrow: should also switch to next card
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.selectedIndex != 1 {
		t.Errorf("Right arrow should switch to next card, got %d", m.selectedIndex)
	}

	// Press Left arrow: should also switch to previous card
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.selectedIndex != 0 {
		t.Errorf("Left arrow should switch to previous card, got %d", m.selectedIndex)
	}
}

func TestModelHelpModalToggle(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "card-1",
		Type:    card.TypeProcedure,
		Title:   "AWS Setup",
		Content: "aws configure",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30
	m.updateViewportSize()
	m.updateViewportContent()

	if m.showHelp {
		t.Fatalf("expected showHelp to be false initially")
	}

	// Press '?' to open help modal
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.showHelp {
		t.Fatalf("expected showHelp to be true after pressing '?'")
	}

	// Verify modal output contains key glossary terms
	modalView := m.View()
	if !containsString(modalView, "SYL COMMAND REFERENCE & SHORTCUTS") {
		t.Errorf("expected View() to render modal header, got:\n%s", modalView)
	}
	if !containsString(modalView, "Export session reference cards") {
		t.Errorf("expected View() to contain export shortcut description")
	}
	// Verify background is preserved (blurred backdrop, not blank screen)
	if !containsString(modalView, "test-session") {
		t.Errorf("expected View() to contain blurred background session label, got:\n%s", modalView)
	}

	// Press 'esc' to close modal
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp {
		t.Fatalf("expected showHelp to be false after pressing Esc")
	}

	// Re-open with '?' and close with mouse click
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.showHelp {
		t.Fatalf("expected showHelp to be true after second '?'")
	}

	_, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 10, Y: 10})
	if m.showHelp {
		t.Fatalf("expected showHelp to be false after mouse click")
	}
}

func TestModelActionExport(t *testing.T) {
	exportFile := "./syl-reference.md"
	_ = os.Remove(exportFile)
	defer os.Remove(exportFile)

	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "card-1",
		Type:    card.TypeProcedure,
		Title:   "Setup Kubernetes Cluster",
		Content: "```bash\nminikube start\n```",
	})
	store.Add(&card.Card{
		ID:      "card-2",
		Type:    card.TypeCommand,
		Title:   "Inspect Pods",
		Content: "```bash\nkubectl get pods -A\n```",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "export-test-session")
	m.width = 120
	m.height = 30

	// Trigger export via actionExport
	cmd := m.actionExport()
	if cmd == nil {
		t.Fatalf("expected actionExport to return tea.Cmd")
	}

	if !containsString(m.toast, "Exported 2 cards to ./syl-reference.md") {
		t.Errorf("expected toast to indicate successful export, got %q", m.toast)
	}

	// Verify file was written to disk
	data, err := os.ReadFile(exportFile)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	content := string(data)
	if !containsString(content, "export-test-session") {
		t.Errorf("expected exported file to contain session label")
	}
	if !containsString(content, "Setup Kubernetes Cluster") {
		t.Errorf("expected exported file to contain card-1 title")
	}
	if !containsString(content, "Inspect Pods") {
		t.Errorf("expected exported file to contain card-2 title")
	}

	// Test pressing 'e' key
	_ = os.Remove(exportFile)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if _, err := os.Stat(exportFile); err != nil {
		t.Errorf("expected 'e' key to trigger file export: %v", err)
	}
}

func TestModelSessionPersistence(t *testing.T) {
	sessionLabel := "test-persisted-session-123"
	snapPath, err := card.SessionFilePath(sessionLabel)
	if err != nil {
		t.Fatalf("failed to get session file path: %v", err)
	}
	_ = os.Remove(snapPath)
	defer os.Remove(snapPath)

	// Step 1: Create initial session with cards and saved state
	initialStore := card.NewStore()
	c1 := initialStore.Add(&card.Card{
		ID:      "card-1",
		Type:    card.TypeProcedure,
		Title:   "Original Title",
		Content: "step 1",
	})
	initialStore.UpdateTitle("card-1", "AI Persisted Short Title")
	initialStore.TogglePin("card-1")

	m1 := NewModel(initialStore, reconciler.NewMockReconciler(), nil, nil, nil, false, sessionLabel)
	m1.width = 100
	m1.height = 30
	m1.lastTurnProcessed = 8

	// Simulate user quitting (saves state to disk)
	m1.saveSessionState()

	if _, err := os.Stat(snapPath); err != nil {
		t.Fatalf("expected session file to be written to %s: %v", snapPath, err)
	}

	// Step 2: Simulate starting syl on a brand new process/run
	freshStore := card.NewStore()
	m2 := NewModel(freshStore, reconciler.NewMockReconciler(), nil, nil, nil, false, sessionLabel)
	m2.width = 120
	m2.height = 30
	m2.updateViewportSize()

	// Verify cards and titles were automatically restored from disk on startup
	restoredCards := m2.store.List()
	if len(restoredCards) != 1 {
		t.Fatalf("expected 1 card restored, got %d", len(restoredCards))
	}
	if restoredCards[0].Title != "AI Persisted Short Title" {
		t.Errorf("expected restored title 'AI Persisted Short Title', got %q", restoredCards[0].Title)
	}
	if !restoredCards[0].Pinned {
		t.Errorf("expected restored card to remain pinned")
	}
	if m2.lastTurnProcessed != 8 {
		t.Errorf("expected lastTurnProcessed to be 8, got %d", m2.lastTurnProcessed)
	}

	// Verify view immediately renders the persisted title without waiting for turns
	view := m2.View()
	if !containsString(view, "AI Persisted") {
		t.Errorf("expected View() to immediately render restored title, got:\n%s", view)
	}
	_ = c1
}

func TestModelPinPreservesSelection(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "Card 1"})
	store.Add(&card.Card{ID: "c2", Title: "Card 2"})
	store.Add(&card.Card{ID: "c3", Title: "Card 3"})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 120
	m.height = 30
	m.updateViewportSize()

	// Select Card 3 (index 2)
	m.selectedIndex = 2
	cards := store.List()
	if cards[m.selectedIndex].ID != "c3" {
		t.Fatalf("expected selected card to be c3, got %s", cards[m.selectedIndex].ID)
	}

	// Pin Card 3
	_ = m.actionPin()

	// Pinned card c3 is moved to index 0 by store.List(), and selectedIndex must update to 0 so the user is still on c3!
	cardsAfterPin := store.List()
	if cardsAfterPin[0].ID != "c3" {
		t.Fatalf("expected c3 to be at index 0 after pin, got %s", cardsAfterPin[0].ID)
	}
	if m.selectedIndex != 0 {
		t.Errorf("expected selectedIndex to update to 0 to track c3, got %d", m.selectedIndex)
	}
	if cardsAfterPin[m.selectedIndex].ID != "c3" {
		t.Errorf("expected selection to remain on c3, got %s", cardsAfterPin[m.selectedIndex].ID)
	}
}

func TestModelCardListWindowing(t *testing.T) {
	store := card.NewStore()
	for i := 1; i <= 30; i++ {
		store.Add(&card.Card{
			ID:      fmt.Sprintf("card-%d", i),
			Title:   fmt.Sprintf("Step %d Task", i),
			Content: fmt.Sprintf("content for card %d", i),
		})
	}

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 120
	m.height = 15 // Terminal height is 15 lines (viewport height ~ 10)
	m.updateViewportSize()

	// Navigate to card index 25 (Step 26)
	m.selectedIndex = 25
	viewOutput := m.renderCardsAndDetail(store.List())

	// Windowing must advance cardListOffset
	if m.cardListOffset == 0 {
		t.Errorf("expected cardListOffset > 0 when selectedIndex is 25, got %d", m.cardListOffset)
	}

	// View output must contain Step 26
	if !containsString(viewOutput, "Step 26 Task") {
		t.Errorf("expected windowed view output to contain selected 'Step 26 Task', got:\n%s", viewOutput)
	}

	// View output lines must not exceed viewport height
	lines := strings.Split(viewOutput, "\n")
	if len(lines) > m.height {
		t.Errorf("rendered cards and detail has %d lines, exceeding terminal height %d", len(lines), m.height)
	}
}

func TestModelActionSend(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "card-1",
		Title:   "AWS Deployment",
		Content: "aws ecs list-clusters",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30
	m.selectedIndex = 0

	cmd := m.actionSend()
	if cmd == nil {
		t.Fatalf("expected actionSend to return a tea.Cmd")
	}

	// Toast should be populated with either send message or clipboard fallback
	if m.toast == "" {
		t.Errorf("expected m.toast to be set after actionSend")
	}
}

func TestModelOpUpdatePreservesDefinitiveTitle(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "card-1",
		Title:   "Deploy Redis Cluster",
		Content: "helm install redis bitnami/redis",
	})

	mockH := harness.NewHeuristicHarness()
	m := NewModel(store, reconciler.NewMockReconciler(), mockH, nil, nil, false, "test-session")

	turn := &watcher.Turn{
		ID:        2,
		User:      "also change replica count to 3",
		Assistant: "Updated values.yaml",
	}

	mutMsg := MutationsAppliedMsg{
		Turn: turn,
		Mutations: []reconciler.Mutation{
			{
				Op:      reconciler.OpUpdate,
				CardID:  "card-1",
				Content: "helm upgrade redis bitnami/redis --set replicaCount=3",
				Reason:  "Adjusted configuration in Turn 2",
			},
		},
	}

	_, _ = m.Update(mutMsg)

	// Verify that the title is still "Deploy Redis Cluster" and not overwritten
	c, ok := store.Get("card-1")
	if !ok {
		t.Fatalf("card-1 not found")
	}
	if c.Title != "Deploy Redis Cluster" {
		t.Fatalf("expected title 'Deploy Redis Cluster' to be preserved, got %q", c.Title)
	}
}

func TestModelOpUpdateUpgradesNonDefinitiveTitle(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "card-1",
		Title:   "Procedure", // non-definitive title
		Content: "some code",
	})

	mockH := harness.NewHeuristicHarness()
	m := NewModel(store, reconciler.NewMockReconciler(), mockH, nil, nil, false, "test-session")

	turn := &watcher.Turn{
		ID:        2,
		User:      "install docker on ubuntu",
		Assistant: "sudo apt-get install docker.io",
	}

	mutMsg := MutationsAppliedMsg{
		Turn: turn,
		Mutations: []reconciler.Mutation{
			{
				Op:      reconciler.OpUpdate,
				CardID:  "card-1",
				Content: "sudo apt-get install docker.io",
				Reason:  "Turn 2",
			},
		},
	}

	_, cmd := m.Update(mutMsg)

	if cmd == nil {
		t.Fatalf("expected cmd to be returned for non-definitive title")
	}
}

func TestModelBlockPickerToggle(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "card-1",
		Title:   "Deployment Guide",
		Content: "1. Build container:\n```bash\ndocker build -t app .\n```\n2. Open https://example.com/status\n",
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30

	// Press 'b' to open block picker
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	model := updated.(*Model)

	if !model.showBlockPicker {
		t.Fatalf("expected showBlockPicker to be true after pressing 'b'")
	}
	if len(model.activeBlocks) == 0 {
		t.Fatalf("expected activeBlocks to be extracted")
	}

	// View should contain the block picker modal
	view := model.View()
	if !strings.Contains(view, "ACTIONABLE STEPS") {
		t.Fatalf("expected view to contain block picker modal, got:\n%s", view)
	}

	// Press '1' to copy first block
	updated2, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	model2 := updated2.(*Model)
	if model2.showBlockPicker {
		t.Fatalf("expected block picker to close after selecting key 1")
	}
}

func TestModelSearchMode(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "PostgreSQL Database", Content: "postgres config"})
	store.Add(&card.Card{ID: "c2", Title: "Redis Cache Cluster", Content: "redis config"})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30

	// Press '/' to start search
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = u.(*Model)
	if !m.searchMode {
		t.Fatalf("expected searchMode to be true")
	}

	// Type 'r', 'e', 'd'
	for _, r := range "red" {
		u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = u.(*Model)
	}

	// Should select card 2 (Redis)
	if m.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex to jump to 1 for 'red', got %d", m.selectedIndex)
	}

	// Press Enter to confirm search
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = u.(*Model)
	if m.searchMode {
		t.Fatalf("expected searchMode to be false after Enter")
	}
	if m.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex to remain 1, got %d", m.selectedIndex)
	}
}

func TestModelSessionSwitcherToggle(t *testing.T) {
	store := card.NewStore()
	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 100
	m.height = 30

	// Press 'S' to open session switcher
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = u.(*Model)

	// If sessions found on this machine, modal opens
	if m.showSessionSwitcher {
		view := m.View()
		if !strings.Contains(view, "SWITCH") {
			t.Fatalf("expected view to contain session switcher modal")
		}
		// Press 'Esc' to close
		u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = u.(*Model)
		if m.showSessionSwitcher {
			t.Fatalf("expected session switcher to close on Esc")
		}
	}
}


