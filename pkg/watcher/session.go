package watcher

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SessionInfo struct {
	ID      string    `json:"id"`
	ShortID string    `json:"short_id"`
	Title   string    `json:"title"`
	Path    string    `json:"path"`
	ModTime time.Time `json:"mod_time"`
	Agent   string    `json:"agent"` // "antigravity", "claude", "pi", "custom", "local", "file"
}

type discoverySource struct {
	agent       string
	patterns    []string
	idExtractor func(path string) (id, shortID string)
}

// ListSessions discovers all conversation sessions from Antigravity, Claude Code,
// Pi Agent, and local project environments, sorted by most recent.
func ListSessions(limit int) ([]SessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	sources := []discoverySource{
		{
			agent: "antigravity",
			patterns: []string{
				filepath.Join(home, ".gemini", "antigravity-cli", "brain", "*", ".system_generated", "logs", "transcript.jsonl"),
			},
			idExtractor: func(path string) (string, string) {
				parts := strings.Split(path, string(filepath.Separator))
				for i, p := range parts {
					if p == "brain" && i+1 < len(parts) {
						id := parts[i+1]
						shortID := id
						if len(shortID) > 8 {
							shortID = shortID[:8]
						}
						return id, shortID
					}
				}
				return "unknown", "unknown"
			},
		},
		{
			agent: "claude",
			patterns: []string{
				filepath.Join(home, ".claude", "projects", "*", "*.jsonl"),
				filepath.Join(home, ".claude", "sessions", "*.jsonl"),
				filepath.Join(home, ".config", "claude", "projects", "*", "*.jsonl"),
			},
			idExtractor: func(path string) (string, string) {
				base := filepath.Base(path)
				id := strings.TrimSuffix(base, filepath.Ext(base))
				shortID := id
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				return id, shortID
			},
		},
		{
			agent: "pi",
			patterns: []string{
				filepath.Join(home, ".pi", "sessions", "*.jsonl"),
				filepath.Join(home, ".pi", "agent", "sessions", "*.jsonl"),
				filepath.Join(home, ".config", "pi", "sessions", "*.jsonl"),
			},
			idExtractor: func(path string) (string, string) {
				base := filepath.Base(path)
				id := strings.TrimSuffix(base, filepath.Ext(base))
				shortID := id
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				return id, shortID
			},
		},
	}

	// Also discover custom directory if configured in env
	if customDir := os.Getenv("SYL_TRANSCRIPT_DIR"); customDir != "" {
		sources = append(sources, discoverySource{
			agent: "custom",
			patterns: []string{
				filepath.Join(customDir, "*.jsonl"),
				filepath.Join(customDir, "*", "*.jsonl"),
			},
			idExtractor: func(path string) (string, string) {
				base := filepath.Base(path)
				id := strings.TrimSuffix(base, filepath.Ext(base))
				shortID := id
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				return id, shortID
			},
		})
	}

	seenPaths := make(map[string]bool)
	var sessions []SessionInfo

	for _, src := range sources {
		for _, pat := range src.patterns {
			matches, err := filepath.Glob(pat)
			if err != nil {
				continue
			}

			for _, m := range matches {
				if seenPaths[m] {
					continue
				}
				seenPaths[m] = true

				info, err := os.Stat(m)
				if err != nil || info.IsDir() {
					continue
				}

				id, shortID := src.idExtractor(m)
				title := extractSessionTitle(m)
				if strings.HasPrefix(title, "Return ONLY a 2 to 4 word") || strings.Contains(title, "describing the topic") {
					continue
				}

				sessions = append(sessions, SessionInfo{
					ID:      id,
					ShortID: shortID,
					Title:   title,
					Path:    m,
					ModTime: info.ModTime(),
					Agent:   src.agent,
				})
			}
		}
	}

	// Check local current working directory for transcript.jsonl
	for _, localPath := range []string{"transcript.jsonl", filepath.Join(".syl", "transcript.jsonl")} {
		if abs, err := filepath.Abs(localPath); err == nil && !seenPaths[abs] {
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				seenPaths[abs] = true
				title := extractSessionTitle(abs)
				sessions = append(sessions, SessionInfo{
					ID:      "local",
					ShortID: "local",
					Title:   title,
					Path:    abs,
					ModTime: info.ModTime(),
					Agent:   "local",
				})
			}
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

func extractSessionTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "Untitled session"
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var rec TranscriptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}

		if rec.IsUser() && rec.GetContent() != "" {
			text := rec.GetContent()
			if matches := userRequestRegex.FindStringSubmatch(text); len(matches) > 1 {
				text = strings.TrimSpace(matches[1])
			}
			lines := strings.Split(text, "\n")
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if l != "" {
					if len(l) > 60 {
						return l[:57] + "..."
					}
					return l
				}
			}
		}
	}

	return "Empty session"
}

// FormatTimeAgo formats a time difference into a human readable string.
func FormatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Format("Jan 02 15:04")
}

// DetectAgentFromPath attempts to infer the AI agent from a transcript path.
func DetectAgentFromPath(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(lower, "antigravity") || strings.Contains(lower, ".gemini") {
		return "antigravity"
	}
	if strings.Contains(lower, ".claude") || strings.Contains(lower, "claude") {
		return "claude"
	}
	if strings.Contains(lower, ".pi") || strings.Contains(lower, "/pi/") {
		return "pi"
	}
	return "file"
}

// FindSessionByID matches a session by exact UUID, prefix, or direct file path.
func FindSessionByID(query string) (*SessionInfo, error) {
	// 1. Direct file path check
	if stat, err := os.Stat(query); err == nil && !stat.IsDir() {
		abs, err := filepath.Abs(query)
		if err == nil {
			base := filepath.Base(abs)
			id := strings.TrimSuffix(base, filepath.Ext(base))
			shortID := id
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			return &SessionInfo{
				ID:      id,
				ShortID: shortID,
				Title:   extractSessionTitle(abs),
				Path:    abs,
				ModTime: stat.ModTime(),
				Agent:   DetectAgentFromPath(abs),
			}, nil
		}
	}

	// 2. Discover sessions from supported agents
	sessions, err := ListSessions(100)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	for _, s := range sessions {
		if strings.ToLower(s.ID) == queryLower || strings.ToLower(s.ShortID) == queryLower || strings.HasPrefix(strings.ToLower(s.ID), queryLower) {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("no session found matching ID or prefix '%s'", query)
}
