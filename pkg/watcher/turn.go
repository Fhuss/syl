package watcher

import "time"

// Turn represents a single completed request-response exchange between user and AI agent.
type Turn struct {
	ID        int       `json:"id"`
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Timestamp time.Time `json:"timestamp"`
}

// AgentActivity represents real-time execution state of the agent while a turn is in flight.
type AgentActivity struct {
	IsBusy    bool      `json:"is_busy"`
	Prompt    string    `json:"prompt"`
	Tool      string    `json:"tool"`
	Timestamp time.Time `json:"timestamp"`
}
