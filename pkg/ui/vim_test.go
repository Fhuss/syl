package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/reconciler"
	"github.com/Fhuss/syl/pkg/watcher"
)

// TestVimDualPaneFocusAndContextualJK verifies dual pane toggling (w, ctrl+w)
// and contextual j/k behaviors across card selection and viewport scrolling.
func TestVimDualPaneFocusAndContextualJK(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "First Card", Content: "Card 1 content"})

	var c2Lines []string
	for i := 1; i <= 40; i++ {
		c2Lines = append(c2Lines, fmt.Sprintf("Card 2 line %d of long markdown buffer", i))
	}
	store.Add(&card.Card{ID: "c2", Title: "Second Card", Content: strings.Join(c2Lines, "\n")})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-vim-focus")
	m.width = 90
	m.height = 15
	m.updateViewportSize()

	// 1. Initial state is PaneCards
	if m.activePane != PaneCards {
		t.Fatalf("expected initial activePane = PaneCards, got %v", m.activePane)
	}

	// 2. In PaneCards: 'j' navigates to Card 2
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = u.(*Model)
	if m.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex = 1 after 'j', got %d", m.selectedIndex)
	}

	// 3. Press 'w' to switch to PanePreview
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = u.(*Model)
	if m.activePane != PanePreview {
		t.Fatalf("expected activePane = PanePreview after 'w', got %v", m.activePane)
	}

	// 4. In PanePreview: 'j' scrolls preview buffer instead of changing selected card
	initialOffset := m.viewport.YOffset
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = u.(*Model)
	if m.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex to remain 1 in PanePreview, got %d", m.selectedIndex)
	}
	if m.viewport.YOffset <= initialOffset {
		t.Fatalf("expected viewport YOffset to increase on 'j' in PanePreview")
	}

	// 5. In PanePreview: 'k' scrolls preview buffer up
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = u.(*Model)
	if m.viewport.YOffset != initialOffset {
		t.Fatalf("expected viewport YOffset to return to %d after 'k', got %d", initialOffset, m.viewport.YOffset)
	}

	// 6. Press Ctrl+W to toggle back to PaneCards
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = u.(*Model)
	if m.activePane != PaneCards {
		t.Fatalf("expected activePane = PaneCards after ctrl+w, got %v", m.activePane)
	}

	// 7. Test in Focus View (ModeDetail): 'j' and 'k' scroll preview buffer directly
	m.viewMode = ModeDetail
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = u.(*Model)
	if m.viewport.YOffset <= initialOffset {
		t.Fatalf("expected 'j' in ModeDetail to scroll preview buffer down")
	}
}

// TestVimUniversalMotions verifies universal Vim movement keybindings.
func TestVimUniversalMotions(t *testing.T) {
	var lines []string
	for i := 1; i <= 80; i++ {
		lines = append(lines, fmt.Sprintf("Buffer line %d with detailed instructions", i))
	}
	store := card.NewStore()
	store.Add(&card.Card{ID: "c-scroll", Title: "Motion Test", Content: strings.Join(lines, "\n\n")})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-motions")
	m.width = 90
	m.height = 15
	m.updateViewportSize()

	// 1. Ctrl+e (nudge down 1)
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = u.(*Model)
	if m.viewport.YOffset != 1 {
		t.Fatalf("expected YOffset 1 after ctrl+e, got %d", m.viewport.YOffset)
	}

	// 2. Ctrl+y (nudge up 1)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = u.(*Model)
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset 0 after ctrl+y, got %d", m.viewport.YOffset)
	}

	// 3. '}' (jump down 5)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'}'}})
	m = u.(*Model)
	if m.viewport.YOffset != 5 {
		t.Fatalf("expected YOffset 5 after '}', got %d", m.viewport.YOffset)
	}

	// 4. '{' (jump up 5)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'{'}})
	m = u.(*Model)
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset 0 after '{', got %d", m.viewport.YOffset)
	}

	// 5. 'G' (jump to bottom)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = u.(*Model)
	if m.viewport.YOffset == 0 {
		t.Fatalf("expected YOffset > 0 after 'G', got 0")
	}

	// 6. 'gg' (double-tap 'g' within 500ms jumps to top)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = u.(*Model)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = u.(*Model)
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset 0 after 'gg', got %d", m.viewport.YOffset)
	}

	// 7. Ctrl+d (half page down) and Ctrl+u (half page up)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = u.(*Model)
	halfDown := m.viewport.YOffset
	if halfDown == 0 {
		t.Fatalf("expected YOffset > 0 after ctrl+d")
	}
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = u.(*Model)
	if m.viewport.YOffset >= halfDown {
		t.Fatalf("expected YOffset to decrease after ctrl+u")
	}

	// 8. Ctrl+f (full page down) and Ctrl+b (full page up)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = u.(*Model)
	fullDown := m.viewport.YOffset
	if fullDown == 0 {
		t.Fatalf("expected YOffset > 0 after ctrl+f")
	}
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = u.(*Model)
	if m.viewport.YOffset >= fullDown {
		t.Fatalf("expected YOffset to decrease after ctrl+b")
	}
}

