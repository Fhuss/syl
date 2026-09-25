package reconciler

import (
	"context"
	"testing"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/watcher"
)

func TestHeuristicReconciler(t *testing.T) {
	h := NewHeuristicReconciler()
	ctx := context.Background()

	// Turn with code block
	turn1 := &watcher.Turn{
		ID:        1,
		User:      "How do I build the binary?",
		Assistant: "Run the following command:\n```bash\ngo build -o bin/syl ./cmd/syl\n```",
	}

	muts1, err := h.Reconcile(ctx, nil, turn1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(muts1) != 1 || muts1[0].Op != OpAdd {
		t.Fatalf("expected 1 ADD op, got %v", muts1)
	}

	// Turn correcting previous step
	existingCards := []*card.Card{
		{
			ID:      muts1[0].CardID,
			Title:   muts1[0].Title,
			Content: muts1[0].Content,
		},
	}
	turn2 := &watcher.Turn{
		ID:        2,
		User:      "Step failed with error, can we fix it?",
		Assistant: "Here is the fixed build:\n```bash\ngo build -v -o bin/syl ./cmd/syl\n```",
	}

	muts2, err := h.Reconcile(ctx, existingCards, turn2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(muts2) != 1 || muts2[0].Op != OpUpdate {
		t.Fatalf("expected 1 UPDATE op, got %v", muts2)
	}

	// Turn with pure conversational chatter -> OpNoop
	turnChatter := &watcher.Turn{
		ID:        3,
		User:      "Are we still doing the prompt title summary?",
		Assistant: "Yes, the prompt title summary is active and running in the background.",
	}
	mutsChatter, err := h.Reconcile(ctx, existingCards, turnChatter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mutsChatter) != 1 || mutsChatter[0].Op != OpNoop {
		t.Fatalf("expected OpNoop for chatter, got %v", mutsChatter)
	}

	// Turn invalidating a tool
	turnInvalid := &watcher.Turn{
		ID:        4,
		User:      "Don't use go build directly, that is deprecated now",
		Assistant: "Understood, avoid using direct go build.",
	}
	mutsInvalid, err := h.Reconcile(ctx, existingCards, turnInvalid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mutsInvalid) != 1 || mutsInvalid[0].Op != OpInvalidate {
		t.Fatalf("expected OpInvalidate, got %v", mutsInvalid)
	}
}

