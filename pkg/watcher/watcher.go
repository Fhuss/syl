package watcher

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

var userRequestRegex = regexp.MustCompile(`(?s)<USER_REQUEST>(.*?)</USER_REQUEST>`)

// TranscriptRecord represents a single JSON line in an Antigravity, Claude Code, or generic agent transcript.
type TranscriptRecord struct {
	StepIndex  int    `json:"step_index"`
	Source     string `json:"source"`
	Type       string `json:"type"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	StopReason string `json:"stopReason"`
	Content    string `json:"content"`
	Text       string `json:"text"`
	Message    string `json:"message"`
	CreatedAt  string `json:"created_at"`
	ToolCalls  []struct {
		Name string `json:"name"`
	} `json:"tool_calls"`
}

func (r *TranscriptRecord) UnmarshalJSON(data []byte) error {
	type Alias TranscriptRecord
	var raw struct {
		Alias
		RawContent json.RawMessage `json:"content"`
		RawMessage json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = TranscriptRecord(raw.Alias)

	// 1. Process RawContent if string or array of blocks
	if len(raw.RawContent) > 0 {
		var s string
		if err := json.Unmarshal(raw.RawContent, &s); err == nil {
			r.Content = s
		} else {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw.RawContent, &blocks); err == nil {
				var sb strings.Builder
				for _, b := range blocks {
					if b.Text != "" {
						if sb.Len() > 0 {
							sb.WriteString("\n")
						}
						sb.WriteString(b.Text)
					}
				}
				r.Content = sb.String()
			}
		}
	}

	// 2. Process RawMessage if string or nested message object (Claude Code / Pi)
	if len(raw.RawMessage) > 0 {
		var s string
		if err := json.Unmarshal(raw.RawMessage, &s); err == nil {
			r.Message = s
		} else {
			var nested struct {
				Role       string          `json:"role"`
				StopReason string          `json:"stopReason"`
				RawContent json.RawMessage `json:"content"`
			}
			if err := json.Unmarshal(raw.RawMessage, &nested); err == nil {
				if nested.Role != "" && r.Role == "" {
					r.Role = nested.Role
				}
				if nested.StopReason != "" {
					r.StopReason = nested.StopReason
				}
				if r.Content == "" && len(nested.RawContent) > 0 {
					var text string
					if err := json.Unmarshal(nested.RawContent, &text); err == nil {
						r.Content = text
					} else {
						var blocks []struct {
							Type string `json:"type"`
							Text string `json:"text"`
							Name string `json:"name"`
						}
						if err := json.Unmarshal(nested.RawContent, &blocks); err == nil {
							var sb strings.Builder
							for _, b := range blocks {
								if b.Text != "" {
									if sb.Len() > 0 {
										sb.WriteString("\n")
									}
									sb.WriteString(b.Text)
								}
								if (b.Type == "tool_use" || b.Type == "tool_call" || b.Type == "toolCall") && b.Name != "" {
									r.ToolCalls = append(r.ToolCalls, struct {
										Name string `json:"name"`
									}{Name: b.Name})
								}
							}
							r.Content = sb.String()
						}
					}
				}
			}
		}
	}

	return nil
}

func (r *TranscriptRecord) IsUser() bool {
	t := strings.ToUpper(r.Type)
	role := strings.ToUpper(r.Role)
	return t == "USER_INPUT" || t == "USER" || role == "USER" || t == "HUMAN" || role == "HUMAN"
}

func (r *TranscriptRecord) IsAssistant() bool {
	t := strings.ToUpper(r.Type)
	role := strings.ToUpper(r.Role)
	return t == "PLANNER_RESPONSE" || t == "ASSISTANT" || role == "ASSISTANT" || t == "MODEL" || role == "MODEL" || t == "BOT"
}

func (r *TranscriptRecord) GetContent() string {
	if r.Content != "" {
		return r.Content
	}
	if r.Text != "" {
		return r.Text
	}
	return r.Message
}

type FileWatcher struct {
	path string
}

func NewFileWatcher(path string) *FileWatcher {
	return &FileWatcher{path: path}
}

// Watch streams newly completed Turns and real-time AgentActivity from the transcript file.
func (w *FileWatcher) Watch(ctx context.Context, turnCh chan<- *Turn, activityCh chan<- AgentActivity) error {
	file, err := os.Open(w.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsWatcher.Close()

	if err := fsWatcher.Add(w.path); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	var pendingUser string
	turnCount := 0
	isCatchingUp := true

	sendActivity := func(act AgentActivity) {
		if activityCh == nil {
			return
		}
		select {
		case activityCh <- act:
		default:
		}
	}

	var partialLine string

	processAvailableLines := func() {
		for {
			chunk, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF && len(chunk) > 0 {
					partialLine += chunk
				}
				break
			}

			fullLine := partialLine + chunk
			partialLine = ""

			line := strings.TrimSpace(fullLine)
			if line == "" {
				continue
			}

			var rec TranscriptRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}

			if rec.IsUser() && rec.GetContent() != "" {
				userText := rec.GetContent()
				if matches := userRequestRegex.FindStringSubmatch(userText); len(matches) > 1 {
					userText = strings.TrimSpace(matches[1])
				}
				pendingUser = userText
				if !isCatchingUp {
					sendActivity(AgentActivity{
						IsBusy:    true,
						Prompt:    pendingUser,
						Tool:      "Thinking...",
						Timestamp: time.Now(),
					})
				}
			} else if rec.IsAssistant() && rec.StopReason == "toolUse" {
				// Pi persists intermediate assistant messages before tool results.
				// Their text is not the final answer for this user turn.
				if pendingUser == "" || isCatchingUp {
					continue
				}
				toolName := "Thinking..."
				if len(rec.ToolCalls) > 0 {
					toolName = "Running " + rec.ToolCalls[0].Name
				}
				sendActivity(AgentActivity{IsBusy: true, Prompt: pendingUser, Tool: toolName, Timestamp: time.Now()})
			} else if len(rec.ToolCalls) > 0 && pendingUser != "" && !isCatchingUp {
				toolName := rec.ToolCalls[0].Name
				sendActivity(AgentActivity{
					IsBusy:    true,
					Prompt:    pendingUser,
					Tool:      "Running " + toolName,
					Timestamp: time.Now(),
				})
			} else if rec.IsAssistant() && rec.GetContent() != "" && pendingUser != "" {
				turnCount++
				t := &Turn{
					ID:        turnCount,
					User:      pendingUser,
					Assistant: rec.GetContent(),
					Timestamp: time.Now(),
					IsCatchUp: isCatchingUp,
				}
				pendingUser = ""

				select {
				case turnCh <- t:
				case <-ctx.Done():
					return
				}

				if !isCatchingUp {
					sendActivity(AgentActivity{
						IsBusy:    false,
						Timestamp: time.Now(),
					})
				}
			} else if rec.IsAssistant() && rec.StopReason == "stop" && pendingUser != "" {
				// A completed Pi turn with no text should not leave the busy indicator on.
				pendingUser = ""
				if !isCatchingUp {
					sendActivity(AgentActivity{IsBusy: false, Timestamp: time.Now()})
				}
			}
		}
	}

	// Reopen after atomic replacement, or rewind after an in-place truncate.
	// Replaying the new file from the beginning preserves turn IDs, allowing
	// the UI's persisted checkpoint to discard previously seen turns.
	refreshFile := func() {
		current, err := file.Stat()
		if err != nil {
			return
		}
		latest, err := os.Stat(w.path)
		if err != nil { // The path may briefly disappear during rotation.
			return
		}
		if !os.SameFile(current, latest) {
			next, err := os.Open(w.path)
			if err != nil {
				return
			}
			_ = fsWatcher.Remove(w.path) // Old inode may already have lost its watch.
			_ = fsWatcher.Add(w.path)    // Polling still works if a watch cannot be added.
			_ = file.Close()
			file = next
			reader = bufio.NewReader(file)
			partialLine, pendingUser = "", ""
			turnCount = 0
		} else if position, err := file.Seek(0, io.SeekCurrent); err == nil && latest.Size() < position {
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return
			}
			reader.Reset(file)
			partialLine, pendingUser = "", ""
			turnCount = 0
		}
	}

	// Initial catch-up on existing content
	processAvailableLines()
	isCatchingUp = false
	if pendingUser != "" {
		sendActivity(AgentActivity{
			IsBusy:    true,
			Prompt:    pendingUser,
			Tool:      "Thinking...",
			Timestamp: time.Now(),
		})
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
				refreshFile()
				processAvailableLines()
			}
		case <-ticker.C:
			// Also recover if the path was replaced and the old watch went away.
			refreshFile()
			processAvailableLines()
		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return nil
			}
			_ = err
		}
	}
}
