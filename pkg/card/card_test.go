package card

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreAddAndUpdate(t *testing.T) {
	s := NewStore()

	c := &Card{
		ID:      "aws-ecs-deploy",
		Type:    TypeProcedure,
		Title:   "Deploy to ECS Fargate",
		Content: "1. Create cluster\n2. Run task in us-east-1",
	}

	added := s.Add(c)
	if added.Version != 1 {
		t.Fatalf("expected version 1, got %d", added.Version)
	}
	if len(added.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(added.History))
	}

	// Update to v2
	updated, ok := s.Update("aws-ecs-deploy", "1. Create cluster\n2. Run task in eu-central-1", "User switched region to Frankfurt")
	if !ok {
		t.Fatalf("expected card update to succeed")
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}
	if updated.Status != StatusRevised {
		t.Fatalf("expected status revised, got %s", updated.Status)
	}
	if len(updated.History) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(updated.History))
	}

	// Invalidate
	inv, ok := s.Invalidate("aws-ecs-deploy", "Superseded by Terraform template")
	if !ok {
		t.Fatalf("expected card invalidate to succeed")
	}
	if inv.Status != StatusInvalidated {
		t.Fatalf("expected status invalidated, got %s", inv.Status)
	}
}

func TestRestoreRevisionPreservesHistoryAndPersists(t *testing.T) {
	s := NewStore()
	s.Add(&Card{ID: "revision", Title: "Keep title", Content: "original"})
	s.Update("revision", "mistake", "User edit")
	s.Update("revision", "another mistake", "AI update")
	s.TogglePin("revision")
	if _, ok := s.RestoreRevision("revision", 3); ok {
		t.Fatal("out-of-range revision was accepted")
	}
	if _, ok := s.RestoreRevision("revision", 2); ok {
		t.Fatal("current revision should not create a duplicate")
	}
	restored, ok := s.RestoreRevision("revision", 0)
	if !ok || restored.Version != 4 || restored.Content != "original" || restored.Title != "Keep title" || !restored.Pinned {
		t.Fatalf("restore did not keep card identity and metadata: %+v, %v", restored, ok)
	}
	if len(restored.History) != 4 || restored.History[1].Content != "mistake" || restored.History[3].Reason != "Restored content from v1" {
		t.Fatalf("restore destroyed history: %+v", restored.History)
	}
	path := filepath.Join(t.TempDir(), "revision.json")
	if err := s.SaveSnapshot(path, "revision-session", 2); err != nil {
		t.Fatal(err)
	}
	reloaded := NewStore()
	if _, err := reloaded.LoadSnapshot(path); err != nil {
		t.Fatal(err)
	}
	c, ok := reloaded.Get("revision")
	if !ok || c.Content != "original" || c.Version != 4 || len(c.History) != 4 {
		t.Fatalf("restored revision did not survive restart: %+v, %v", c, ok)
	}
}

func TestStorePinning(t *testing.T) {
	s := NewStore()
	c1 := &Card{ID: "c1", Title: "First"}
	c2 := &Card{ID: "c2", Title: "Second"}

	s.Add(c1)
	s.Add(c2)

	// Pin c2
	s.TogglePin("c2")

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list[0].ID != "c2" {
		t.Fatalf("expected pinned c2 to be first in list, got %s", list[0].ID)
	}
}

func TestStoreRemove(t *testing.T) {
	s := NewStore()
	c1 := &Card{ID: "c1", Title: "First"}
	c2 := &Card{ID: "c2", Title: "Second"}

	s.Add(c1)
	s.Add(c2)

	if !s.Remove("c1") {
		t.Fatalf("expected remove to succeed")
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 card, got %d", len(list))
	}
	if list[0].ID != "c2" {
		t.Fatalf("expected c2 remaining, got %s", list[0].ID)
	}
}

