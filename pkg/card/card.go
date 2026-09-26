package card

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Fhuss/syl/internal/atomicfile"
)

type CardType string

const (
	TypeProcedure CardType = "PROCEDURE"
	TypeCommand   CardType = "COMMAND"
	TypeDecision  CardType = "DECISION"
	TypeQuestion  CardType = "QUESTION"
	TypeInfo      CardType = "INFO"
)

type CardStatus string

const (
	StatusActive      CardStatus = "active"
	StatusRevised     CardStatus = "revised"
	StatusInvalidated CardStatus = "invalidated"
)

type CardVersion struct {
	Version   int       `json:"version"`
	Content   string    `json:"content"`
	Reason    string    `json:"reason"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Card struct {
	ID        string        `json:"id"`
	Type      CardType      `json:"type"`
	Title     string        `json:"title"`
	Content   string        `json:"content"`
	Status    CardStatus    `json:"status"`
	Version   int           `json:"version"`
	Number    int           `json:"number"`
	Pinned    bool          `json:"pinned"`
	History   []CardVersion `json:"history"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type Store struct {
	mu      sync.RWMutex
	cards   map[string]*Card
	order   []string // preserve insertion order
	counter int
}

func NewStore() *Store {
	return &Store{
		cards: make(map[string]*Card),
		order: make([]string, 0),
	}
}

// Add creates or inserts a new card.
func (s *Store) Add(c *Card) *Card {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Status == "" {
		c.Status = StatusActive
	}

	if c.Number > s.counter {
		s.counter = c.Number
	} else if c.Number == 0 {
		s.counter++
		c.Number = s.counter
	}

	if len(c.History) == 0 {
		c.History = []CardVersion{
			{
				Version:   c.Version,
				Content:   c.Content,
				Reason:    "Initial extraction",
				UpdatedAt: now,
			},
		}
	}

	// Ensure card has an ID and it doesn't collide with existing cards
	if c.ID == "" {
		c.ID = fmt.Sprintf("card-%d", c.Number)
	}
	origID := c.ID
	collisionCount := 0
	for {
		if _, exists := s.cards[c.ID]; !exists {
			break
		}
		collisionCount++
		c.ID = fmt.Sprintf("%s-%d", origID, collisionCount)
	}

	s.cards[c.ID] = c

	alreadyInOrder := false
	for _, id := range s.order {
		if id == c.ID {
			alreadyInOrder = true
			break
		}
	}
	if !alreadyInOrder {
		s.order = append(s.order, c.ID)
	}

	return c
}

// Update patches an existing card and increments its version.
func (s *Store) Update(id, newContent, reason string) (*Card, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, exists := s.cards[id]
	if !exists {
		return nil, false
	}

	c.Version++
	c.Content = newContent
	c.Status = StatusRevised
	c.UpdatedAt = time.Now()
	c.History = append(c.History, CardVersion{
		Version:   c.Version,
		Content:   newContent,
		Reason:    reason,
		UpdatedAt: c.UpdatedAt,
	})

	return c, true
}

// Invalidate marks a card as disproven or superseded.
func (s *Store) Invalidate(id, reason string) (*Card, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, exists := s.cards[id]
	if !exists {
		return nil, false
	}

	c.Status = StatusInvalidated
	c.UpdatedAt = time.Now()
	c.History = append(c.History, CardVersion{
		Version:   c.Version,
		Content:   c.Content,
		Reason:    "Invalidated: " + reason,
		UpdatedAt: c.UpdatedAt,
	})

	return c, true
}

// TogglePin toggles the pin status of a card.
func (s *Store) TogglePin(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, exists := s.cards[id]
	if !exists {
		return false
	}
	c.Pinned = !c.Pinned
	return c.Pinned
}

// UpdateTitle updates the title of a card in place without bumping its version.
func (s *Store) UpdateTitle(id, newTitle string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, exists := s.cards[id]
	if !exists {
		return false
	}
	c.Title = newTitle
	return true
}

// Remove deletes a card from the store.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.cards[id]; !exists {
		return false
	}
	delete(s.cards, id)

	newOrder := make([]string, 0, len(s.order)-1)
	for _, oid := range s.order {
		if oid != id {
			newOrder = append(newOrder, oid)
		}
	}
	s.order = newOrder
	return true
}

// Get retrieves a card by ID.
func (s *Store) Get(id string) (*Card, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, exists := s.cards[id]
	return c, exists
}

