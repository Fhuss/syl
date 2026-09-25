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

func TestHeuristicNoSubstringFalsePositive(t *testing.T) {
	h := NewHeuristicReconciler()
	ctx := context.Background()

	// Existing Card 1 is about Go build commands
	existingCards := []*card.Card{
		{
			ID:      "card-1",
			Title:   "Build Syl Binary",
			Content: "```bash\ngo build -o bin/syl ./cmd/syl\n```",
		},
	}

	// Turn 2 contains the word "algorithm" (which has substring "go", but is NOT the tool "go")
	turnAlgorithm := &watcher.Turn{
		ID:        2,
		User:      "What algorithm should we use for sorting?",
		Assistant: "Here is the quicksort procedure:\n1. Choose a pivot\n2. Partition elements",
	}

	muts, err := h.Reconcile(ctx, existingCards, turnAlgorithm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(muts) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(muts))
	}
	// Must be OpAdd (new card), NEVER OpUpdate (which would overwrite card-1)
	if muts[0].Op != OpAdd {
		t.Errorf("expected OpAdd for unrelated 'algorithm' prompt, but got %v (falsely matched substring 'go')", muts[0].Op)
	}

	// Turn 3 contains "flaws" (which has substring "aws")
	awsCard := []*card.Card{
		{
			ID:      "card-aws",
			Title:   "Deploy to AWS ECS",
			Content: "```bash\naws ecs create-cluster --cluster-name prod\n```",
		},
	}
	turnFlaws := &watcher.Turn{
		ID:        3,
		User:      "Explain the flaws of this architectural approach",
		Assistant: "The main trade-offs are:\n1. Increased latency\n2. Eventual consistency challenges",
	}
	mutsAWS, err := h.Reconcile(ctx, awsCard, turnFlaws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mutsAWS) != 1 || mutsAWS[0].Op != OpAdd {
		t.Errorf("expected OpAdd for 'flaws', got %v (falsely matched substring 'aws')", mutsAWS)
	}
}

func TestHeuristicIndependentTurnsInSameLanguage(t *testing.T) {
	h := NewHeuristicReconciler()
	ctx := context.Background()

	// Existing Card 1 is a Go HTTP server
	card1 := &card.Card{
		ID:      "card-1",
		Title:   "Go HTTP Server",
		Content: "```go\nhttp.ListenAndServe(\":8080\", nil)\n```",
	}

	// Turn 2 is an independent question in Go (JSON unmarshaling), NOT a correction
	turn2 := &watcher.Turn{
		ID:        2,
		User:      "How do I unmarshal JSON into a struct in Go?",
		Assistant: "Use the json.Unmarshal function:\n```go\nerr := json.Unmarshal(data, &user)\n```",
	}

	muts, err := h.Reconcile(ctx, []*card.Card{card1}, turn2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(muts) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(muts))
	}
	// Must be OpAdd — the HTTP server reference card must NOT be destroyed!
	if muts[0].Op != OpAdd {
		t.Errorf("expected OpAdd for independent Go question, got %v (overwrote existing card)", muts[0].Op)
	}
}

func TestHeuristicCardIDNoCollisionOnRestart(t *testing.T) {
	// Simulate starting a fresh HeuristicReconciler on a session that already has 2 saved cards: card-1 and card-2
	freshReconciler := NewHeuristicReconciler()
	ctx := context.Background()

	currentCards := []*card.Card{
		{
			ID:      "card-1",
			Title:   "First Restored Card",
			Content: "```bash\necho step 1\n```",
		},
		{
			ID:      "card-2",
			Title:   "Second Restored Card",
			Content: "```bash\necho step 2\n```",
		},
	}

	newTurn := &watcher.Turn{
		ID:        3,
		User:      "How do I install postgres?",
		Assistant: "Run:\n```bash\nbrew install postgresql\n```",
	}

	muts, err := freshReconciler.Reconcile(ctx, currentCards, newTurn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(muts) != 1 || muts[0].Op != OpAdd {
		t.Fatalf("expected 1 OpAdd mutation, got %v", muts)
	}

	// Must NOT collide with card-1 or card-2!
	if muts[0].CardID == "card-1" || muts[0].CardID == "card-2" {
		t.Errorf("expected fresh reconciler to generate non-colliding ID, but got %s", muts[0].CardID)
	}
	if muts[0].CardID != "card-3" {
		t.Errorf("expected CardID 'card-3', got %s", muts[0].CardID)
	}
}