func TestUndoDropRestoresContentHistoryPinAndOrder(t *testing.T) {
	s := NewStore()
	s.Add(&Card{ID: "a", Title: "A"})
	b := s.Add(&Card{ID: "b", Title: "B", Content: "draft"})
	s.Add(&Card{ID: "c", Title: "C"})
	s.Update("b", "edited", "Editor change")
	s.TogglePin("b")
	if !s.Remove("b") || !s.CanUndoDrop() {
		t.Fatal("drop should retain undo state")
	}
	b.Content = "changed by a stale pointer"
	restored, ok := s.UndoDrop()
	if !ok || restored.ID != "b" || restored.Content != "edited" || restored.Version != 2 || !restored.Pinned || len(restored.History) != 2 {
		t.Fatalf("undo lost card state: %+v, %v", restored, ok)
	}
	if len(s.order) != 3 || s.order[0] != "a" || s.order[1] != "b" || s.order[2] != "c" || s.CanUndoDrop() {
		t.Fatalf("undo did not restore original order: %+v", s.order)
	}
	if _, ok := s.UndoDrop(); ok {
		t.Fatal("second undo must not remove anything")
	}
}

func TestUndoDropAvoidsReusedCardID(t *testing.T) {
	s := NewStore()
	s.Add(&Card{ID: "card-1", Content: "old"})
	s.Remove("card-1")
	s.Add(&Card{ID: "card-1", Content: "new"})
	c, ok := s.UndoDrop()
	if !ok || c.ID != "card-1-restored-1" || c.Content != "old" || len(s.List()) != 2 {
		t.Fatalf("restoring reused ID failed: %+v, %v", c, ok)
	}
}

func TestPurgeSelectedDropWithoutDiscardingOtherUndoHistory(t *testing.T) {
	s := NewStore()
	for _, id := range []string{"one", "two", "three"} {
		s.Add(&Card{ID: id, Title: id})
		s.Remove(id)
	}
	drops := s.DroppedCards()
	if len(drops) != 3 || drops[0].ID != "three" || drops[1].ID != "two" || drops[2].ID != "one" {
		t.Fatalf("trash order incorrect: %+v", drops)
	}
	drops[0].Title = "caller mutation"
	if s.DroppedCards()[0].Title != "three" {
		t.Fatal("trash exposed mutable store pointers")
	}
	if _, ok := s.PurgeDropped(-1); ok {
		t.Fatal("negative purge index accepted")
	}
	purged, ok := s.PurgeDropped(1)
	if !ok || purged.ID != "two" {
		t.Fatalf("wrong card purged: %+v, %v", purged, ok)
	}
	path := filepath.Join(t.TempDir(), "session.json")
	if err := s.SaveSnapshot(path, "purge-session", 3); err != nil {
		t.Fatal(err)
	}
	loaded := NewStore()
	if _, err := loaded.LoadSnapshot(path); err != nil {
		t.Fatal(err)
	}
	if cards := loaded.DroppedCards(); len(cards) != 2 || cards[0].ID != "three" || cards[1].ID != "one" {
		t.Fatalf("purge did not persist: %+v", cards)
	}
	if c, ok := loaded.RestoreDropped(1); !ok || c.ID != "one" {
		t.Fatalf("selective restore failed: %+v, %v", c, ok)
	}
	if c, ok := loaded.UndoDrop(); !ok || c.ID != "three" {
		t.Fatalf("most recent undo failed: %+v, %v", c, ok)
	}
	if loaded.CanUndoDrop() {
		t.Fatal("purged card came back into undo history")
	}
}

