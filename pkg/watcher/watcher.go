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

// TranscriptRecord represents a single JSON line in an Antigravity/agent transcript.
type TranscriptRecord struct {
	StepIndex int    `json:"step_index"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	ToolCalls []struct {
		Name string `json:"name"`
	} `json:"tool_calls"`
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
	defer file.Close()

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

	processAvailableLines := func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					// read error
				}
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

			if rec.Type == "USER_INPUT" && rec.Content != "" {
				userText := rec.Content
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
			} else if len(rec.ToolCalls) > 0 && pendingUser != "" && !isCatchingUp {
				toolName := rec.ToolCalls[0].Name
				sendActivity(AgentActivity{
					IsBusy:    true,
					Prompt:    pendingUser,
					Tool:      "Running " + toolName,
					Timestamp: time.Now(),
				})
			} else if rec.Type == "PLANNER_RESPONSE" && rec.Content != "" && pendingUser != "" {
				turnCount++
				t := &Turn{
					ID:        turnCount,
					User:      pendingUser,
					Assistant: rec.Content,
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
			}
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
			if event.Has(fsnotify.Write) {
				processAvailableLines()
			}
		case <-ticker.C:
			// Fallback poll in case fsnotify misses an append
			processAvailableLines()
		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return nil
			}
			_ = err
		}
	}
}
