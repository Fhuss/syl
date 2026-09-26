package reconciler

import (
	"context"
	"strings"
	"testing"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/harness"
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

func TestHeuristicCapturesSubstantiveRecommendationWithoutCode(t *testing.T) {
	h := NewHeuristicReconciler()
	turn := &watcher.Turn{
		ID:   1,
		User: "Tell me about the project itself, what shouold be the main USP of using it?",
		Assistant: `Syl’s main USP should be **a living, actionable reference beside your AI chat**.

A chat transcript preserves everything you said. Syl should preserve **what you can still use**: the current procedure, command, or decision—updated when the conversation corrects it, and ready to copy or send back with a keypress.

> **Syl turns long AI conversations into a live reference pane, so the useful steps don’t get buried or go stale.**

I’d lead with **in-place updates to instructions** as the differentiator.`,
	}
	muts, err := h.Reconcile(context.Background(), nil, turn)
	if err != nil || len(muts) != 1 || muts[0].Op != OpAdd || muts[0].Type != card.TypeDecision {
		t.Fatalf("expected a decision card, got %+v, %v", muts, err)
	}
	if muts[0].Content != turn.Assistant {
		t.Fatalf("decision content should preserve the recommendation, got %q", muts[0].Content)
	}

	// A new product decision must not overwrite an unrelated command card.
	existing := []*card.Card{{ID: "build", Type: card.TypeCommand, Title: "Build Syl", Content: "```bash\ngo build ./cmd/syl\n```"}}
	muts, err = h.Reconcile(context.Background(), existing, turn)
	if err != nil || len(muts) != 1 || muts[0].Op != OpAdd {
		t.Fatalf("expected a new card beside the build command, got %+v, %v", muts, err)
	}
}

func TestHeuristicSkipsConversationalAndVagueRecommendations(t *testing.T) {
	tests := []watcher.Turn{
		{User: "yes it did", Assistant: "Great—session selection and card capture are both working."},
		{User: "Should I keep going?", Assistant: "Yes, go ahead."},
		{User: "What should I do next?", Assistant: strings.Repeat("Here is some general background on the problem. ", 6)},
		{User: "This didn't produce a card either", Assistant: strings.Repeat("The main issue is that no reusable information was added. ", 5)},
	}
	for _, turn := range tests {
		muts, err := NewHeuristicReconciler().Reconcile(context.Background(), nil, &turn)
		if err != nil || len(muts) != 1 || muts[0].Op != OpNoop {
			t.Errorf("expected NOOP for %q, got %+v, %v", turn.User, muts, err)
		}
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

func TestHeuristicUnrelatedCorrectionDoesNotOverwriteCard(t *testing.T) {
	existing := []*card.Card{{ID: "deployment", Title: "Production deployment", Content: "```bash\naws deploy prod\n```"}}
	turn := &watcher.Turn{ID: 2, User: "Can you fix a README typo?", Assistant: "Use this documentation edit:\n```bash\nsed -i '' s/teh/the/ README.md\n```"}
	muts, err := NewHeuristicReconciler().Reconcile(context.Background(), existing, turn)
	if err != nil || len(muts) != 1 || muts[0].Op != OpAdd {
		t.Fatalf("unrelated correction must create its own card, got %+v, %v", muts, err)
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

func TestHeuristicReconcilerRefTagAndElaboration(t *testing.T) {
	h := NewHeuristicReconciler()
	ctx := context.Background()

	card1 := &card.Card{
		ID:      "card-1",
		Title:   "Switching Terminal Panes",
		Content: "Initial terminal instructions...",
		Version: 1,
	}
	existing := []*card.Card{card1}

	turn := &watcher.Turn{
		ID:   2,
		User: "[Ref: Switching Terminal Panes (v1)] can you elaborate a bit more",
		Assistant: `Based on **[Ref: Switching Terminal Panes (v1)]**, here is an in-depth breakdown:

### Step 1: Navigate Adjacent Panes via Keyboard
* Next Pane: Cmd + Option + Arrow

### Step 2: Switch Panes Programmatically via cmux CLI
` + "```bash\ncmux list-panels\n```",
	}

	muts, err := h.Reconcile(ctx, existing, turn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(muts) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(muts))
	}
	if muts[0].Op != OpUpdate {
		t.Fatalf("expected OpUpdate, got %v", muts[0].Op)
	}
	if muts[0].CardID != "card-1" {
		t.Errorf("expected CardID card-1, got %s", muts[0].CardID)
	}
	// Verify that the content appends the previous content
	if !strings.Contains(muts[0].Content, "Initial terminal instructions...") {
		t.Errorf("expected updated content to preserve and append to initial content, got %s", muts[0].Content)
	}
	if !strings.Contains(muts[0].Content, "### Step 1: Navigate Adjacent Panes via Keyboard") {
		t.Errorf("expected updated content to include step heading, got %s", muts[0].Content)
	}
}

func TestHeuristicReconcilerSameTitleMatching(t *testing.T) {
	h := NewHeuristicReconciler()
	ctx := context.Background()

	card1 := &card.Card{
		ID:      "card-1",
		Title:   "AWS Deployment Guide",
		Content: "```bash\necho aws v1\n```",
		Version: 1,
	}
	existing := []*card.Card{card1}

	// Turn has prompt that triggers the same title
	harness.GetTitleCache().SetByPrompt("Can you give more details on the deploy guide?", "AWS Deployment Guide")

	turn := &watcher.Turn{
		ID:        2,
		User:      "Can you give more details on the deploy guide?",
		Assistant: "```bash\necho aws v2\n```",
	}

	muts, err := h.Reconcile(ctx, existing, turn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(muts) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(muts))
	}
	if muts[0].Op != OpUpdate {
		t.Fatalf("expected OpUpdate for same-title card, got %v", muts[0].Op)
	}
	if muts[0].CardID != "card-1" {
		t.Errorf("expected CardID card-1, got %s", muts[0].CardID)
	}
}
