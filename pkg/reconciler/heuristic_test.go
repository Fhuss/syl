package reconciler

import (
	"context"
	"testing"

	"github.com/jakobhuss/syl/pkg/card"
	"github.com/jakobhuss/syl/pkg/watcher"
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
}
