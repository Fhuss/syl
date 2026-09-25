package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileWatcher(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "transcript.jsonl")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	turnCh := make(chan *Turn, 5)
	activityCh := make(chan AgentActivity, 5)
	w := NewFileWatcher(logFile)

	go func() {
		_ = w.Watch(ctx, turnCh, activityCh)
	}()

	// Write turn 1
	line1 := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"<USER_REQUEST>How do I deploy on AWS?</USER_REQUEST>"}` + "\n"
	line2 := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"Here is the AWS deployment guide."}` + "\n"

	_, _ = f.WriteString(line1)
	_, _ = f.WriteString(line2)
	_ = f.Sync()

	select {
	case turn := <-turnCh:
		if turn.User != "How do I deploy on AWS?" {
			t.Fatalf("unexpected user prompt: %s", turn.User)
		}
		if turn.Assistant != "Here is the AWS deployment guide." {
			t.Fatalf("unexpected assistant response: %s", turn.Assistant)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for turn event")
	}
}

func TestFileWatcherPartialLineChunk(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "transcript.jsonl")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	turnCh := make(chan *Turn, 5)
	activityCh := make(chan AgentActivity, 5)
	w := NewFileWatcher(logFile)

	go func() {
		_ = w.Watch(ctx, turnCh, activityCh)
	}()

	// Write half of line 1 without newline
	part1 := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT"`
	_, _ = f.WriteString(part1)
	_ = f.Sync()

	// Wait 150ms to allow watcher ticker / fsnotify to read EOF on partial line
	time.Sleep(150 * time.Millisecond)

	// Write second half of line 1 with newline
	part2 := `,"status":"DONE","content":"<USER_REQUEST>How do I test streaming?</USER_REQUEST>"}` + "\n"
	_, _ = f.WriteString(part2)
	_ = f.Sync()

	// Write assistant response
	line2 := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"Streaming test successful."}` + "\n"
	_, _ = f.WriteString(line2)
	_ = f.Sync()

	select {
	case turn := <-turnCh:
		if turn.User != "How do I test streaming?" {
			t.Fatalf("expected user prompt 'How do I test streaming?', got: %s", turn.User)
		}
		if turn.Assistant != "Streaming test successful." {
			t.Fatalf("unexpected assistant response: %s", turn.Assistant)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for turn event after partial chunk write")
	}
}