// List returns all cards in order, prioritizing pinned cards.
func (s *Store) List() []*Card {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pinned := make([]*Card, 0)
	unpinned := make([]*Card, 0)

	for _, id := range s.order {
		if c, ok := s.cards[id]; ok {
			if c.Pinned {
				pinned = append(pinned, c)
			} else {
				unpinned = append(unpinned, c)
			}
		}
	}

	return append(pinned, unpinned...)
}

// ExportMarkdown formats all active and pinned cards into a clean Markdown document.
func (s *Store) ExportMarkdown(sessionLabel string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("# Syl Reference Notes\n\n")
	if sessionLabel != "" {
		sb.WriteString(fmt.Sprintf("> **Session**: `%s` | **Exported**: %s\n\n", sessionLabel, time.Now().Format("2006-01-02 15:04:05")))
	} else {
		sb.WriteString(fmt.Sprintf("> **Exported**: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	}

	pinned := make([]*Card, 0)
	unpinned := make([]*Card, 0)
	for _, id := range s.order {
		if c, ok := s.cards[id]; ok {
			if c.Pinned {
				pinned = append(pinned, c)
			} else {
				unpinned = append(unpinned, c)
			}
		}
	}
	cards := append(pinned, unpinned...)

	if len(cards) == 0 {
		sb.WriteString("*No reference cards saved in this session.*\n")
		return sb.String()
	}

	for _, c := range cards {
		pinBadge := ""
		if c.Pinned {
			pinBadge = " 📌"
		}
		statusBadge := ""
		if c.Status == StatusInvalidated {
			statusBadge = " [INVALIDATED]"
		}
		sb.WriteString(fmt.Sprintf("## %d. %s (v%d) [%s]%s%s\n\n", c.Number, c.Title, c.Version, c.Type, pinBadge, statusBadge))
		sb.WriteString(c.Content)
		sb.WriteString("\n\n")
		if len(c.History) > 1 {
			sb.WriteString("<details><summary>Revision History</summary>\n\n")
			for _, h := range c.History {
				sb.WriteString(fmt.Sprintf("- **v%d** (%s): %s\n", h.Version, h.UpdatedAt.Format("15:04:05"), h.Reason))
			}
			sb.WriteString("\n</details>\n\n")
		}
		sb.WriteString("---\n\n")
	}

	return sb.String()
}

// SessionSnapshot represents a complete serialized dump of the session cards and progress.
type SessionSnapshot struct {
	SessionID         string    `json:"session_id"`
	LastTurnProcessed int       `json:"last_turn_processed"`
	UpdatedAt         time.Time `json:"updated_at"`
	Cards             []*Card   `json:"cards"`
}

// SessionFilePath returns the default path ~/.config/syl/sessions/<sessionID>.json, or respects SYL_CONFIG_DIR if set.
func SessionFilePath(sessionID string) (string, error) {
	var dir string
	if custom := os.Getenv("SYL_CONFIG_DIR"); custom != "" {
		dir = filepath.Join(custom, "sessions")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		dir = filepath.Join(home, ".config", "syl", "sessions")
	}
	if sessionID == "" {
		sessionID = "default"
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	// Tighten directories created by older Syl versions as well: this also
	// protects snapshots not yet rewritten with the new private file mode.
	// Do not change the permissions of a user-selected symlink destination.
	if info, err := os.Lstat(dir); err != nil {
		return "", err
	} else if info.Mode()&os.ModeSymlink == 0 {
		if err := os.Chmod(dir, 0700); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s.json", sessionID)), nil
}

// SaveSnapshot serializes the store and its cards to disk.
func (s *Store) SaveSnapshot(filePath string, sessionID string, lastTurnProcessed int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cards := make([]*Card, 0, len(s.order))
	for _, id := range s.order {
		if c, ok := s.cards[id]; ok {
			cards = append(cards, c)
		}
	}

	snap := SessionSnapshot{
		SessionID:         sessionID,
		LastTurnProcessed: lastTurnProcessed,
		UpdatedAt:         time.Now(),
		Cards:             cards,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	return atomicfile.Write(filePath, data)
}

// LoadSnapshot loads cards and state from a session snapshot file.
func (s *Store) LoadSnapshot(filePath string) (*SessionSnapshot, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var snap SessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cards = make(map[string]*Card)
	s.order = make([]string, 0, len(snap.Cards))
	maxNumber := 0

	for _, c := range snap.Cards {
		s.cards[c.ID] = c
		s.order = append(s.order, c.ID)
		if c.Number > maxNumber {
			maxNumber = c.Number
		}
	}
	s.counter = maxNumber

	return &snap, nil
}
