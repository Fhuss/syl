package reconciler

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/harness"
	"github.com/Fhuss/syl/pkg/watcher"
)

var (
	fenceRegex = regexp.MustCompile("(?s)```([a-zA-Z0-9_-]+)?\n(.*?)\n```")
	stepRegex  = regexp.MustCompile(`(?m)^\s*(\d+\.|\*|-)\s+(.*)`)
)

// HeuristicReconciler extracts code blocks and procedures without requiring an LLM API key.
type HeuristicReconciler struct {
	counter int
}

func NewHeuristicReconciler() *HeuristicReconciler {
	return &HeuristicReconciler{}
}

func (h *HeuristicReconciler) Reconcile(ctx context.Context, currentCards []*card.Card, turn *watcher.Turn) ([]Mutation, error) {
	// Look for code blocks in assistant output
	matches := fenceRegex.FindAllStringSubmatch(turn.Assistant, -1)

	// If no code blocks, check for numbered procedural steps
	hasSteps := len(stepRegex.FindAllString(turn.Assistant, -1)) >= 2

	if len(matches) == 0 && !hasSteps {
		return []Mutation{
			{
				Op:     OpNoop,
				Reason: "No actionable code blocks or procedures in turn",
			},
		}, nil
	}

	h.counter++
	userPromptLower := strings.ToLower(turn.User)

	// Check if this looks like a correction/revision to an existing card
	isCorrection := strings.Contains(userPromptLower, "error") ||
		strings.Contains(userPromptLower, "failed") ||
		strings.Contains(userPromptLower, "fix") ||
		strings.Contains(userPromptLower, "instead") ||
		strings.Contains(userPromptLower, "change") ||
		strings.Contains(userPromptLower, "update")

	if isCorrection && len(currentCards) > 0 {
		// Update the most recent card
		target := currentCards[len(currentCards)-1]
		var newContent strings.Builder
		for i, m := range matches {
			lang := m[1]
			code := strings.TrimSpace(m[2])
			if i > 0 {
				newContent.WriteString("\n\n")
			}
			newContent.WriteString(fmt.Sprintf("```%s\n%s\n```", lang, code))
		}
		if newContent.Len() == 0 {
			newContent.WriteString(turn.Assistant)
		}

		return []Mutation{
			{
				Op:      OpUpdate,
				CardID:  target.ID,
				Content: newContent.String(),
				Reason:  fmt.Sprintf("Revised after user follow-up (Turn %d)", turn.ID),
			},
		}, nil
	}

	// Create a new card
	cardID := fmt.Sprintf("card-%d", h.counter)
	cardType := card.TypeCommand
	if len(matches) > 1 || hasSteps {
		cardType = card.TypeProcedure
	}

	// Determine initial concise title using smart heuristic extraction
	title, _ := harness.NewHeuristicHarness().Summarize(ctx, turn.User, turn.Assistant)
	if title == "" {
		title = fmt.Sprintf("Turn #%d", turn.ID)
	}

	var contentBuilder strings.Builder
	for i, m := range matches {
		lang := m[1]
		code := strings.TrimSpace(m[2])
		if i > 0 {
			contentBuilder.WriteString("\n\n")
		}
		contentBuilder.WriteString(fmt.Sprintf("```%s\n%s\n```", lang, code))
	}
	if contentBuilder.Len() == 0 {
		contentBuilder.WriteString(turn.Assistant)
	}

	return []Mutation{
		{
			Op:      OpAdd,
			CardID:  cardID,
			Type:    cardType,
			Title:   title,
			Content: contentBuilder.String(),
			Reason:  fmt.Sprintf("Extracted from conversation Turn %d", turn.ID),
		},
	}, nil
}
