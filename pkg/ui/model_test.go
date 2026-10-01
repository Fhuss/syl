package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/harness"
	"github.com/Fhuss/syl/pkg/reconciler"
	"github.com/Fhuss/syl/pkg/watcher"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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

func TestUnreadableSnapshotCannotBeOverwritten(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	path, err := card.SessionFilePath("unreadable-start")
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"session_id":"unreadable-start","cards":[{"id":"important","content":"recoverable"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "unreadable-start")
	if m.loadError == nil || !strings.Contains(m.toast, "Snapshot unreadable") {
		t.Fatal("failed snapshot load must be visible")
	}
	m.width, m.height = 100, 30
	if !strings.Contains(m.View(), "SNAPSHOT UNREADABLE") {
		t.Fatal("status must show an unreadable snapshot warning")
	}
	transcript := filepath.Join(t.TempDir(), "turns.jsonl")
	if err := os.WriteFile(transcript, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m.StartWatching(transcript)
	if m.turnCh != nil {
		t.Fatal("watcher must not process turns with a corrupt snapshot")
	}
	m.Update(MutationsAppliedMsg{Turn: &watcher.Turn{ID: 1}, Mutations: []reconciler.Mutation{{Op: reconciler.OpAdd, CardID: "new", Content: "new"}}})
	if len(m.store.List()) != 0 || m.lastTurnProcessed != 0 {
		t.Fatal("corrupt snapshot must not accept mutations")
	}
	if err := m.saveSessionState(); err == nil {
		t.Fatal("corrupt snapshot should block saving")
	}
	m.Update(ClearToastMsg{})
	if !strings.Contains(m.toast, "Original preserved") {
		t.Fatal("load failure warning should remain visible")
	}
	if current, err := os.ReadFile(path); err != nil || string(current) != string(original) {
		t.Fatalf("corrupt snapshot was changed: %q, %v", current, err)
	}
	_, quit := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quit == nil {
		t.Fatal("q should allow quitting an unreadable, unmodified session")
	}
}

func TestUnreadableSnapshotBlocksPrepopulatedStoreAndSwitch(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	path, err := card.SessionFilePath("unreadable-prepopulated")
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"session_id":"unreadable-prepopulated","cards":[null]}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store := card.NewStore()
	store.Add(&card.Card{ID: "caller-card", Content: "caller"})
	m := NewModel(store, reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "unreadable-prepopulated")
	if m.loadError == nil || m.saveSessionState() == nil {
		t.Fatal("caller-supplied cards must not overwrite corrupt snapshot")
	}
	m.actionDrop()
	if len(store.List()) != 1 {
		t.Fatal("mutating actions must be blocked on corrupt snapshot")
	}
	_, quit := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quit != nil {
		t.Fatal("q must not discard caller-supplied unsaved cards")
	}
	currentPath := filepath.Join(t.TempDir(), "current.jsonl")
	if err := os.WriteFile(currentPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m.actionSwitchSession(watcher.SessionInfo{ShortID: "healthy", Path: currentPath})
	if m.sessionLabel != "unreadable-prepopulated" || len(m.store.List()) != 1 {
		t.Fatal("switching discarded caller-supplied unsaved cards")
	}
	if current, err := os.ReadFile(path); err != nil || string(current) != string(original) {
		t.Fatalf("corrupt snapshot was overwritten: %q, %v", current, err)
	}
}

func TestSwitchAwayFromUnreadableSnapshot(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	path, err := card.SessionFilePath("bad-target")
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"session_id":"bad-target","cards":[null]}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "bad-target")
	if m.loadError == nil {
		t.Fatal("expected unreadable initial snapshot")
	}
	transcript := filepath.Join(t.TempDir(), "current.jsonl")
	if err := os.WriteFile(transcript, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m.actionSwitchSession(watcher.SessionInfo{ShortID: "healthy", Path: transcript})
	defer m.StopWatching()
	if m.loadError != nil || m.sessionLabel != "healthy" {
		t.Fatal("could not switch to a healthy session")
	}
	m.store.Add(&card.Card{ID: "healthy-card", Content: "keep"})
	m.actionSwitchSession(watcher.SessionInfo{ShortID: "bad-target", Path: transcript})
	if m.sessionLabel != "healthy" || len(m.store.List()) != 1 || m.store.List()[0].ID != "healthy-card" {
		t.Fatal("switching to a corrupt snapshot discarded the healthy session")
	}
	if current, err := os.ReadFile(path); err != nil || string(current) != string(original) {
		t.Fatalf("corrupt snapshot was overwritten: %q, %v", current, err)
	}
}

func TestSwitchSessionIgnoresOldWatcherEvents(t *testing.T) {
	oldPath := filepath.Join(t.TempDir(), "old.jsonl")
	newPath := filepath.Join(t.TempDir(), "new.jsonl")
	for _, path := range []string{oldPath, newPath} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "old")
	m.StartWatching(oldPath)
	oldCtx, oldGeneration := m.watchCtx, m.generation
	m.actionSwitchSession(watcher.SessionInfo{Path: newPath, ShortID: "new", Agent: "pi"})
	defer m.StopWatching()
	if oldCtx.Err() == nil || m.generation == oldGeneration {
		t.Fatal("old watcher was not canceled on session switch")
	}
	oldTurn := &watcher.Turn{ID: 1, User: "old prompt", Assistant: "```bash\necho old\n```"}
	m.Update(TurnReceivedMsg{Turn: oldTurn, Generation: oldGeneration})
	m.Update(MutationsAppliedMsg{Turn: oldTurn, Generation: oldGeneration, Mutations: []reconciler.Mutation{{Op: reconciler.OpAdd, CardID: "old", Content: "old"}}})
	if len(m.store.List()) != 0 || m.lastTurnProcessed != 0 {
		t.Fatalf("old session changed new store: %+v", m.store.List())
	}
	newCard := m.store.Add(&card.Card{ID: "same-id", Content: "new session content"})
	temp := filepath.Join(t.TempDir(), "old-editor.md")
	if err := os.WriteFile(temp, []byte("old session edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Update(EditorFinishedMsg{TempFile: temp, CardID: newCard.ID, Generation: oldGeneration})
	if newCard.Content != "new session content" {
		t.Fatal("stale editor completion changed a card in the new session")
	}
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Fatalf("stale editor file was not cleaned up: %v", err)
	}
}

func TestReconciliationQueuesTurnsInOrder(t *testing.T) {
	m := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "queue")
	defer m.StopWatching()
	first := &watcher.Turn{ID: 1, User: "first", Assistant: "```bash\necho first\n```"}
	second := &watcher.Turn{ID: 2, User: "second", Assistant: "```bash\necho second\n```"}
	m.reconciling = true
	m.Update(TurnReceivedMsg{Turn: second})
	if len(m.pendingTurns) != 1 {
		t.Fatal("second turn should wait for first reconciliation")
	}
	m.Update(MutationsAppliedMsg{Turn: first, Mutations: []reconciler.Mutation{{Op: reconciler.OpAdd, CardID: "first", Content: "first"}}})
	if len(m.pendingTurns) != 0 || !m.reconciling || m.lastTurnProcessed != 1 {
		t.Fatal("queued second turn was not scheduled after first")
	}
	m.Update(MutationsAppliedMsg{Turn: second, Mutations: []reconciler.Mutation{{Op: reconciler.OpAdd, CardID: "second", Content: "second"}}})
	if len(m.store.List()) != 2 || m.lastTurnProcessed != 2 || m.reconciling {
		t.Fatalf("turns not applied in order: %+v", m.store.List())
	}
}