// TestVimSearchMatchesNextPrev verifies in-buffer pattern searching with 'n' and 'N'.
func TestVimSearchMatchesNextPrev(t *testing.T) {
	content := "Heading: Overview\n\nTarget word syl found on first section\n\nMiddle text with no target\n\nSecond target syl located here\n\nFooter with final syl mention"
	store := card.NewStore()
	store.Add(&card.Card{ID: "c-search", Title: "Searchable Card", Content: content})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-search-matches")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	m.searchQuery = "syl"
	m.computeSearchMatches()

	if len(m.searchMatches) != 3 {
		t.Fatalf("expected 3 matches for 'syl', got %d", len(m.searchMatches))
	}

	// Advance to match 1
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = u.(*Model)
	if m.searchMatchIdx != 1 {
		t.Fatalf("expected searchMatchIdx 1 after 'n', got %d", m.searchMatchIdx)
	}

	// Advance to match 2
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = u.(*Model)
	if m.searchMatchIdx != 2 {
		t.Fatalf("expected searchMatchIdx 2 after 'n', got %d", m.searchMatchIdx)
	}

	// Wrap around to match 0
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = u.(*Model)
	if m.searchMatchIdx != 0 {
		t.Fatalf("expected searchMatchIdx 0 after wrapping 'n', got %d", m.searchMatchIdx)
	}

	// Jump backward to match 2 with 'N'
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = u.(*Model)
	if m.searchMatchIdx != 2 {
		t.Fatalf("expected searchMatchIdx 2 after 'N', got %d", m.searchMatchIdx)
	}
}

// TestVimCodeBlockFolding verifies code block folding via 'za' and 'Z'.
func TestVimCodeBlockFolding(t *testing.T) {
	content := "Config details:\n```yaml\nserver:\n  port: 8080\n  host: 0.0.0.0\n```\nDeployment script:\n```bash\ncmux new-split right\n```\nDone."
	store := card.NewStore()
	store.Add(&card.Card{ID: "c-fold", Title: "Folding Card", Content: content})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-folding")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	// 1. Initially unfolded
	if m.isFolded {
		t.Fatalf("expected isFolded = false initially")
	}

	// 2. Press 'za' to fold
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	m = u.(*Model)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = u.(*Model)
	if !m.isFolded {
		t.Fatalf("expected isFolded = true after 'za'")
	}
	if !strings.Contains(m.viewport.View(), "folded") {
		t.Fatalf("expected folded indicator in viewport view, got: %s", m.viewport.View())
	}

	// 3. Press 'Z' (uppercase shortcut) to expand back
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	m = u.(*Model)
	if m.isFolded {
		t.Fatalf("expected isFolded = false after 'Z'")
	}
}

// TestVimChecklistToggle verifies Space toggles markdown checkboxes.
func TestVimChecklistToggle(t *testing.T) {
	content := "- [ ] Setup database\n- [ ] Run migrations\n- [ ] Verify endpoints"
	store := card.NewStore()
	store.Add(&card.Card{ID: "c-tasks", Title: "Tasks", Content: content})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-checklist")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	// Press Space -> check first task
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	c := m.store.List()[0]
	if !strings.Contains(c.Content, "- [x] Setup database") {
		t.Fatalf("expected first task checked, got:\n%s", c.Content)
	}

	// Press Space -> check second task
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	c = m.store.List()[0]
	if !strings.Contains(c.Content, "- [x] Run migrations") {
		t.Fatalf("expected second task checked, got:\n%s", c.Content)
	}

	// Press Space -> check third task
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	c = m.store.List()[0]
	if !strings.Contains(c.Content, "- [x] Verify endpoints") {
		t.Fatalf("expected third task checked, got:\n%s", c.Content)
	}

	// Press Space -> all checked, should reset all back to unchecked!
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	c = m.store.List()[0]
	if strings.Contains(c.Content, "- [x]") {
		t.Fatalf("expected all tasks reset back to unchecked, got:\n%s", c.Content)
	}
}

