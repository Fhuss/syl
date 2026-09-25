package card

import (
	"fmt"
	"regexp"
	"strings"
)

type BlockType string

const (
	BlockCode BlockType = "CODE"
	BlockStep BlockType = "STEP"
	BlockURL  BlockType = "URL"
)

// ActionBlock represents a distinct, actionable snippet (command, step, or URL)
// extracted from card markdown content.
type ActionBlock struct {
	Index   int       `json:"index"` // 1-based index (1..9, 0)
	Key     string    `json:"key"`   // "1", "2", ... "9", "0"
	Type    BlockType `json:"type"`  // CODE, STEP, URL
	Badge   string    `json:"badge"` // e.g. "BASH", "STEP 1", "URL", "SQL"
	Preview string    `json:"preview"`
	Content string    `json:"content"`
}

var (
	codeBlockRegex = regexp.MustCompile("(?s)```([a-zA-Z0-9_\\-\\+]*)\\s*\\n(.*?)```")
	urlRegex       = regexp.MustCompile(`https?://[^\s)\]">]+`)
	numberedStepRe = regexp.MustCompile(`(?m)^\s*(?:(\d+)[\.\)]|Step\s+(\d+)[:\.]?)\s+(.+)`)
	taskListRe     = regexp.MustCompile(`(?m)^\s*-\s*\[([ xX])\]\s+(.+)`)
	inlineCmdRe    = regexp.MustCompile("`([^`]+)`")
)

// ExtractActionBlocks scans markdown content and extracts actionable blocks:
// code snippets, numbered procedure steps, and URLs.
func ExtractActionBlocks(content string) []ActionBlock {
	var blocks []ActionBlock
	seenContent := make(map[string]bool)

	// 1. Extract fenced code blocks
	matches := codeBlockRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		lang := strings.TrimSpace(match[1])
		code := strings.TrimSpace(match[2])
		if code == "" || seenContent[code] {
			continue
		}
		seenContent[code] = true

		badge := "CMD"
		if lang != "" {
			badge = strings.ToUpper(lang)
			if len(badge) > 6 {
				badge = badge[:6]
			}
		}

		firstLine := ""
		lines := strings.Split(code, "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "//") {
				firstLine = l
				break
			}
		}
		if firstLine == "" && len(lines) > 0 {
			firstLine = strings.TrimSpace(lines[0])
		}
		if len(firstLine) > 55 {
			firstLine = firstLine[:52] + "..."
		}

		blocks = append(blocks, ActionBlock{
			Type:    BlockCode,
			Badge:   badge,
			Preview: firstLine,
			Content: code,
		})
	}

	// 2. Extract numbered steps and task checklist items (outside code blocks)
	cleanWithoutCode := codeBlockRegex.ReplaceAllString(content, "")

	stepMatches := numberedStepRe.FindAllStringSubmatch(cleanWithoutCode, -1)
	for _, m := range stepMatches {
		stepNum := m[1]
		if stepNum == "" {
			stepNum = m[2]
		}
		text := strings.TrimSpace(m[3])
		if text == "" {
			continue
		}
		// If text ends with ":" and is short, it's just a header introducing a code block
		if strings.HasSuffix(text, ":") {
			continue
		}

		// If step text contains a URL and no inline backticks, let URL extractor handle it
		if urlRegex.MatchString(text) && !inlineCmdRe.MatchString(text) {
			continue
		}

		// If the step contains inline backtick command e.g. `kubectl apply...`
		cmd := text
		if cmdMatch := inlineCmdRe.FindStringSubmatch(text); len(cmdMatch) > 1 {
			cmd = strings.TrimSpace(cmdMatch[1])
		}

		if seenContent[cmd] {
			continue
		}
		seenContent[cmd] = true

		preview := text
		if len(preview) > 55 {
			preview = preview[:52] + "..."
		}

		blocks = append(blocks, ActionBlock{
			Type:    BlockStep,
			Badge:   fmt.Sprintf("STEP %s", stepNum),
			Preview: preview,
			Content: cmd,
		})
	}

	taskMatches := taskListRe.FindAllStringSubmatch(cleanWithoutCode, -1)
	for i, m := range taskMatches {
		text := strings.TrimSpace(m[2])
		if text == "" {
			continue
		}
		cmd := text
		if cmdMatch := inlineCmdRe.FindStringSubmatch(text); len(cmdMatch) > 1 {
			cmd = strings.TrimSpace(cmdMatch[1])
		}
		if seenContent[cmd] {
			continue
		}
		seenContent[cmd] = true

		preview := text
		if len(preview) > 55 {
			preview = preview[:52] + "..."
		}

		blocks = append(blocks, ActionBlock{
			Type:    BlockStep,
			Badge:   fmt.Sprintf("TASK %d", i+1),
			Preview: preview,
			Content: cmd,
		})
	}

	// 3. Extract URLs
	urlMatches := urlRegex.FindAllString(content, -1)
	for _, rawURL := range urlMatches {
		u := strings.TrimRight(rawURL, ".,;:!?'\")>")
		if u == "" || seenContent[u] {
			continue
		}
		seenContent[u] = true

		preview := u
		if strings.HasPrefix(preview, "https://") {
			preview = strings.TrimPrefix(preview, "https://")
		} else if strings.HasPrefix(preview, "http://") {
			preview = strings.TrimPrefix(preview, "http://")
		}
		if len(preview) > 55 {
			preview = preview[:52] + "..."
		}

		blocks = append(blocks, ActionBlock{
			Type:    BlockURL,
			Badge:   "URL",
			Preview: preview,
			Content: u,
		})
	}

	// Assign 1-based indices and key shortcuts
	for i := range blocks {
		idx := i + 1
		blocks[i].Index = idx
		if idx <= 9 {
			blocks[i].Key = fmt.Sprintf("%d", idx)
		} else if idx == 10 {
			blocks[i].Key = "0"
		} else {
			blocks[i].Key = ""
		}
	}

	return blocks
}