func TestSnapshotFailureIsVisibleAndRecoverable(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "saved-card", Content: "important"})
	m := NewModel(store, reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "snapshot-failure")
	m.width, m.height = 100, 30
	m.updateViewportSize()
	validPath := m.sessionFilePath
	m.sessionFilePath = t.TempDir() // Renaming a file onto a directory must fail.
	m.actionPin()
	if m.saveError == "" || !strings.Contains(m.toast, "Save failed") || strings.Contains(m.toast, "Pinned card") {
		t.Fatalf("save failure was hidden: badge=%q toast=%q", m.saveError, m.toast)
	}
	if !strings.Contains(m.View(), "SAVE FAILED") {
		t.Fatal("save failure must remain visible in the status badge")
	}
	m.Update(ClearToastMsg{})
	if m.toast != m.saveError {
		t.Fatal("toast timer cleared persistent save warning")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("q must not discard unsaved edits")
	}
	m.actionSwitchSession(watcher.SessionInfo{ShortID: "another-session"})
	if m.sessionLabel != "snapshot-failure" || len(m.store.List()) != 1 {
		t.Fatal("session switch discarded unsaved cards")
	}
	m.sessionFilePath = validPath
	if err := m.saveSessionState(); err != nil || m.saveError != "" {
		t.Fatalf("save did not recover: %v, %q", err, m.saveError)
	}
	loaded := card.NewStore()
	if _, err := loaded.LoadSnapshot(validPath); err != nil || len(loaded.List()) != 1 || !loaded.List()[0].Pinned {
		t.Fatalf("recovered snapshot missing pin: cards=%+v err=%v", loaded.List(), err)
	}
}

