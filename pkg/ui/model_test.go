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

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, false, "test-session")
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

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, false, "test-session")
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
