package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSessionTitle(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")

	content := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"<USER_REQUEST>Build AWS ECS Fargate deployment guide</USER_REQUEST>"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	title := extractSessionTitle(logPath)
	expected := "Build AWS ECS Fargate deployment guide"
	if title != expected {
		t.Fatalf("expected title %q, got %q", expected, title)
	}
}