func TestReconcileFailureBlocksLaterCheckpointsUntilRetry(t *testing.T) {
	m := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "failed-turn-test")
	defer m.StopWatching()
	first := &watcher.Turn{ID: 1, User: "first", Assistant: "```bash\necho first\n```"}
	second := &watcher.Turn{ID: 2, User: "second", Assistant: "```bash\necho second\n```"}
	m.Update(ReconcileFailedMsg{Err: errors.New("API unavailable"), Turn: first})
	m.Update(TurnReceivedMsg{Turn: second})
	if m.lastTurnProcessed != 0 || m.failedTurn != first || len(m.pendingTurns) != 1 {
		t.Fatalf("failed turn should block later checkpoint: last=%d pending=%d", m.lastTurnProcessed, len(m.pendingTurns))
	}
	m.Update(retryReconcileMsg{TurnID: 1, Attempt: 1})
	if !m.reconciling {
		t.Fatal("automatic retry did not start")
	}
	m.Update(ReconcileFailedMsg{Err: errors.New("API unavailable"), Turn: first})
	m.Update(retryReconcileMsg{TurnID: 1, Attempt: 2})
	m.Update(ReconcileFailedMsg{Err: errors.New("API unavailable"), Turn: first})
	if m.lastTurnProcessed != 0 || m.reconciling || m.failedTurn != first {
		t.Fatal("exhausted retries must leave the failed turn recoverable")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if !m.reconciling {
		t.Fatal("manual R did not retry failed turn")
	}
	m.Update(MutationsAppliedMsg{Turn: first, Mutations: []reconciler.Mutation{{Op: reconciler.OpAdd, CardID: "first", Content: "first"}}})
	if m.lastTurnProcessed != 1 || len(m.pendingTurns) != 0 || !m.reconciling {
		t.Fatal("later turn should start only after failed turn succeeds")
	}
	m.Update(MutationsAppliedMsg{Turn: second, Mutations: []reconciler.Mutation{{Op: reconciler.OpAdd, CardID: "second", Content: "second"}}})
	if m.lastTurnProcessed != 2 || m.failedTurn != nil || len(m.store.List()) != 2 {
		t.Fatal("backlog was not processed in order")
	}
}

func TestFailedTurnRemainsReplayableAfterRestart(t *testing.T) {
	m := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "failed-restart")
	m.Update(MutationsAppliedMsg{Turn: &watcher.Turn{ID: 1}, Mutations: []reconciler.Mutation{{Op: reconciler.OpAdd, CardID: "saved", Content: "saved"}}})
	failed := &watcher.Turn{ID: 2, User: "failed turn"}
	m.Update(ReconcileFailedMsg{Err: errors.New("unavailable"), Turn: failed})
	m.Update(TurnReceivedMsg{Turn: &watcher.Turn{ID: 3, User: "later turn"}})
	restarted := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "failed-restart")
	if restarted.lastTurnProcessed != 1 || len(restarted.store.List()) != 1 {
		t.Fatalf("restart skipped failed turn: last=%d cards=%d", restarted.lastTurnProcessed, len(restarted.store.List()))
	}
}

