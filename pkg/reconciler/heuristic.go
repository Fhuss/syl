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

// HeuristicReconciler extracts and mutates reference cards without requiring an external LLM API.
type HeuristicReconciler struct {
	counter int
}

func NewHeuristicReconciler() *HeuristicReconciler {
	return &HeuristicReconciler{}
}

func (h *HeuristicReconciler) Reconcile(ctx context.Context, currentCards []*card.Card, turn *watcher.Turn) ([]Mutation, error) {
	userPromptLower := strings.ToLower(turn.User)

	// 1. Check for invalidation signals first: e.g. "don't use X", "deprecated", "disproven", "broken"
	isInvalidation := strings.Contains(userPromptLower, "don't use") ||
		strings.Contains(userPromptLower, "do not use") ||
		strings.Contains(userPromptLower, "deprecated") ||
		strings.Contains(userPromptLower, "disproven") ||
		strings.Contains(userPromptLower, "obsolete") ||
		strings.Contains(userPromptLower, "drop this") ||
		strings.Contains(userPromptLower, "invalid")

	if isInvalidation && len(currentCards) > 0 {
		if target := findMatchingCard(currentCards, turn, turn.Assistant); target != nil {
			return []Mutation{
				{
					Op:     OpInvalidate,
					CardID: target.ID,
					Reason: fmt.Sprintf("Superseded or marked invalid in Turn %d", turn.ID),
				},
			}, nil
		}
	}

	// 2. Filter out conversational chatter: check for actionable code or procedures
	matches := fenceRegex.FindAllStringSubmatch(turn.Assistant, -1)
	hasSteps := len(stepRegex.FindAllString(turn.Assistant, -1)) >= 2

	if len(matches) == 0 && !hasSteps {
		return []Mutation{
			{
				Op:     OpNoop,
				Reason: "No actionable code blocks or procedures in turn",
			},
		}, nil
	}

	// Build clean content from code blocks or fallback to formatted assistant steps
	var contentBuilder strings.Builder
	for i, m := range matches {
		lang := m[1]
		code := strings.TrimSpace(m[2])
		if code == "" {
			continue
		}
		if i > 0 {
			contentBuilder.WriteString("\n\n")
		}
		contentBuilder.WriteString(fmt.Sprintf("```%s\n%s\n```", lang, code))
	}
	if contentBuilder.Len() == 0 {
		contentBuilder.WriteString(turn.Assistant)
	}

	content := contentBuilder.String()

	// 3. Check for in-place mutation: does this turn refine, correct, or evolve an existing card?
	if target := findMatchingCard(currentCards, turn, content); target != nil {
		reason := fmt.Sprintf("Revised after user follow-up (Turn %d)", turn.ID)
		if strings.Contains(userPromptLower, "error") || strings.Contains(userPromptLower, "failed") {
			reason = fmt.Sprintf("Fixed error reported in Turn %d", turn.ID)
		} else if strings.Contains(userPromptLower, "instead") || strings.Contains(userPromptLower, "change") || strings.Contains(userPromptLower, "region") {
			reason = fmt.Sprintf("Adjusted configuration in Turn %d", turn.ID)
		}

		return []Mutation{
			{
				Op:      OpUpdate,
				CardID:  target.ID,
				Content: content,
				Reason:  reason,
			},
		}, nil
	}

	// 4. If no existing card matches, create a genuine new reference card
	h.counter++
	cardID := fmt.Sprintf("card-%d", h.counter)
	cardType := card.TypeCommand
	if len(matches) > 1 || hasSteps {
		cardType = card.TypeProcedure
	}

	// Determine title using cache first, then smart heuristic extraction
	var title string
	if cached, ok := harness.GetTitleCache().GetByPrompt(turn.User); ok {
		title = cached
	} else {
		title, _ = harness.NewHeuristicHarness().Summarize(ctx, turn.User, turn.Assistant)
	}
	if title == "" {
		title = fmt.Sprintf("Turn #%d", turn.ID)
	}

	return []Mutation{
		{
			Op:      OpAdd,
			CardID:  cardID,
			Type:    cardType,
			Title:   title,
			Content: content,
			Reason:  fmt.Sprintf("Extracted from conversation Turn %d", turn.ID),
		},
	}, nil
}

// findMatchingCard identifies whether a turn is an in-place revision of an existing card.
func findMatchingCard(cards []*card.Card, turn *watcher.Turn, newCommands string) *card.Card {
	if len(cards) == 0 {
		return nil
	}

	userLower := strings.ToLower(turn.User)
	newCmdLower := strings.ToLower(newCommands)

	// 1. Check for CLI tool domain overlap (e.g. AWS, Docker, Git, Curl, etc.)
	knownTools := []string{
		"aws", "docker", "kubectl", "curl", "git", "go", "npm", "pnpm", "yarn",
		"cargo", "python", "terraform", "redis", "postgres", "cmux", "tmux",
	}

	for _, tool := range knownTools {
		hasInNew := strings.Contains(newCmdLower, tool) || strings.Contains(userLower, tool)
		if hasInNew {
			for i := len(cards) - 1; i >= 0; i-- {
				c := cards[i]
				cLower := strings.ToLower(c.Content + " " + c.Title)
				if strings.Contains(cLower, tool) && c.Status != card.StatusInvalidated {
					return c
				}
			}
		}
	}

	// 2. Check for explicit correction/follow-up signals targeting the most recent active card
	isCorrection := strings.Contains(userLower, "error") ||
		strings.Contains(userLower, "failed") ||
		strings.Contains(userLower, "fix") ||
		strings.Contains(userLower, "instead") ||
		strings.Contains(userLower, "change") ||
		strings.Contains(userLower, "update") ||
		strings.Contains(userLower, "switch") ||
		strings.Contains(userLower, "replace") ||
		strings.Contains(userLower, "still")

	if isCorrection {
		for i := len(cards) - 1; i >= 0; i-- {
			if cards[i].Status != card.StatusInvalidated {
				return cards[i]
			}
		}
	}

	return nil
}
