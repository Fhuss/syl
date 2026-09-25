package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/reconciler"
)

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