func TestEditorRoundTripUpdatesOriginalCard(t *testing.T) {
	store := card.NewStore()
	first := store.Add(&card.Card{ID: "first", Content: "original first"})
	second := store.Add(&card.Card{ID: "second", Content: "original second"})
	m := NewModel(store, reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "editor-original")
	m.selectedIndex = 1 // Selection changed while editing the first card.
	path := filepath.Join(t.TempDir(), "edit.md")
	if err := os.WriteFile(path, []byte("edited first"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Update(EditorFinishedMsg{TempFile: path, CardID: first.ID})
	if first.Content != "edited first" || second.Content != "original second" {
		t.Fatalf("edited wrong card: first=%q second=%q", first.Content, second.Content)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("editor temporary file not removed: %v", err)
	}
}

func TestEditorTempFilesAreUniqueAndPrivate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("EDITOR", "vim -f")
	store := card.NewStore()
	store.Add(&card.Card{ID: "first", Content: "private content"})
	m := NewModel(store, reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "editor-temp")
	if m.actionOpenInEditor() == nil || m.actionOpenInEditor() == nil {
		t.Fatal("editor was not prepared")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 || entries[0].Name() == entries[1].Name() {
		t.Fatalf("expected two unique editor files, got %v, err=%v", entries, err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		defer os.Remove(path)
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("editor file must be private: %v, err=%v", info, err)
		}
	}
}

func TestHistoryPreviewAndRestoreContent(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	store := card.NewStore()
	store.Add(&card.Card{ID: "history-card", Title: "Original title", Content: "first content"})
	store.Update("history-card", "second content", "User edit")
	store.Update("history-card", "third content", "AI revision")
	store.Add(&card.Card{ID: "other-history", Content: "other first"})
	store.Update("other-history", "other latest", "Other edit")
	m := NewModel(store, reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "history-restore")
	m.width, m.height = 100, 30
	m.updateViewportSize()
	m.actionToggleHistory()
	if m.historyIndex != 2 || !strings.Contains(ansi.Strip(m.currentContent), "third content") {
		t.Fatalf("history should open on latest content: index=%d content=%q", m.historyIndex, m.currentContent)
	}
	for _, want := range []string{"second content", "first content"} {
		m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		if !strings.Contains(ansi.Strip(m.currentContent), want) {
			t.Fatalf("did not preview older content %q: %q", want, m.currentContent)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	c, _ := store.Get("history-card")
	if c.Content != "first content" || c.Version != 4 || len(c.History) != 4 || m.historyIndex != 3 {
		t.Fatalf("history restoration failed: %+v index=%d", c, m.historyIndex)
	}
	if !strings.Contains(m.toast, "Restored content from v1 as v4") {
		t.Fatalf("no restore confirmation: %q", m.toast)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if c.Version != 4 {
		t.Fatal("restoring current version should not create duplicate history")
	}
	m.selectedIndex = 1 // Switching cards while viewing history starts on its latest revision.
	m.updateViewportContent()
	if m.historyIndex != 1 || !strings.Contains(ansi.Strip(m.currentContent), "other latest") {
		t.Fatalf("history selection leaked across cards: index=%d content=%q", m.historyIndex, m.currentContent)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.viewMode != ModeSplit {
		t.Fatal("h should close version preview")
	}
	reloaded := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "history-restore")
	restored, ok := reloaded.store.Get("history-card")
	if !ok || restored.Version != 4 || restored.Content != "first content" {
		t.Fatalf("restored version not persisted: %+v", restored)
	}
}

func TestTrashSelectionPurgeConfirmationAndRestore(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	store := card.NewStore()
	for _, id := range []string{"one", "two", "three"} {
		store.Add(&card.Card{ID: id, Title: id})
		store.Remove(id)
	}
	m := NewModel(store, reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "trash-picker")
	m.width, m.height = 100, 30
	if err := m.saveSessionState(); err != nil {
		t.Fatal(err)
	}
	key := func(s string) { m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	key("T")
	if !m.showTrash || !strings.Contains(m.View(), "DROPPED CARDS") {
		t.Fatal("trash should open with dropped cards")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // Select 'two', not the newest 'three'.
	if m.selectedTrashIdx != 1 {
		t.Fatalf("trash navigation failed: %d", m.selectedTrashIdx)
	}
	key("x")
	if !m.confirmTrashPurge || len(store.DroppedCards()) != 3 {
		t.Fatal("purge must require confirmation")
	}
	key("n")
	if m.confirmTrashPurge || len(store.DroppedCards()) != 3 {
		t.Fatal("cancel should leave trash unchanged")
	}
	key("x")
	key("y")
	drops := store.DroppedCards()
	if len(drops) != 2 || drops[0].ID != "three" || drops[1].ID != "one" || m.confirmTrashPurge {
		t.Fatalf("selected purge failed: %+v", drops)
	}
	if !strings.Contains(m.toast, "backups may retain") {
		t.Fatalf("purge confirmation omitted backup warning: %q", m.toast)
	}
	reloaded := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "trash-picker")
	if drops := reloaded.store.DroppedCards(); len(drops) != 2 || drops[0].ID != "three" || drops[1].ID != "one" {
		t.Fatalf("purge was not persisted: %+v", drops)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Restore selected 'one'.
	if m.showTrash || len(store.List()) != 1 || store.List()[0].ID != "one" {
		t.Fatalf("selective restore failed: %+v", store.List())
	}
}

func TestUndoDropAfterRestartRestoresEditedCard(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	store := card.NewStore()
	store.Add(&card.Card{ID: "edited", Title: "Important", Content: "original"})
	store.Update("edited", "edited in editor", "Editor change")
	m := NewModel(store, reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "undo-restart")
	m.width, m.height = 100, 30
	m.actionDrop()
	if !strings.Contains(m.toast, "U to undo") || len(store.List()) != 0 || !store.CanUndoDrop() {
		t.Fatalf("drop should offer undo: %q", m.toast)
	}
	foundButton := false
	for _, badge := range m.getActionBarBadges() {
		if badge.action == "undo_drop" {
			foundButton = true
		}
	}
	if !foundButton {
		t.Fatal("action bar must offer Undo after a drop")
	}
	restarted := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "undo-restart")
	if !restarted.store.CanUndoDrop() || len(restarted.store.List()) != 0 {
		t.Fatal("undo history did not survive restart")
	}
	restarted.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	cards := restarted.store.List()
	if len(cards) != 1 || cards[0].ID != "edited" || cards[0].Content != "edited in editor" || cards[0].Version != 2 || restarted.store.CanUndoDrop() {
		t.Fatalf("undo did not restore edited card: %+v", cards)
	}
	if !strings.Contains(restarted.toast, "Restored card") {
		t.Fatalf("undo success was not shown: %q", restarted.toast)
	}
	again := NewModel(card.NewStore(), reconciler.NewHeuristicReconciler(), nil, nil, nil, false, "undo-restart")
	if len(again.store.List()) != 1 || again.store.CanUndoDrop() {
		t.Fatal("restored card not persisted after undo")
	}
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

func TestModelBlockPickerNumberingPastNine(t *testing.T) {
	store := card.NewStore()
	var contentBuilder strings.Builder
	contentBuilder.WriteString("Deploying infrastructure:\n\n")
	for i := 1; i <= 14; i++ {
		contentBuilder.WriteString(fmt.Sprintf("%d. Step number %d: `echo step_%d`\n\n", i, i, i))
	}
	store.Add(&card.Card{ID: "c1", Title: "Deployment Steps", Content: contentBuilder.String()})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-picker-numbering")
	m.width = 100
	m.height = 40

	// Open block picker
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = u.(*Model)

	if !m.showBlockPicker {
		t.Fatalf("expected block picker to be open")
	}
	if len(m.activeBlocks) != 14 {
		t.Fatalf("expected 14 active blocks, got %d", len(m.activeBlocks))
	}

	view := m.View()
	// Must contain both single digit [1] and double digit [10], [11], [14]
	if !strings.Contains(view, "[1]") {
		t.Fatalf("expected view to contain [1], got:\n%s", view)
	}
	if !strings.Contains(view, "[9]") {
		t.Fatalf("expected view to contain [9], got:\n%s", view)
	}
	if !strings.Contains(view, "[10]") {
		t.Fatalf("expected view to contain [10], got:\n%s", view)
	}
	if !strings.Contains(view, "[14]") {
		t.Fatalf("expected view to contain [14], got:\n%s", view)
	}

	// Test 'G' jump to bottom
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = u.(*Model)
	if m.selectedBlockIdx != 13 {
		t.Fatalf("expected selectedBlockIdx to be 13 after 'G', got %d", m.selectedBlockIdx)
	}

	// Test 'g' jump to top
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = u.(*Model)
	if m.selectedBlockIdx != 0 {
		t.Fatalf("expected selectedBlockIdx to be 0 after 'g', got %d", m.selectedBlockIdx)
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

func TestModelResponsiveActionBarWrapping(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "Test Card", Content: "echo test"})
	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")

	// 1. Narrow width (80 columns, standard split)
	m.width = 80
	m.height = 30
	m.updateViewportSize()
	bar := m.renderActionBar()

	lines := m.getActionBarLines()
	if lines < 2 {
		t.Fatalf("expected at least 2 rows for 80-col width, got %d", lines)
	}
	barLinesCount := strings.Count(bar, "\n") + 1
	if barLinesCount != lines {
		t.Fatalf("rendered action bar lines (%d) != getActionBarLines (%d)", barLinesCount, lines)
	}

	// Verify crucial actions are NOT hidden on 80 cols
	hasSend := false
	hasSteps := false
	hasFind := false
	hasPin := false
	for _, btn := range m.buttons {
		if btn.Action == "send" {
			hasSend = true
		}
		if btn.Action == "block_picker" {
			hasSteps = true
		}
		if btn.Action == "search" {
			hasFind = true
		}
		if btn.Action == "pin" {
			hasPin = true
		}
	}
	if !hasSend {
		t.Fatalf("expected 'send' action button to be present on 80-col terminal")
	}
	if !hasSteps {
		t.Fatalf("expected 'block_picker' action button to be present on 80-col terminal")
	}
	if !hasFind {
		t.Fatalf("expected 'search' action button to be present on 80-col terminal")
	}
	if !hasPin {
		t.Fatalf("expected 'pin' action button to be present on 80-col terminal")
	}

	// 2. Very narrow width (50 columns)
	m.width = 50
	m.updateViewportSize()
	narrowLines := m.getActionBarLines()
	if narrowLines < 3 {
		t.Fatalf("expected at least 3 rows for 50-col width, got %d", narrowLines)
	}

	// 3. Wide width (180 columns)
	m.width = 180
	m.updateViewportSize()
	wideLines := m.getActionBarLines()
	if wideLines > 2 {
		t.Fatalf("expected <= 2 rows for wide 180-col screen, got %d", wideLines)
	}
}

func TestModelActionBarMultiRowMouseClick(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "Card 1", Content: "content 1"})
	store.Add(&card.Card{ID: "c2", Title: "Card 2", Content: "content 2"})
	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")

	m.width = 80
	m.height = 30
	m.updateViewportSize()
	_ = m.renderActionBar()

	// Find a button on Row 0 and a button on Row 1
	var row0Btn *Button
	var row1Btn *Button
	for i := range m.buttons {
		if m.buttons[i].Row == 0 && row0Btn == nil {
			row0Btn = &m.buttons[i]
		}
		if m.buttons[i].Row == 1 && row1Btn == nil {
			row1Btn = &m.buttons[i]
		}
	}

	if row0Btn == nil {
		t.Fatalf("expected at least one button on Row 0")
	}
	if row1Btn == nil {
		t.Fatalf("expected at least one button on Row 1")
	}

	// Click row 0 button (Row 0 is terminal Y=1)
	clickX0 := (row0Btn.X1 + row0Btn.X2) / 2
	u, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX0, Y: 1})
	m = u.(*Model)

	// Click row 1 button (Row 1 is terminal Y=2)
	clickX1 := (row1Btn.X1 + row1Btn.X2) / 2
	u, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX1, Y: 2})
	m = u.(*Model)

	// If clicking the action bar button toggled focus mode, restore split mode for card list testing
	if m.viewMode == ModeDetail {
		m.actionToggleFocus()
	}

	// Card list starts directly below the action bar rows:
	// Clicking card 2 (idx 1) is at cardListStartY + 1.
	cardListStartY := 1 + m.getActionBarLines()
	card2Y := cardListStartY + 1
	leftColWidth := m.getLeftColWidth()
	u, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftColWidth / 2, Y: card2Y})
	m = u.(*Model)
	if m.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex to be 1 after clicking second card row at Y=%d, got %d", card2Y, m.selectedIndex)
	}
}

