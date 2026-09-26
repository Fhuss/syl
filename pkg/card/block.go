package card

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type BlockType string

const (
	BlockCode BlockType = "CODE"
	BlockStep BlockType = "STEP"
	BlockURL  BlockType = "URL"
	BlockFile BlockType = "FILE"
)

// ActionBlock represents a distinct, actionable snippet (command, step, URL, or file)
// extracted from card markdown content.
type ActionBlock struct {
	Index   int       `json:"index"` // 1-based index (1..9, 0)
	Key     string    `json:"key"`   // "1", "2", ... "9", "0"
	Type    BlockType `json:"type"`  // CODE, STEP, URL, FILE
	Badge   string    `json:"badge"` // e.g. "BASH", "STEP 1", "URL", "SQL", "FILE", "GO"
	Preview string    `json:"preview"`
	Content string    `json:"content"`
}

var (
	codeBlockRegex = regexp.MustCompile("(?s)```([a-zA-Z0-9_\\-\\+]*)\\s*\\n(.*?)```")
	urlRegex       = regexp.MustCompile(`https?://[^\s)\]">]+`)
	numberedStepRe = regexp.MustCompile(`(?m)^\s*(?:(\d+)[\.)]|Step\s+(\d+)[:.]?)\s+(.+)`)
	taskListRe     = regexp.MustCompile(`(?m)^\s*-\s*\[([ xX])\]\s+(.+)`)
	inlineCmdRe    = regexp.MustCompile("`([^`]+)`")

	// File path patterns:
	// - file:// URIs (Antigravity/IDE style): file:///Users/fabian/file.go#L10-L20
	// - Markdown file links: [filename.go](file:///path/to/file.go)
	// - Absolute paths with extensions: /Users/.../file.ext, /etc/nginx/nginx.conf
	// - Home-relative paths: ~/.config/syl/config.yaml, ~/Dev/project/main.go
	fileURIRegex  = regexp.MustCompile(`file:///[^\s)\]">#]+(?:#[^\s)\]">]*)?`)
	mdFileLinkRe  = regexp.MustCompile(`\[([^\]]*)\]\((file:///[^)]+)\)`)
	absFilePathRe = regexp.MustCompile(`(?m)(?:^|\s|` + "`" + `)((?:/[a-zA-Z0-9._-]+)+\.[a-zA-Z0-9]+(?:#L\d+(?:-L?\d+)?)?)(?:$|\s|` + "`" + `|[,;:)\]\.'\"!?])`)
	homeFileRe    = regexp.MustCompile(`(?m)(?:^|\s|` + "`" + `)(~/[^\s` + "`" + `)\]"]+\.[a-zA-Z0-9]+)(?:$|\s|` + "`" + `|[,;:)\]\.'\"!?])`)

	// Known extensions that indicate source/config files (not random paths like /etc/hosts version references)
	fileExtensions = map[string]string{
		".go": "GO", ".py": "PY", ".js": "JS", ".ts": "TS", ".tsx": "TSX", ".jsx": "JSX",
		".rs": "RUST", ".rb": "RUBY", ".java": "JAVA", ".kt": "KOTLIN", ".swift": "SWIFT",
		".c": "C", ".cpp": "C++", ".h": "C", ".cs": "C#",
		".html": "HTML", ".css": "CSS", ".scss": "SCSS", ".less": "LESS",
		".json": "JSON", ".yaml": "YAML", ".yml": "YAML", ".toml": "TOML", ".xml": "XML",
		".md": "MD", ".txt": "TXT", ".csv": "CSV", ".log": "LOG",
		".sh": "BASH", ".bash": "BASH", ".zsh": "ZSH", ".fish": "FISH",
		".sql": "SQL", ".graphql": "GQL",
		".conf": "CONF", ".cfg": "CONF", ".ini": "INI", ".env": "ENV",
		".dockerfile": "DOCKER", ".proto": "PROTO", ".tf": "TF", ".hcl": "HCL",
		".mod": "GOMOD", ".sum": "GOSUM", ".lock": "LOCK",
		".png": "IMG", ".jpg": "IMG", ".jpeg": "IMG", ".gif": "IMG", ".svg": "SVG", ".webp": "IMG",
		".pdf": "PDF",
	}
)

// resolveFilePath normalizes a raw path string to an absolute path.
// Handles file:// URIs, ~/home paths, and strips #Lnn line fragments and trailing punctuation.
func resolveFilePath(raw string) string {
	p := strings.TrimSpace(raw)
	p = strings.TrimRight(p, ".,;:!?'\">)")
	// Strip file:// prefix
	if strings.HasPrefix(p, "file://") {
		p = strings.TrimPrefix(p, "file://")
	}
	// Strip line number fragment (#L10-L20)
	if idx := strings.Index(p, "#"); idx != -1 {
		p = p[:idx]
	}
	// Expand ~/
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	return p
}

// fileBadge returns a descriptive badge based on the file extension.
func fileBadge(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if badge, ok := fileExtensions[ext]; ok {
		return badge
	}
	// Check for Dockerfile (no extension)
	base := strings.ToLower(filepath.Base(path))
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return "DOCKER"
	}
	if base == "makefile" || base == "gnumakefile" {
		return "MAKE"
	}
	return "FILE"
}

// ExtractActionBlocks scans markdown content and extracts actionable blocks:
// code snippets, numbered procedure steps, URLs, and file paths.
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
		u := strings.TrimRight(rawURL, ".,;:!?'\">")
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

	// 4. Extract file paths
	fileBlocks := extractFilePaths(content, seenContent)
	blocks = append(blocks, fileBlocks...)

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

// extractFilePaths finds file paths in content and returns FILE-type ActionBlocks.
func extractFilePaths(content string, seen map[string]bool) []ActionBlock {
	var blocks []ActionBlock

	addFile := func(raw string) {
		resolved := resolveFilePath(raw)
		if seen[resolved] {
			return
		}
		seen[resolved] = true

		badge := fileBadge(resolved)
		preview := filepath.Base(resolved)
		// Add parent dir for context if the basename is generic
		dir := filepath.Base(filepath.Dir(resolved))
		if dir != "" && dir != "." && dir != "/" {
			preview = dir + "/" + preview
		}
		if len(preview) > 55 {
			preview = preview[:52] + "..."
		}

		blocks = append(blocks, ActionBlock{
			Type:    BlockFile,
			Badge:   badge,
			Preview: preview,
			Content: resolved,
		})
	}

	// Strip code blocks to avoid matching paths inside code (they're already captured as CODE blocks)
	clean := codeBlockRegex.ReplaceAllString(content, "")

	// 4a. Markdown file links: [text](file:///path)
	for _, m := range mdFileLinkRe.FindAllStringSubmatch(clean, -1) {
		if len(m) > 2 {
			addFile(m[2])
		}
	}

	// 4b. Bare file:// URIs (not already captured by markdown links)
	for _, u := range fileURIRegex.FindAllString(clean, -1) {
		addFile(u)
	}

	// 4c. Absolute paths with file extensions: /path/to/file.ext
	for _, m := range absFilePathRe.FindAllStringSubmatch(clean, -1) {
		if len(m) > 1 {
			addFile(m[1])
		}
	}

	// 4d. Home-relative paths: ~/path/to/file.ext
	for _, m := range homeFileRe.FindAllStringSubmatch(clean, -1) {
		if len(m) > 1 {
			addFile(m[1])
		}
	}

	return blocks
}
