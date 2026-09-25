package card

import (
	"sync"
	"time"
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
	c.Version = 1
	c.Status = StatusActive
	s.counter++
	if c.Number == 0 {
		c.Number = s.counter
	}
	c.History = []CardVersion{
		{
			Version:   1,
			Content:   c.Content,
			Reason:    "Initial extraction",
			UpdatedAt: now,
		},
	}

	s.cards[c.ID] = c
	s.order = append(s.order, c.ID)
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
