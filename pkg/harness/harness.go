package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Harness defines an AI agent or model runner capable of summarizing turns into succinct titles.
type Harness interface {
	Name() string
	Available() bool
	Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error)
}

// CleanTitle strips formatting, quotes, and conversational prefixes, ensuring a concise title.
func CleanTitle(raw string) string {
	t := strings.TrimSpace(raw)
	// If output contains multiple lines, take the first non-empty line
	lines := strings.Split(t, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "*(") && !strings.HasPrefix(l, "(") {
			t = l
			break
		}
	}

	t = strings.Trim(t, "\"`'*_# ")
	for _, prefix := range []string{"Title:", "Topic:", "Subject:", "Summary:"} {
		if strings.HasPrefix(strings.ToLower(t), strings.ToLower(prefix)) {
			t = strings.TrimSpace(t[len(prefix):])
		}
	}
	t = strings.Trim(t, "\"`'*_# ")

	// Remove trailing periods or punctuation except ellipsis
	if !strings.HasSuffix(t, "...") {
		t = strings.TrimRight(t, ".!?;,:")
	}

	if len([]rune(t)) > 36 {
		runes := []rune(t)
		t = string(runes[:33]) + "..."
	}
	return t
}

func buildPrompt(userPrompt, assistantContent string) string {
	u := strings.TrimSpace(userPrompt)
	var ctxSnippet string
	if len(u) < 15 || isGenericPrompt(u) {
		lines := strings.Split(strings.TrimSpace(assistantContent), "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "```") {
				if len(l) > 120 {
					l = l[:117] + "..."
				}
				ctxSnippet = l
				break
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Return ONLY a 2 to 4 word plain text title (no markdown, no bold, no quotes, no extra words) describing the topic of this developer request:\n")
	sb.WriteString("User: " + u + "\n")
	if ctxSnippet != "" {
		sb.WriteString("Context: " + ctxSnippet + "\n")
	}
	return sb.String()
}

func isGenericPrompt(p string) bool {
	lower := strings.ToLower(strings.TrimSpace(p))
	switch lower {
	case "yes", "no", "ok", "okay", "let's try it", "try it", "go ahead", "continue", "next", "done", "thanks", "sure":
		return true
	}
	return false
}

// -------------------------------------------------------------------------
// 1. Antigravity CLI Harness (agy)
// -------------------------------------------------------------------------

type AgyHarness struct {
	Model string
}

func findAgyPath() (string, error) {
	if p, err := exec.LookPath("agy"); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		localPath := filepath.Join(home, ".local", "bin", "agy")
		if _, err := os.Stat(localPath); err == nil {
			return localPath, nil
		}
	}
	return "", fmt.Errorf("agy executable not found")
}

func NewAgyHarness(model string) *AgyHarness {
	return &AgyHarness{Model: model}
}

func (h *AgyHarness) Name() string {
	return "agy"
}

func (h *AgyHarness) Available() bool {
	_, err := findAgyPath()
	return err == nil
}

func (h *AgyHarness) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	agyPath, err := findAgyPath()
	if err != nil {
		return "", err
	}

	prompt := buildPrompt(userPrompt, assistantContent)
	args := []string{"--effort", "low"}
	if h.Model != "" {
		args = append(args, "--model", h.Model)
	}
	args = append(args, "-p", prompt)

	cmd := exec.CommandContext(ctx, agyPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("agy execution error: %w", err)
	}

	res := CleanTitle(string(out))
	if res == "" {
		return "", fmt.Errorf("agy returned empty title")
	}
	return res, nil
}

// -------------------------------------------------------------------------
// 2. Claude Code CLI Harness (claude)
// -------------------------------------------------------------------------

func findClaudePath() (string, error) {
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		for _, sub := range []string{".local/bin/claude"} {
			localPath := filepath.Join(home, sub)
			if _, err := os.Stat(localPath); err == nil {
				return localPath, nil
			}
		}
	}
	return "", fmt.Errorf("claude executable not found")
}

type ClaudeHarness struct {
	Model string
}

func NewClaudeHarness(model string) *ClaudeHarness {
	return &ClaudeHarness{Model: model}
}

func (h *ClaudeHarness) Name() string {
	return "claude"
}

func (h *ClaudeHarness) Available() bool {
	_, err := findClaudePath()
	return err == nil
}

func (h *ClaudeHarness) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	claudePath, err := findClaudePath()
	if err != nil {
		return "", err
	}

	prompt := buildPrompt(userPrompt, assistantContent)
	args := []string{"-p", prompt}
	if h.Model != "" {
		args = append(args, "--model", h.Model)
	}

	cmd := exec.CommandContext(ctx, claudePath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude execution error: %w", err)
	}

	res := CleanTitle(string(out))
	if res == "" {
		return "", fmt.Errorf("claude returned empty title")
	}
	return res, nil
}

// -------------------------------------------------------------------------
// 3. Pi Agent CLI Harness (pi)
// -------------------------------------------------------------------------

type PiHarness struct {
	Model string
}

func NewPiHarness(model string) *PiHarness {
	return &PiHarness{Model: model}
}

func (h *PiHarness) Name() string {
	return "pi"
}

func (h *PiHarness) Available() bool {
	_, err := exec.LookPath("pi")
	return err == nil
}

func (h *PiHarness) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	piPath, err := exec.LookPath("pi")
	if err != nil {
		return "", err
	}

	prompt := buildPrompt(userPrompt, assistantContent)
	args := []string{"--no-session", "-p", prompt}
	if h.Model != "" {
		args = append(args, "--model", h.Model)
	}

	cmd := exec.CommandContext(ctx, piPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pi execution error: %w", err)
	}

	res := CleanTitle(string(out))
	if res == "" {
		return "", fmt.Errorf("pi returned empty title")
	}
	return res, nil
}

// -------------------------------------------------------------------------
// 4. Ollama Local LLM Harness (ollama)
// -------------------------------------------------------------------------

type OllamaHarness struct {
	Model string
}

func NewOllamaHarness(model string) *OllamaHarness {
	if model == "" {
		model = "llama3.2"
	}
	return &OllamaHarness{Model: model}
}

func (h *OllamaHarness) Name() string {
	return "ollama"
}

func (h *OllamaHarness) Available() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}

func (h *OllamaHarness) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		return "", err
	}

	prompt := buildPrompt(userPrompt, assistantContent)
	cmd := exec.CommandContext(ctx, ollamaPath, "run", h.Model, prompt)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ollama execution error: %w", err)
	}

	res := CleanTitle(string(out))
	if res == "" {
		return "", fmt.Errorf("ollama returned empty title")
	}
	return res, nil
}

// -------------------------------------------------------------------------
// 5. Smart Rule-Based Heuristic Harness (Pure Go Fallback)
// -------------------------------------------------------------------------

var (
	politeRegex        = regexp.MustCompile(`(?i)^(can you please|could you please|please|how do i|how can i|how to|why did you|i think|let's|let us|is it possible to|i want to|i need to|what if we|would it be possible to|so can you|can you|could you|one thing i'm missing is|one thing i'm|try out now i'm asking about|i'm asking about|are we still doing|how does it)\s+`)
	headingPrefixRegex = regexp.MustCompile(`(?m)^#{1,3}\s+(?:\d+\.\s*)?([^\n]+)`)
	cmdWordRegex       = regexp.MustCompile(`(?i)\b(docker|kubectl|aws|git|go|npm|pnpm|yarn|cargo|python|terraform|redis|postgres|syl|cmux|tmux)\b.*`)
)

type HeuristicHarness struct{}

func NewHeuristicHarness() *HeuristicHarness {
	return &HeuristicHarness{}
}

func (h *HeuristicHarness) Name() string {
	return "heuristic"
}

func (h *HeuristicHarness) Available() bool {
	return true
}

func (h *HeuristicHarness) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	lines := strings.Split(strings.TrimSpace(userPrompt), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "Reference Note", nil
	}

	firstLine := strings.TrimSpace(lines[0])
	stripped := politeRegex.ReplaceAllString(firstLine, "")
	stripped = strings.TrimSpace(stripped)

	// Check if assistant response has a clean heading
	if headingMatch := headingPrefixRegex.FindStringSubmatch(assistantContent); len(headingMatch) > 1 {
		heading := strings.TrimSpace(headingMatch[1])
		if heading != "" && len(heading) <= 45 && !strings.HasPrefix(heading, "Context") {
			return CleanTitle(heading), nil
		}
	}

	// Inspect assistant commands
	if len(stripped) < 8 || isGenericPrompt(stripped) {
		for _, l := range strings.Split(assistantContent, "\n") {
			l = strings.TrimSpace(l)
			if cmdMatch := cmdWordRegex.FindString(l); cmdMatch != "" {
				parts := strings.Fields(cmdMatch)
				if len(parts) >= 2 {
					return CleanTitle(strings.Title(parts[0]) + " " + strings.Title(parts[1])), nil
				}
			}
		}
	}

	if stripped == "" {
		stripped = firstLine
	}

	// Limit to ~4 words
	words := strings.Fields(stripped)
	if len(words) > 5 {
		stripped = strings.Join(words[:4], " ") + "..."
	}

	return CleanTitle(stripped), nil
}

// -------------------------------------------------------------------------
// 6. Cascading Auto Harness
// -------------------------------------------------------------------------

type AutoHarness struct {
	harnesses []Harness
}

func NewAutoHarness(preferred string, modelOverride string) *AutoHarness {
	var rawCandidates []Harness

	// Add preferred harness first if specified
	switch strings.ToLower(preferred) {
	case "agy", "antigravity":
		rawCandidates = append(rawCandidates, NewAgyHarness(modelOverride))
	case "claude", "claude-code":
		rawCandidates = append(rawCandidates, NewClaudeHarness(modelOverride))
	case "pi":
		rawCandidates = append(rawCandidates, NewPiHarness(modelOverride))
	case "ollama":
		rawCandidates = append(rawCandidates, NewOllamaHarness(modelOverride))
	case "heuristic":
		return &AutoHarness{harnesses: []Harness{NewHeuristicHarness()}}
	}

	// Add other supported CLI harnesses in order of preference
	rawCandidates = append(rawCandidates,
		NewAgyHarness(modelOverride),
		NewClaudeHarness(modelOverride),
		NewPiHarness(modelOverride),
		NewOllamaHarness(modelOverride),
		NewHeuristicHarness(),
	)

	seen := make(map[string]bool)
	var candidates []Harness
	for _, c := range rawCandidates {
		name := c.Name()
		if !seen[name] {
			seen[name] = true
			candidates = append(candidates, c)
		}
	}

	return &AutoHarness{harnesses: candidates}
}

func (h *AutoHarness) Name() string {
	for _, cand := range h.harnesses {
		if cand.Available() {
			return cand.Name()
		}
	}
	return "heuristic"
}

func (h *AutoHarness) Available() bool {
	return true
}

func (h *AutoHarness) Summarize(ctx context.Context, userPrompt, assistantContent string) (string, error) {
	for _, cand := range h.harnesses {
		if cand.Available() {
			subCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
			title, err := cand.Summarize(subCtx, userPrompt, assistantContent)
			cancel()
			if err == nil && title != "" {
				return title, nil
			}
		}
	}
	// Fallback to pure heuristic
	return NewHeuristicHarness().Summarize(ctx, userPrompt, assistantContent)
}