func TestModelHelpModalActionShortcuts(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "Steps Card", Content: "```bash\necho hello\n```"})
	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 30
	m.updateViewportSize()

	// Open Help modal
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = u.(*Model)
	if !m.showHelp {
		t.Fatalf("expected showHelp to be true")
	}

	// Press 'b' to trigger Step Picker directly from the help palette
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = u.(*Model)
	if m.showHelp {
		t.Fatalf("expected showHelp to be closed after pressing 'b'")
	}
	if !m.showBlockPicker {
		t.Fatalf("expected showBlockPicker to be true after pressing 'b' in help modal")
	}
}

func TestModelBlockPickerOpenFile(t *testing.T) {
	// Create a temp file to test opening
	tmpFile, err := os.CreateTemp("", "syl-test-*.go")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_ = tmpFile.Close()

	store := card.NewStore()
	store.Add(&card.Card{
		ID:      "c-file",
		Title:   "File Card",
		Content: fmt.Sprintf("Please review the code in: %s and also /nonexistent/path/file.py", tmpFile.Name()),
	})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 30
	m.updateViewportSize()

	// Open block picker
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = u.(*Model)

	if !m.showBlockPicker {
		t.Fatalf("expected showBlockPicker to be true")
	}
	if len(m.activeBlocks) < 2 {
		t.Fatalf("expected at least 2 file blocks, got %d", len(m.activeBlocks))
	}

	// First block is the real temp file
	if m.activeBlocks[0].Type != card.BlockFile {
		t.Fatalf("expected BlockFile, got %s", m.activeBlocks[0].Type)
	}

	// Press 'o' on the first block (existing file)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = u.(*Model)

	if m.showBlockPicker {
		t.Fatalf("expected showBlockPicker to close after action")
	}
	if !strings.HasPrefix(m.toast, "✓ Opened") {
		t.Fatalf("expected toast '✓ Opened...', got %q", m.toast)
	}

	// Re-open block picker and test non-existent file
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = u.(*Model)
	m.selectedBlockIdx = 1 // select /nonexistent/path/file.py

	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = u.(*Model)

	if !strings.HasPrefix(m.toast, "⚠ File not found") {
		t.Fatalf("expected toast '⚠ File not found...', got %q", m.toast)
	}
}