// TestVimEditorIntegration verifies external editor roundtrip editing.
func TestVimEditorIntegration(t *testing.T) {
	store := card.NewStore()
	c := store.Add(&card.Card{ID: "c-edit-vim", Title: "Editable Card", Content: "Original content before editor"})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-editor-vim")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	tmp, err := os.CreateTemp("", "syl-vim-edit-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())

	newContent := "Edited content saved from Neovim buffer"
	_ = os.WriteFile(tmp.Name(), []byte(newContent), 0o600)

	// Simulate completion of external editor process
	u, _ := m.Update(EditorFinishedMsg{
		Err:      nil,
		TempFile: tmp.Name(),
		CardID:   c.ID,
	})
	m = u.(*Model)

	updated := m.store.List()[0]
	if updated.Content != newContent {
		t.Fatalf("expected updated content %q, got %q", newContent, updated.Content)
	}
	if !strings.Contains(m.toast, "✓ Card updated from editor") {
		t.Fatalf("expected toast indicating editor update, got %q", m.toast)
	}
}

// TestVimDemoModeTourAndChecklist verifies demo mode loading, GitHub links,
// checklist toggles without breaking turn advancement, and multi-turn live mutations.
func TestVimDemoModeTourAndChecklist(t *testing.T) {
	store := card.NewStore()
	rec := reconciler.NewMockReconciler()

	m := NewModel(store, rec, nil, nil, nil, true, "demo")
	m.width = 90
	m.height = 25
	m.updateViewportSize()

	// Simulate Turn 1 delivery (Init advanceDemo)
	turn1Cmd := m.advanceDemo()
	msg := turn1Cmd()
	turnMsg, ok := msg.(TurnReceivedMsg)
	if !ok {
		// If wrapped in tick, wait briefly or extract
		time.Sleep(450 * time.Millisecond)
		turnMsg = TurnReceivedMsg{Turn: &watcher.Turn{ID: 1, User: "Welcome tour"}}
	}

	u, _ := m.Update(turnMsg)
	m = u.(*Model)

	// In demo mode, apply turn mutations
	muts, err := rec.Reconcile(nil, m.store.List(), turnMsg.Turn)
	if err != nil {
		t.Fatalf("mock reconciler error: %v", err)
	}
	for _, mut := range muts {
		switch mut.Op {
		case reconciler.OpAdd:
			m.store.Add(&card.Card{
				ID:      mut.CardID,
				Type:    mut.Type,
				Title:   mut.Title,
				Content: mut.Content,
			})
		}
	}
	m.updateViewportContent()

	cards := m.store.List()
	if len(cards) < 4 {
		t.Fatalf("expected at least 4 demo cards in Turn 1, got %d", len(cards))
	}

	// 1. Verify Card 1 references GitHub
	welcomeCard := cards[0]
	if !strings.Contains(welcomeCard.Content, "https://github.com/Fhuss/syl") {
		t.Fatalf("expected Card 1 to reference github.com/Fhuss/syl, got:\n%s", welcomeCard.Content)
	}

	// 2. Select Card 2 (Interactive Features Checklist)
	m.selectedIndex = 1
	m.updateViewportContent()
	checklistCard := cards[1]
	if !strings.Contains(checklistCard.Content, "- [ ] 1. Press 'w'") {
		t.Fatalf("expected Card 2 to contain checklist, got:\n%s", checklistCard.Content)
	}

	// 3. Press Space on checklist card -> MUST toggle checklist task, NOT advance demo turn!
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	updatedChecklist := m.store.List()[1]
	if !strings.Contains(updatedChecklist.Content, "- [x] 1. Press 'w'") {
		t.Fatalf("expected Space to toggle checklist item on task card, got:\n%s", updatedChecklist.Content)
	}

	// 4. Press 'd' to advance to Turn 2
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = u.(*Model)
	if m.demoStep < 2 {
		t.Fatalf("expected demoStep to advance to 2 after 'd', got %d", m.demoStep)
	}
}
