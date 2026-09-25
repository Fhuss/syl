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
	w := NewFileWatcher(logFile)

	go func() {
		_ = w.Watch(ctx, turnCh)
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