func TestModelVimPaneFocusToggleAndJK(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "Card 1", Content: "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10"})
	card2Content := ""
	for i := 1; i <= 30; i++ {
		card2Content += fmt.Sprintf("card 2 line %d\n", i)
	}
	store.Add(&card.Card{ID: "c2", Title: "Card 2", Content: card2Content})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 15
	m.updateViewportSize()

	// Initial state: activePane is PaneCards
	if m.activePane != PaneCards {
		t.Fatalf("expected default activePane to be PaneCards")
	}

	// In PaneCards: 'j' selects next card
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = u.(*Model)
	if m.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex 1 after 'j', got %d", m.selectedIndex)
	}

	// Press 'w' to toggle to PanePreview
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = u.(*Model)
	if m.activePane != PanePreview {
		t.Fatalf("expected activePane to be PanePreview after 'w'")
	}

	// In PanePreview: 'j' scrolls down viewport instead of changing card
	initialOffset := m.viewport.YOffset
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = u.(*Model)
	if m.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex to remain 1 in PanePreview, got %d", m.selectedIndex)
	}
	if m.viewport.YOffset <= initialOffset {
		t.Fatalf("expected viewport offset to increase after 'j' in PanePreview")
	}

	// Test 'k' scrolls up in PanePreview
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = u.(*Model)
	if m.viewport.YOffset != initialOffset {
		t.Fatalf("expected viewport offset to return to initial after 'k'")
	}

	// Switch back to PaneCards with 'w'
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = u.(*Model)
	if m.activePane != PaneCards {
		t.Fatalf("expected activePane to be PaneCards after second 'w'")
	}
}

