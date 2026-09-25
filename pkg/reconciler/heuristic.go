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

	toolWordRegexes = func() map[string]*regexp.Regexp {
		tools := []string{
			"aws", "docker", "kubectl", "curl", "git", "go", "golang", "npm", "pnpm", "yarn",
			"cargo", "python", "terraform", "redis", "postgres", "cmux", "tmux",
		}
		m := make(map[string]*regexp.Regexp, len(tools))
		for _, t := range tools {
			m[t] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(t) + `\b`)
		}
		return m
	}()

	correctionRegex   = regexp.MustCompile(`(?i)\b(error|failed|fails|failing|fix|fixed|instead|instead of|switch to|switch from|still getting|still failing)\b|\bchange\s+(the\s+)?(region|port|flag|subnet|image|version|config|cluster|user|password|name|directory|path|env)\b`)
	continuationRegex = regexp.MustCompile(`(?i)\b((this|that|the|previous)\s+(step|procedure|command|script|card|snippet|config)|step\s+\d+|same\s+(command|script|step))\b`)
	invalidationRegex = regexp.MustCompile(`(?i)\b(don't use|do not use|deprecated|disproven|obsolete|drop this|invalid)\b`)
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
	var cardID string
	for {
		h.counter++
		candidate := fmt.Sprintf("card-%d", h.counter)
		exists := false
		for _, c := range currentCards {
			if c.ID == candidate {
				exists = true
				break
			}
		}
		if !exists {
			cardID = candidate
			break
		}
	}
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

	hasCorrection := correctionRegex.MatchString(userLower)
	hasContinuation := continuationRegex.MatchString(userLower)
	hasInvalidation := invalidationRegex.MatchString(userLower)

	// 1. If user indicates a correction, adjustment, continuation, or invalidation:
	if hasCorrection || hasContinuation || hasInvalidation {
		// A. First look for domain tool match using strict word boundaries
		for _, re := range toolWordRegexes {
			if re.MatchString(newCmdLower) || re.MatchString(userLower) {
				for i := len(cards) - 1; i >= 0; i-- {
					c := cards[i]
					if c.Status != card.StatusInvalidated && (re.MatchString(c.Content) || re.MatchString(c.Title)) {
						return c
					}
				}
			}
		}

		// B. If no specific tool matched, target the most recent active card
		for i := len(cards) - 1; i >= 0; i-- {
			if cards[i].Status != card.StatusInvalidated {
				return cards[i]
			}
		}
	}

	// 2. Check for explicit command similarity (e.g. updating the same CLI invocation)
	newFirstCmd := extractFirstCommandLine(newCommands)
	if newFirstCmd != "" && len(newFirstCmd) >= 8 {
		for i := len(cards) - 1; i >= 0; i-- {
			c := cards[i]
			if c.Status == card.StatusInvalidated {
				continue
			}
			cardFirstCmd := extractFirstCommandLine(c.Content)
			if cardFirstCmd != "" && (strings.HasPrefix(newFirstCmd, cardFirstCmd) || strings.HasPrefix(cardFirstCmd, newFirstCmd)) {
				return c
			}
		}
	}

	return nil
}

func extractFirstCommandLine(content string) string {
	matches := fenceRegex.FindStringSubmatch(content)
	body := content
	if len(matches) > 2 {
		body = matches[2]
	}
	lines := strings.Split(body, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "//") {
			return strings.ToLower(l)
		}
	}
	return ""
}
