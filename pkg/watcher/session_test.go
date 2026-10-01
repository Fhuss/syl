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

func TestFindSessionByIDBeyondRecentHundred(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".pi", "agent", "sessions", "--project--")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "2026-01-01T00-00-00-000Z_old-session.jsonl")
	if err := os.WriteFile(old, []byte("{\"type\":\"session\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	then := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(old, then, then); err != nil {
		t.Fatal(err)
	}
	for i := range 120 {
		path := filepath.Join(dir, fmt.Sprintf("2026-09-26T12-00-00-%03dZ_new-%03d.jsonl", i, i))
		if err := os.WriteFile(path, []byte("{\"type\":\"session\"}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session, err := FindSessionByID("old-session")
	if err != nil || session == nil || session.Path != old {
		t.Fatalf("old explicit session must be found: %+v, %v", session, err)
	}
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

func TestSelectAutoSessionAntigravityCWD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_SESSION_FILE", "")

	myProject := filepath.Join(home, "Dev", "syl")
	otherProject := filepath.Join(home, "Dev", "wedding-app")

	brainDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain")

	// Session 1: my project (older)
	s1Dir := filepath.Join(brainDir, "session-syl", ".system_generated", "logs")
	if err := os.MkdirAll(s1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s1Path := filepath.Join(s1Dir, "transcript.jsonl")
	s1Data := fmt.Sprintf("{\"step_index\":0,\"source\":\"USER_EXPLICIT\",\"type\":\"USER_INPUT\",\"content\":\"User request in syl\"}\n{\"step_index\":1,\"tool_calls\":[{\"name\":\"run_command\",\"args\":{\"Cwd\":\"%s\"}}]}\n", myProject)
	if err := os.WriteFile(s1Path, []byte(s1Data), 0o644); err != nil {
		t.Fatal(err)
	}
	tOld := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(s1Path, tOld, tOld)

	// Session 2: other project (newer)
	s2Dir := filepath.Join(brainDir, "session-wedding", ".system_generated", "logs")
	if err := os.MkdirAll(s2Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s2Path := filepath.Join(s2Dir, "transcript.jsonl")
	s2Data := fmt.Sprintf("{\"step_index\":0,\"source\":\"USER_EXPLICIT\",\"type\":\"USER_INPUT\",\"content\":\"User request in wedding\"}\n{\"step_index\":1,\"tool_calls\":[{\"name\":\"run_command\",\"args\":{\"Cwd\":\"%s\"}}]}\n", otherProject)
	if err := os.WriteFile(s2Path, []byte(s2Data), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. When querying with myProject CWD, should match session-syl even though session-wedding is newer
	sess, err := SelectAutoSession(myProject, "")
	if err != nil || sess == nil {
		t.Fatalf("expected to find session for %s, got %v", myProject, err)
	}
	if sess.ID != "session-syl" {
		t.Errorf("expected session-syl, got %s", sess.ID)
	}

	// 2. When querying with non-existent project CWD, should return nil and NOT fall back to session-wedding
	nonExistent := filepath.Join(home, "Dev", "unknown-repo")
	sessNone, err := SelectAutoSession(nonExistent, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessNone != nil {
		t.Errorf("expected nil session for unknown CWD, but got %s (%s)", sessNone.ID, sessNone.Path)
	}
}

func TestFindNewerSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	myProject := filepath.Join(home, "Dev", "syl")
	brainDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain")

	sDir := filepath.Join(brainDir, "session-new", ".system_generated", "logs")
	if err := os.MkdirAll(sDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sPath := filepath.Join(sDir, "transcript.jsonl")
	sData := fmt.Sprintf("{\"step_index\":0,\"source\":\"USER_EXPLICIT\",\"type\":\"USER_INPUT\",\"content\":\"New conversation started\"}\n{\"step_index\":1,\"tool_calls\":[{\"name\":\"run_command\",\"args\":{\"Cwd\":\"%s\"}}]}\n", myProject)
	if err := os.WriteFile(sPath, []byte(sData), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	_ = os.Chtimes(sPath, now, now)

	// Since 5 seconds ago -> should find session-new
	found, err := FindNewerSession(now.Add(-5*time.Second), myProject, "")
	if err != nil || found == nil {
		t.Fatalf("expected to find newer session, got %v", err)
	}
	if found.ID != "session-new" {
		t.Errorf("expected session-new, got %s", found.ID)
	}

	// With excludeID = session-new -> should return nil
	excluded, err := FindNewerSession(now.Add(-5*time.Second), myProject, "session-new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if excluded != nil {
		t.Errorf("expected nil when excluded, got %s", excluded.ID)
	}

	// Since 5 minutes in future -> should return nil
	future, err := FindNewerSession(now.Add(5*time.Minute), myProject, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if future != nil {
		t.Errorf("expected nil for future since time, got %s", future.ID)
	}
}