func TestModelVimMotions(t *testing.T) {
	longContent := ""
	for i := 1; i <= 60; i++ {
		longContent += fmt.Sprintf("line %d of content\n", i)
	}

	store := card.NewStore()
	store.Add(&card.Card{ID: "c-long", Title: "Long Card", Content: longContent})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 15
	m.updateViewportSize()

	// 1. Ctrl-E nudges 1 line down
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = u.(*Model)
	if m.viewport.YOffset != 1 {
		t.Fatalf("expected offset 1 after ctrl+e, got %d", m.viewport.YOffset)
	}

	// 2. Ctrl-Y nudges 1 line up
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = u.(*Model)
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected offset 0 after ctrl+y, got %d", m.viewport.YOffset)
	}

	// 3. '}' jumps 5 lines down
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'}'}})
	m = u.(*Model)
	if m.viewport.YOffset != 5 {
		t.Fatalf("expected offset 5 after '}', got %d", m.viewport.YOffset)
	}

	// 4. '{' jumps 5 lines up
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'{'}})
	m = u.(*Model)
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected offset 0 after '{', got %d", m.viewport.YOffset)
	}

	// 5. 'G' jumps to bottom
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = u.(*Model)
	if m.viewport.YOffset == 0 {
		t.Fatalf("expected offset to be near bottom after 'G', got 0")
	}

	// 6. Double 'g' ('gg') jumps to top
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = u.(*Model)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = u.(*Model)
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected offset 0 after 'gg', got %d", m.viewport.YOffset)
	}
}

func TestModelVimChecklistToggle(t *testing.T) {
	content := `- [ ] Setup database
- [ ] Run migrations`

	store := card.NewStore()
	store.Add(&card.Card{ID: "c-tasks", Title: "Tasks Card", Content: content})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	// 1. Press space to toggle first task
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	card1 := m.store.List()[0]
	if !strings.Contains(card1.Content, "- [x] Setup database") {
		t.Fatalf("expected first task checked, got: %s", card1.Content)
	}

	// 2. Press space again to toggle second task
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	card2 := m.store.List()[0]
	if !strings.Contains(card2.Content, "- [x] Run migrations") {
		t.Fatalf("expected second task checked, got: %s", card2.Content)
	}

	// 3. Press space a third time to reset
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = u.(*Model)
	card3 := m.store.List()[0]
	if strings.Contains(card3.Content, "- [x]") {
		t.Fatalf("expected all tasks reset, got: %s", card3.Content)
	}
}

func TestModelVimFoldToggle(t *testing.T) {
	content := "Config:\n```yaml\nserver:\n  port: 8080\n  host: 0.0.0.0\n```\nDone."
	store := card.NewStore()
	store.Add(&card.Card{ID: "c-fold", Title: "Fold Card", Content: content})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	// Press 'Z' to fold code
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	m = u.(*Model)
	if !m.isFolded {
		t.Fatalf("expected isFolded to be true")
	}
	if !strings.Contains(m.viewport.View(), "folded") {
		t.Fatalf("expected folded indicator in viewport view, got: %s", m.viewport.View())
	}

	// Press 'Z' again to expand code
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	m = u.(*Model)
	if m.isFolded {
		t.Fatalf("expected isFolded to be false")
	}
}

func TestModelVimEditorFinished(t *testing.T) {
	store := card.NewStore()
	c := store.Add(&card.Card{ID: "c-edit", Title: "Edit Card", Content: "original content"})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	// Create temp file with new content
	tmp, err := os.CreateTemp("", "syl-edit-test-*.md")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmp.Name())
	_, _ = tmp.WriteString("updated content from nvim")
	_ = tmp.Close()

	// Simulate EditorFinishedMsg
	u, _ := m.Update(EditorFinishedMsg{
		Err:      nil,
		TempFile: tmp.Name(),
		CardID:   c.ID,
	})
	m = u.(*Model)

	updatedCard := m.store.List()[0]
	if updatedCard.Content != "updated content from nvim" {
		t.Fatalf("expected updated content, got: %q", updatedCard.Content)
	}
	if !strings.Contains(m.toast, "✓ Card updated from editor") {
		t.Fatalf("expected success toast, got %q", m.toast)
	}
}

