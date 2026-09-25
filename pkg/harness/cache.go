package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TitleCache persists AI-summarized card titles to disk so that restarts
// never revert cards back to raw truncated prompts.
type TitleCache struct {
	mu       sync.RWMutex
	filePath string
	titles   map[string]string
}

var (
	defaultCache *TitleCache
	cacheOnce    sync.Once
)

// GetTitleCache returns the singleton cache instance stored in ~/.config/syl/titles.json.
func GetTitleCache() *TitleCache {
	cacheOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		dir := filepath.Join(home, ".config", "syl")
		_ = os.MkdirAll(dir, 0755)
		defaultCache = NewTitleCache(filepath.Join(dir, "titles.json"))
	})
	return defaultCache
}

func NewTitleCache(filePath string) *TitleCache {
	tc := &TitleCache{
		filePath: filePath,
		titles:   make(map[string]string),
	}
	tc.load()
	return tc
}

func (tc *TitleCache) load() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	data, err := os.ReadFile(tc.filePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &tc.titles)
	if tc.titles == nil {
		tc.titles = make(map[string]string)
	}
}

func (tc *TitleCache) save() {
	if tc.filePath == "" {
		return
	}
	data, err := json.MarshalIndent(tc.titles, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(tc.filePath, data, 0644)
}

// IsDefinitiveTitle checks whether a title is a meaningful topic summary
// rather than an empty string or a truncated prompt fallback ending in "...".
func IsDefinitiveTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return false
	}
	if strings.HasSuffix(t, "...") {
		return false
	}
	if strings.HasPrefix(strings.ToLower(t), "turn #") {
		return false
	}
	return true
}

func (tc *TitleCache) Get(sessionID string, turnID int) (string, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	key := makeKey(sessionID, turnID)
	t, ok := tc.titles[key]
	return t, ok
}

func (tc *TitleCache) Set(sessionID string, turnID int, title string) {
	if title == "" {
		return
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()

	key := makeKey(sessionID, turnID)
	// Do not overwrite an existing definitive title with a truncated prompt fallback
	if existing, ok := tc.titles[key]; ok && IsDefinitiveTitle(existing) && !IsDefinitiveTitle(title) {
		return
	}
	tc.titles[key] = title
	tc.save()
}

func (tc *TitleCache) GetByPrompt(prompt string) (string, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	h := hashPrompt(prompt)
	t, ok := tc.titles["p:"+h]
	return t, ok
}

func (tc *TitleCache) SetByPrompt(prompt, title string) {
	if title == "" || strings.TrimSpace(prompt) == "" {
		return
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()

	h := hashPrompt(prompt)
	key := "p:" + h
	// Do not overwrite an existing definitive title with a truncated prompt fallback
	if existing, ok := tc.titles[key]; ok && IsDefinitiveTitle(existing) && !IsDefinitiveTitle(title) {
		return
	}
	tc.titles[key] = title
	tc.save()
}

func makeKey(sessionID string, turnID int) string {
	if sessionID == "" {
		sessionID = "default"
	}
	return fmt.Sprintf("%s:%d", sessionID, turnID)
}

func hashPrompt(prompt string) string {
	cleaned := strings.ToLower(strings.TrimSpace(prompt))
	h := sha256.Sum256([]byte(cleaned))
	return hex.EncodeToString(h[:8])
}
