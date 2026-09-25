package reconciler

import (
	"context"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/watcher"
)

type OpType string

const (
	OpAdd        OpType = "ADD"
	OpUpdate     OpType = "UPDATE"
	OpInvalidate OpType = "INVALIDATE"
	OpNoop       OpType = "NOOP"
)

// Mutation represents an atomic change applied to the live reference cards.
type Mutation struct {
	Op       OpType        `json:"op"`
	CardID   string        `json:"card_id"`
	Type     card.CardType `json:"type,omitempty"`
	Title    string        `json:"title,omitempty"`
	Content  string        `json:"content,omitempty"`
	Reason   string        `json:"reason"`
}

// Reconciler analyzes current cards against a new conversation turn and emits mutations.
type Reconciler interface {
	Reconcile(ctx context.Context, currentCards []*card.Card, turn *watcher.Turn) ([]Mutation, error)
}