func TestUndoDropPersistsAcrossRestartAndIsBounded(t *testing.T) {
	s := NewStore()
	for i := 1; i <= maxUndoDrops+1; i++ {
		id := fmt.Sprintf("card-%d", i)
		s.Add(&Card{ID: id, Title: id})
		if !s.Remove(id) {
			t.Fatalf("drop %s failed", id)
		}
	}
	path := filepath.Join(t.TempDir(), "session.json")
	if err := s.SaveSnapshot(path, "undo-session", 7); err != nil {
		t.Fatal(err)
	}
	loaded := NewStore()
	if _, err := loaded.LoadSnapshot(path); err != nil {
		t.Fatal(err)
	}
	for i := maxUndoDrops + 1; i > 1; i-- {
		c, ok := loaded.UndoDrop()
		if !ok || c.ID != fmt.Sprintf("card-%d", i) || c.Number != i {
			t.Fatalf("undo #%d failed: %+v, %v", i, c, ok)
		}
	}
	if loaded.CanUndoDrop() {
		t.Fatal("oldest drop should be evicted from bounded history")
	}
	newCard := loaded.Add(&Card{ID: "after-restart"})
	if newCard.Number != maxUndoDrops+2 {
		t.Fatalf("number counter did not survive restart: %d", newCard.Number)
	}
}

func TestStoreGet(t *testing.T) {
	s := NewStore()
	c1 := &Card{ID: "c1", Title: "First"}
	s.Add(c1)

	got, exists := s.Get("c1")
	if !exists || got.Title != "First" {
		t.Fatalf("expected to get card c1, got %v (exists: %v)", got, exists)
	}

	_, exists = s.Get("nonexistent")
	if exists {
		t.Fatalf("expected nonexistent card to not exist")
	}
}

func TestCardNumbering(t *testing.T) {
	s := NewStore()
	c1 := s.Add(&Card{ID: "c1", Title: "First"})
	c2 := s.Add(&Card{ID: "c2", Title: "Second"})

	if c1.Number != 1 {
		t.Errorf("expected card 1 number to be 1, got %d", c1.Number)
	}
	if c2.Number != 2 {
		t.Errorf("expected card 2 number to be 2, got %d", c2.Number)
	}
}

func TestStoreUpdateTitle(t *testing.T) {
	s := NewStore()
	c1 := s.Add(&Card{ID: "c1", Title: "Original Title"})
	if !s.UpdateTitle("c1", "Succinct AI Title") {
		t.Fatalf("expected UpdateTitle to succeed")
	}

	got, _ := s.Get("c1")
	if got.Title != "Succinct AI Title" {
		t.Errorf("expected title 'Succinct AI Title', got %q", got.Title)
	}
	if got.Version != 1 {
		t.Errorf("expected version to remain 1 after title update, got %d", got.Version)
	}

	if s.UpdateTitle("nonexistent", "Title") {
		t.Errorf("expected UpdateTitle on nonexistent card to return false")
	}
	_ = c1
}

func TestStoreExportMarkdown(t *testing.T) {
	s := NewStore()
	s.Add(&Card{
		ID:      "c1",
		Type:    TypeProcedure,
		Title:   "Setup AWS",
		Content: "aws ec2 create-vpc",
	})
	s.Add(&Card{
		ID:      "c2",
		Type:    TypeCommand,
		Title:   "Curl Weather",
		Content: "curl wttr.in/Stockholm",
		Pinned:  true,
	})

	md := s.ExportMarkdown("test-session")
	if len(md) == 0 {
		t.Fatalf("expected non-empty markdown export")
	}
	if !containsString(md, "# Syl Reference Notes") {
		t.Errorf("expected header in export")
	}
	if !containsString(md, "Setup AWS") || !containsString(md, "Curl Weather") {
		t.Errorf("expected card titles in export")
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

func TestSessionFilePathTightensExistingDirectory(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("SYL_CONFIG_DIR", configDir)
	dir := filepath.Join(configDir, "sessions")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionFilePath("old"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("session directory must be private: info=%v err=%v", info, err)
	}
}

func TestLoadSnapshotRejectsIncompleteDataWithoutChangingStore(t *testing.T) {
	for _, data := range []string{
		`null`,
		`{"session_id":"test"}`,
		`{"session_id":"test","cards":[null]}`,
		`{"session_id":"test","cards":[{"id":"duplicate"},{"id":"duplicate"}]}`,
		`{"session_id":"test","cards":[],"undo_drops":[{"card":null,"index":0}]}`,
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			store := NewStore()
			store.Add(&Card{ID: "keep", Content: "existing"})
			if _, err := store.LoadSnapshot(path); err == nil {
				t.Fatal("incomplete snapshot was accepted")
			}
			if cards := store.List(); len(cards) != 1 || cards[0].ID != "keep" {
				t.Fatalf("load failure changed existing cards: %+v", cards)
			}
		})
	}
}