func TestModelVimSearchMatchNavigation(t *testing.T) {
	content := "Line 0: intro\n\nLine 1: apple found\n\nLine 2: middle\n\nLine 3: apple here too\n\nLine 4: end\n\nLine 5: final apple"
	store := card.NewStore()
	store.Add(&card.Card{ID: "c-search", Title: "Search Card", Content: content})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "test-session")
	m.width = 90
	m.height = 20
	m.updateViewportSize()

	m.searchQuery = "apple"
	m.computeSearchMatches()

	if len(m.searchMatches) != 3 {
		t.Fatalf("expected 3 search matches, got %d", len(m.searchMatches))
	}

	// Press 'n' to jump to next match (index 1)
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = u.(*Model)
	if m.searchMatchIdx != 1 {
		t.Fatalf("expected searchMatchIdx 1 after 'n', got %d", m.searchMatchIdx)
	}

	// Press 'n' again to jump to index 2
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = u.(*Model)
	if m.searchMatchIdx != 2 {
		t.Fatalf("expected searchMatchIdx 2 after 'n', got %d", m.searchMatchIdx)
	}

	// Press 'n' again to wrap to index 0
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = u.(*Model)
	if m.searchMatchIdx != 0 {
		t.Fatalf("expected searchMatchIdx 0 after wrapping 'n', got %d", m.searchMatchIdx)
	}

	// Press 'N' to go back to index 2
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = u.(*Model)
	if m.searchMatchIdx != 2 {
		t.Fatalf("expected searchMatchIdx 2 after 'N', got %d", m.searchMatchIdx)
	}
}

func TestModelNewSessionAction(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "Old Session Card", Content: "Card content"})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "old-session")
	m.width = 90
	m.height = 25
	m.updateViewportSize()

	// Press 'N' to clear pane and start waiting for new session
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = u.(*Model)

	if len(m.store.List()) != 0 {
		t.Fatalf("expected 0 cards in store after 'N', got %d", len(m.store.List()))
	}
	if !m.waitingForSession {
		t.Fatalf("expected waitingForSession to be true")
	}
	if m.sessionLabel != "waiting" {
		t.Fatalf("expected sessionLabel to be 'waiting', got %q", m.sessionLabel)
	}
	if !strings.Contains(m.toast, "Waiting for conversation") {
		t.Fatalf("expected waiting toast, got %q", m.toast)
	}
}

func TestModelWaitingForSessionRender(t *testing.T) {
	store := card.NewStore()
	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "waiting")
	m.width = 100
	m.height = 30
	m.updateViewportSize()

	m.SetWaitingForSession(true, "/Users/dev/workspace-demo", "antigravity")
	view := m.renderBaseView()

	if !strings.Contains(view, "WAITING FOR CONVERSATION") {
		t.Fatalf("expected header 'WAITING FOR CONVERSATION' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "/Users/dev/workspace-demo") {
		t.Fatalf("expected workspace path in waiting view, got:\n%s", view)
	}
	if !strings.Contains(strings.ToLower(view), "antigravity") {
		t.Fatalf("expected agent name in waiting view, got:\n%s", view)
	}
}

func TestModelNewSessionDetected(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	store := card.NewStore()
	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "waiting")
	m.width = 90
	m.height = 25
	m.updateViewportSize()
	m.SetWaitingForSession(true, "/test/path", "claude")

	tmpFile := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(tmpFile, []byte(""), 0o600); err != nil {
		t.Fatalf("write temp transcript: %v", err)
	}

	u, _ := m.Update(newSessionDetectedMsg{
		Session: watcher.SessionInfo{
			ShortID: "new-sess-123",
			Path:    tmpFile,
			Agent:   "claude",
		},
	})
	m = u.(*Model)

	if m.waitingForSession {
		t.Fatalf("expected waitingForSession to be false after newSessionDetectedMsg")
	}
	if m.sessionLabel != "new-sess-123" {
		t.Fatalf("expected sessionLabel to be 'new-sess-123', got %q", m.sessionLabel)
	}
}

func TestModelSessionSwitcherStartNewSession(t *testing.T) {
	store := card.NewStore()
	store.Add(&card.Card{ID: "c1", Title: "Active Card", Content: "Body"})

	m := NewModel(store, reconciler.NewMockReconciler(), nil, nil, nil, false, "current-session")
	m.width = 90
	m.height = 25
	m.updateViewportSize()

	// Open session switcher
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = u.(*Model)
	if !m.showSessionSwitcher {
		t.Fatalf("expected showSessionSwitcher to be true")
	}
	if m.selectedSessionIdx != 0 {
		t.Fatalf("expected selectedSessionIdx to default to 0")
	}

	// Press Enter on item 0 ("✨ Start New Session")
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = u.(*Model)

	if len(m.store.List()) != 0 {
		t.Fatalf("expected store cleared, got %d cards", len(m.store.List()))
	}
	if !m.waitingForSession {
		t.Fatalf("expected waitingForSession to be true")
	}
	if m.showSessionSwitcher {
		t.Fatalf("expected session switcher modal to be closed")
	}
}

