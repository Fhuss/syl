package harness

import (
	"context"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"**Kubernetes Deployment**", "Kubernetes Deployment"},
		{"\"AWS ECS Setup\"", "AWS ECS Setup"},
		{"Title: Docker Redis Container", "Docker Redis Container"},
		{"Topic: Redis Configuration.", "Redis Configuration"},
		{"Clean Title", "Clean Title"},
		{"* Redis Setup\n* Alternative options: Foo", "Redis Setup"},
	}

	for _, tt := range tests {
		got := CleanTitle(tt.input)
		if got != tt.expected {
			t.Errorf("CleanTitle(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestHeuristicHarness(t *testing.T) {
	h := NewHeuristicHarness()
	ctx := context.Background()

	// 1. Strip polite conversational filler
	title, err := h.Summarize(ctx, "Can you please deploy our service to AWS ECS Fargate", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "deploy our service to..." {
		t.Errorf("expected stripped title, got %q", title)
	}

	// 2. Generic prompt fallback to assistant command/context
	genericTitle, err := h.Summarize(ctx, "let's try it", "Running command: docker run -d redis")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if genericTitle == "" {
		t.Errorf("expected non-empty generic title")
	}
}

func TestAutoHarnessAvailable(t *testing.T) {
	h := NewAutoHarness("heuristic", "")
	if !h.Available() {
		t.Fatalf("expected auto harness to be available")
	}
	title, err := h.Summarize(context.Background(), "Setup PostgreSQL database", "")
	if err != nil || title == "" {
		t.Fatalf("expected summary from auto harness, got %q (err: %v)", title, err)
	}
}

func TestAutoHarnessAliases(t *testing.T) {
	hAgy := NewAutoHarness("antigravity", "")
	if len(hAgy.harnesses) == 0 || hAgy.harnesses[0].Name() != "agy" {
		t.Errorf("expected first candidate to be agy, got %v", hAgy.harnesses[0].Name())
	}

	hClaude := NewAutoHarness("claude-code", "")
	if len(hClaude.harnesses) == 0 || hClaude.harnesses[0].Name() != "claude" {
		t.Errorf("expected first candidate to be claude, got %v", hClaude.harnesses[0].Name())
	}
}