func TestStoreSnapshotPersistence(t *testing.T) {
	tmpFile := "./test-session-snap.json"
	_ = os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	s1 := NewStore()
	c1 := s1.Add(&Card{
		ID:      "card-1",
		Type:    TypeProcedure,
		Title:   "Original Title",
		Content: "step 1: deploy",
	})
	s1.UpdateTitle("card-1", "Succinct AI Title")
	s1.TogglePin("card-1")

	s1.Add(&Card{
		ID:      "card-2",
		Type:    TypeCommand,
		Title:   "Docker Run",
		Content: "docker run -d nginx",
	})

	// Save snapshot
	err := s1.SaveSnapshot(tmpFile, "session-123", 15)
	if err != nil {
		t.Fatalf("failed to save snapshot: %v", err)
	}

	if info, err := os.Stat(tmpFile); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot must be private (0600): info=%v err=%v", info, err)
	}

	// Load into a fresh store
	s2 := NewStore()
	snap, err := s2.LoadSnapshot(tmpFile)
	if err != nil {
		t.Fatalf("failed to load snapshot: %v", err)
	}

	if snap.SessionID != "session-123" {
		t.Errorf("expected session ID 'session-123', got %s", snap.SessionID)
	}
	if snap.LastTurnProcessed != 15 {
		t.Errorf("expected last turn 15, got %d", snap.LastTurnProcessed)
	}

	cards := s2.List()
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards in restored store, got %d", len(cards))
	}

	// Verify card 1 attributes
	if cards[0].ID != "card-1" {
		t.Errorf("expected first card to be card-1, got %s", cards[0].ID)
	}
	if cards[0].Title != "Succinct AI Title" {
		t.Errorf("expected restored title 'Succinct AI Title', got %q", cards[0].Title)
	}
	if !cards[0].Pinned {
		t.Errorf("expected card-1 to retain pinned state")
	}

	// Verify counter numbering continues properly
	c3 := s2.Add(&Card{
		ID:    "card-3",
		Title: "Third Card",
	})
	if c3.Number != 3 {
		t.Errorf("expected card 3 number to be 3, got %d", c3.Number)
	}
	_ = c1
}

func TestStoreAddCollisionHandling(t *testing.T) {
	s := NewStore()

	// Add first card
	c1 := s.Add(&Card{
		ID:    "card-1",
		Title: "First Card",
	})
	if c1.Number != 1 {
		t.Errorf("expected card 1 number to be 1, got %d", c1.Number)
	}

	// Add second card with the SAME ID "card-1" (simulating a clash)
	c2 := s.Add(&Card{
		ID:    "card-1",
		Title: "Second Card with colliding ID",
	})

	if c2.ID == c1.ID {
		t.Errorf("expected c2 ID to be disambiguated, got same ID %s", c2.ID)
	}
	if c2.Number != 2 {
		t.Errorf("expected c2 number to be 2, got %d", c2.Number)
	}

	cards := s.List()
	if len(cards) != 2 {
		t.Fatalf("expected 2 distinct cards in store, got %d", len(cards))
	}
	if cards[0].Title != "First Card" || cards[1].Title != "Second Card with colliding ID" {
		t.Errorf("expected both cards preserved in order, got: %v, %v", cards[0].Title, cards[1].Title)
	}
}
