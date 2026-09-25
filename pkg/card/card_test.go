package card

import (
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

