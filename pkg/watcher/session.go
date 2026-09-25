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
	ID        string    `json:"id"`
	ShortID   string    `json:"short_id"`
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	ModTime   time.Time `json:"mod_time"`
}

// ListSessions discovers all Antigravity conversation sessions sorted by most recent.
func ListSessions(limit int) ([]SessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	pattern := filepath.Join(home, ".gemini", "antigravity-cli", "brain", "*", ".system_generated", "logs", "transcript.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var sessions []SessionInfo

	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}

		// Extract session UUID from path: .../brain/<UUID>/.system_generated/...
		parts := strings.Split(m, string(filepath.Separator))
		sessionID := "unknown"
		for i, p := range parts {
			if p == "brain" && i+1 < len(parts) {
				sessionID = parts[i+1]
				break
			}
		}

		shortID := sessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		title := extractSessionTitle(m)
		if strings.HasPrefix(title, "Return ONLY a 2 to 4 word") || strings.Contains(title, "describing the topic") {
			continue
		}

		sessions = append(sessions, SessionInfo{
			ID:      sessionID,
			ShortID: shortID,
			Title:   title,
			Path:    m,
			ModTime: info.ModTime(),
		})
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

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rec TranscriptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}

		if rec.Type == "USER_INPUT" && rec.Content != "" {
			text := rec.Content
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

// FindSessionByID matches a session by exact UUID or prefix.
func FindSessionByID(query string) (*SessionInfo, error) {
	sessions, err := ListSessions(50)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	for _, s := range sessions {
		if strings.ToLower(s.ID) == queryLower || strings.HasPrefix(strings.ToLower(s.ID), queryLower) {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("no session found matching ID or prefix '%s'", query)
}
