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
