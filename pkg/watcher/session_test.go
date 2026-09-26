package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestSelectAutoSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_SESSION_FILE", "")
	cwd := filepath.Join(home, "project")
	other := filepath.Join(home, "other")
	writePi := func(dir, id, title string) string {
		t.Helper()
		folder := filepath.Join(home, ".pi", "agent", "sessions", "--"+filepath.Base(dir)+"--")
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(folder, "2026-09-26T12-00-00-000Z_"+id+".jsonl")
		data := fmt.Sprintf("{\"type\":\"session\",\"cwd\":%q}\n{\"type\":\"message\",\"message\":{\"role\":\"user\",\"content\":%q}}\n", dir, title)
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	mine := writePi(cwd, "my-session", "My session")
	newer := writePi(other, "other-session", "Other session")
	if err := os.Chtimes(mine, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	t.Run("same cwd beats newer global session", func(t *testing.T) {
		session, err := SelectAutoSession(cwd, "")
		if err != nil || session == nil || session.Path != mine {
			t.Fatalf("got %+v, %v; want %s", session, err, mine)
		}
	})
	t.Run("peer checkpoint beats cwd", func(t *testing.T) {
		session, err := SelectAutoSession(cwd, newer)
		if err != nil || session == nil || session.Path != newer || session.ID != "other-session" {
			t.Fatalf("got %+v, %v; want peer %s", session, err, newer)
		}
	})
	t.Run("pi env beats peer", func(t *testing.T) {
		t.Setenv("PI_SESSION_FILE", mine)
		session, err := SelectAutoSession(cwd, newer)
		if err != nil || session == nil || session.Path != mine {
			t.Fatalf("got %+v, %v; want %s", session, err, mine)
		}
	})
	t.Run("missing checkpoint falls back to cwd", func(t *testing.T) {
		session, err := SelectAutoSession(cwd, filepath.Join(home, ".pi", "missing.jsonl"))
		if err != nil || session == nil || session.Path != mine {
			t.Fatalf("got %+v, %v; want %s", session, err, mine)
		}
	})
}

func TestDetectAgentFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/Users/user/.gemini/antigravity-cli/brain/abc/.system_generated/logs/transcript.jsonl", "antigravity"},
		{"/Users/user/.claude/projects/proj/session.jsonl", "claude"},
		{"/Users/user/.pi/sessions/sess.jsonl", "pi"},
		{"/Users/user/workspace/custom.jsonl", "file"},
	}

	for _, tt := range tests {
		got := DetectAgentFromPath(tt.path)
		if got != tt.expected {
			t.Errorf("DetectAgentFromPath(%q) = %q, expected %q", tt.path, got, tt.expected)
		}
	}
}
