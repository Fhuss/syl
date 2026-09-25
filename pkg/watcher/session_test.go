package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestExtractSessionTitleLargeLine(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")

	// Create a line that is ~120KB, exceeding default bufio.MaxScanTokenSize (64KB)
	largePadding := strings.Repeat("A", 120*1024)
	content := fmt.Sprintf(`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"<USER_REQUEST>Large line title test %s</USER_REQUEST>"}`+"\n", largePadding)
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	title := extractSessionTitle(logPath)
	if !strings.HasPrefix(title, "Large line title test") {
		t.Fatalf("expected title starting with 'Large line title test', got %q", title)
	}
}

func TestExtractSessionTitleClaudeCodeFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "claude-session.jsonl")

	content := `{"role":"user","content":"Setup PostgreSQL with Docker Compose"}` + "\n" +
		`{"role":"assistant","content":"Here is the docker-compose.yml..."}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	title := extractSessionTitle(logPath)
	expected := "Setup PostgreSQL with Docker Compose"
	if title != expected {
		t.Fatalf("expected title %q, got %q", expected, title)
	}
}

func TestFindSessionByIDDirectPath(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "custom-transcript.jsonl")

	content := `{"type":"USER_INPUT","content":"Custom transcript prompt"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	session, err := FindSessionByID(logPath)
	if err != nil {
		t.Fatalf("unexpected error finding session by direct path: %v", err)
	}

	if session.Path != logPath {
		t.Fatalf("expected path %q, got %q", logPath, session.Path)
	}
	if session.Title != "Custom transcript prompt" {
		t.Fatalf("expected title 'Custom transcript prompt', got %q", session.Title)
	}
	if session.Agent != "file" {
		t.Fatalf("expected agent 'file', got %q", session.Agent)
	}
}

func TestListSessionsWithCustomDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SYL_TRANSCRIPT_DIR", tmpDir)

	sess1 := filepath.Join(tmpDir, "sess-1.jsonl")
	content := `{"role":"user","content":"Deploy Kubernetes helm chart"}` + "\n"
	if err := os.WriteFile(sess1, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sessions, err := ListSessions(10)
	if err != nil {
		t.Fatalf("unexpected error listing sessions: %v", err)
	}

	found := false
	for _, s := range sessions {
		if s.ID == "sess-1" {
			found = true
			if s.Agent != "custom" {
				t.Errorf("expected agent 'custom', got %q", s.Agent)
			}
			if s.Title != "Deploy Kubernetes helm chart" {
				t.Errorf("expected title 'Deploy Kubernetes helm chart', got %q", s.Title)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected sess-1 to be discovered in custom transcript dir")
	}
}


