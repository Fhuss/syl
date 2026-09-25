package watcher

import "time"

// Turn represents a single request-response exchange between user and AI agent.
type Turn struct {
	ID        int       `json:"id"`
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Timestamp time.Time `json:"timestamp"`
}
