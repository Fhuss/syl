package reconciler

import (
	"context"
	"testing"

	"github.com/Fhuss/syl/pkg/watcher"
)

func TestMockReconcilerMultiTurn(t *testing.T) {
	rec := NewMockReconciler()
	ctx := context.Background()

	// Turn 1
	turn1 := &watcher.Turn{ID: 1, User: "Deploy to ECS"}
	muts1, err := rec.Reconcile(ctx, nil, turn1)
	if err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	if len(muts1) != 1 || muts1[0].Op != OpAdd {
		t.Fatalf("expected 1 ADD op, got %v", muts1)
	}

	// Turn 2
	turn2 := &watcher.Turn{ID: 2, User: "AccessDenied on ECR"}
	muts2, err := rec.Reconcile(ctx, nil, turn2)
	if err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
	if len(muts2) != 1 || muts2[0].Op != OpUpdate {
		t.Fatalf("expected 1 UPDATE op, got %v", muts2)
	}

	// Turn 3
	turn3 := &watcher.Turn{ID: 3, User: "Switch to eu-central-1"}
	muts3, err := rec.Reconcile(ctx, nil, turn3)
	if err != nil {
		t.Fatalf("turn 3 error: %v", err)
	}
	if len(muts3) != 2 || muts3[0].Op != OpUpdate || muts3[1].Op != OpAdd {
		t.Fatalf("expected UPDATE and ADD ops, got %v", muts3)
	}

	// Turn 4
	turn4 := &watcher.Turn{ID: 4, User: "Just chatting"}
	muts4, err := rec.Reconcile(ctx, nil, turn4)
	if err != nil {
		t.Fatalf("turn 4 error: %v", err)
	}
	if len(muts4) != 1 || muts4[0].Op != OpNoop {
		t.Fatalf("expected NOOP op, got %v", muts4)
	}
}
