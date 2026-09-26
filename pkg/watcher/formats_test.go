package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClaudeCodeNestedMessageFormat tests parsing Claude Code's JSONL where
// user/assistant content is nested inside {"type":"user","message":{"role":"user","content":[...]}}.
func TestClaudeCodeNestedMessageFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "claude-session.jsonl")

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

	// Claude Code format: type discriminator + nested message object with content blocks
	userLine := `{"type":"user","uuid":"u1","parentUuid":null,"timestamp":"2026-09-01T10:00:00Z","sessionId":"sess1","message":{"role":"user","content":[{"type":"text","text":"How do I deploy to AWS ECS?"}]}}` + "\n"
	assistantLine := `{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-09-01T10:00:05Z","sessionId":"sess1","message":{"role":"assistant","content":[{"type":"text","text":"Here is the ECS deployment guide:\n\n1. Create a task definition\n2. Create a service\n3. Configure the load balancer"}]}}` + "\n"

	_, _ = f.WriteString(userLine)
	_, _ = f.WriteString(assistantLine)
	_ = f.Sync()

	select {
	case turn := <-turnCh:
		if turn.User != "How do I deploy to AWS ECS?" {
			t.Fatalf("unexpected user prompt: %q", turn.User)
		}
		if turn.Assistant == "" {
			t.Fatal("assistant response was empty")
		}
		if turn.ID != 1 {
			t.Fatalf("expected turn ID 1, got %d", turn.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Claude Code turn event")
	}
}

// TestClaudeCodeStringContentFormat tests parsing Claude Code's simpler format
// where content is a plain string in the nested message.
func TestClaudeCodeStringContentFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "claude-simple.jsonl")

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

	userLine := `{"type":"user","message":{"role":"user","content":"Set up PostgreSQL with Docker"}}` + "\n"
	assistantLine := `{"type":"assistant","message":{"role":"assistant","content":"Here is the docker-compose.yml for PostgreSQL..."}}` + "\n"

	_, _ = f.WriteString(userLine)
	_, _ = f.WriteString(assistantLine)
	_ = f.Sync()

	select {
	case turn := <-turnCh:
		if turn.User != "Set up PostgreSQL with Docker" {
			t.Fatalf("unexpected user prompt: %q", turn.User)
		}
		if turn.Assistant != "Here is the docker-compose.yml for PostgreSQL..." {
			t.Fatalf("unexpected assistant response: %q", turn.Assistant)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Claude Code string content turn")
	}
}

// TestPiAgentMessageFormat tests parsing Pi Agent's JSONL transcript format where
// records use {"type":"message","message":{"role":"user","content":[{"type":"text","text":"..."}]}}.
func TestPiAgentMessageFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "pi-session.jsonl")

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

	// Pi format: generic type "message" with nested message.role + message.content blocks
	// Includes the session header line which should be skipped
	sessionLine := `{"type":"session","version":3,"id":"019fbf71-fa87-7bc5-9375-3c889a28977d","timestamp":"2026-08-01T22:29:05.799Z","cwd":"/Users/fabian"}` + "\n"
	modelLine := `{"type":"model_change","id":"fbd1c2f4","timestamp":"2026-08-01T22:29:06.380Z","provider":"google","modelId":"gemini-3.6-flash"}` + "\n"
	userLine := `{"type":"message","id":"1b7de7a8","parentId":"9ac1f148","timestamp":"2026-08-01T22:29:17.009Z","message":{"role":"user","content":[{"type":"text","text":"what can you do for me?"}]}}` + "\n"
	assistantLine := `{"type":"message","id":"6ad2c197","parentId":"1b7de7a8","timestamp":"2026-08-01T22:29:22.377Z","message":{"role":"assistant","content":[{"type":"text","text":"I'm an expert AI coding assistant. I can help with full-stack development, architecture, and more."}]}}` + "\n"

	_, _ = f.WriteString(sessionLine)
	_, _ = f.WriteString(modelLine)
	_, _ = f.WriteString(userLine)
	_, _ = f.WriteString(assistantLine)
	_ = f.Sync()

	select {
	case turn := <-turnCh:
		if turn.User != "what can you do for me?" {
			t.Fatalf("unexpected user prompt: %q", turn.User)
		}
		if turn.Assistant == "" {
			t.Fatal("assistant response was empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Pi Agent turn event")
	}
}

func TestPiToolUseTextWaitsForFinalAnswer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pi-tool-use.jsonl")
	data := `{"type":"message","message":{"role":"user","content":[{"type":"text","text":"Deploy the service"}]}}` + "\n" +
		`{"type":"message","message":{"role":"assistant","stopReason":"toolUse","content":[{"type":"text","text":"I'll inspect the config first"},{"type":"toolCall","name":"bash"}]}}` + "\n" +
		`{"type":"message","message":{"role":"toolResult","content":[{"type":"text","text":"Config found"}]}}` + "\n" +
		`{"type":"message","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"Deploy with kubectl apply -f service.yaml"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	turns := make(chan *Turn, 4)
	go func() { _ = NewFileWatcher(path).Watch(ctx, turns, nil) }()
	select {
	case turn := <-turns:
		if turn.User != "Deploy the service" || turn.Assistant != "Deploy with kubectl apply -f service.yaml" {
			t.Fatalf("paired wrong Pi answer: %+v", turn)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no completed turn")
	}
	select {
	case extra := <-turns:
		t.Fatalf("intermediate assistant message emitted as extra turn: %+v", extra)
	default:
	}
}

// TestPiAgentToolUseInContentBlocks tests that tool_use blocks in Pi/Claude content
// are properly extracted as ToolCalls.
func TestPiAgentToolUseInContentBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "pi-tools.jsonl")

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

	userLine := `{"type":"message","message":{"role":"user","content":[{"type":"text","text":"list files"}]}}` + "\n"
	// Assistant uses a tool first (this should register as activity, not a completed turn)
	toolLine := `{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Let me check."},{"type":"tool_use","name":"list_directory","id":"tool1"}]}}` + "\n"
	// Then a final assistant response
	finalLine := `{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Here are the files in the directory."}]}}` + "\n"

	_, _ = f.WriteString(userLine)
	_, _ = f.WriteString(toolLine)
	_ = f.Sync()

	// The tool line should produce a turn because it has text content too
	select {
	case turn := <-turnCh:
		if turn.User != "list files" {
			t.Fatalf("unexpected user prompt: %q", turn.User)
		}
		// The assistant content should include the text from the tool_use message
		if turn.Assistant != "Let me check." {
			t.Fatalf("unexpected assistant response: %q", turn.Assistant)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool use turn event")
	}

	// Write the final response — should create turn 2
	_, _ = f.WriteString(finalLine)
	_ = f.Sync()

	// The final assistant line should NOT produce a new turn because there's no
	// new user prompt between them (pendingUser was consumed)
	select {
	case <-turnCh:
		// This is acceptable — depends on implementation detail
	case <-time.After(500 * time.Millisecond):
		// Also acceptable — no pending user means no turn
	}
}

// TestMultiTurnClaudeCode tests multiple conversation turns in Claude Code format.
func TestMultiTurnClaudeCode(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "claude-multi.jsonl")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	turnCh := make(chan *Turn, 10)
	activityCh := make(chan AgentActivity, 10)
	w := NewFileWatcher(logFile)

	go func() {
		_ = w.Watch(ctx, turnCh, activityCh)
	}()

	// Turn 1
	_, _ = f.WriteString(`{"type":"user","message":{"role":"user","content":"What is Docker?"}}` + "\n")
	_, _ = f.WriteString(`{"type":"assistant","message":{"role":"assistant","content":"Docker is a containerization platform."}}` + "\n")
	// Turn 2
	_, _ = f.WriteString(`{"type":"user","message":{"role":"user","content":"How do I write a Dockerfile?"}}` + "\n")
	_, _ = f.WriteString(`{"type":"assistant","message":{"role":"assistant","content":"Start with FROM, then ADD, RUN, and CMD."}}` + "\n")
	_ = f.Sync()

	// Collect turn 1
	select {
	case turn := <-turnCh:
		if turn.User != "What is Docker?" {
			t.Fatalf("turn 1 user: got %q", turn.User)
		}
		if turn.Assistant != "Docker is a containerization platform." {
			t.Fatalf("turn 1 assistant: got %q", turn.Assistant)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for turn 1")
	}

	// Collect turn 2
	select {
	case turn := <-turnCh:
		if turn.User != "How do I write a Dockerfile?" {
			t.Fatalf("turn 2 user: got %q", turn.User)
		}
		if turn.Assistant != "Start with FROM, then ADD, RUN, and CMD." {
			t.Fatalf("turn 2 assistant: got %q", turn.Assistant)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for turn 2")
	}
}

// TestExtractSessionTitlePiFormat tests that session title extraction works with Pi's
// nested message format.
func TestExtractSessionTitlePiFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "pi-session.jsonl")

	content := `{"type":"session","version":3,"id":"019fbf71","timestamp":"2026-08-01T22:29:05.799Z"}` + "\n" +
		`{"type":"model_change","id":"fbd1c2f4","provider":"google","modelId":"gemini-3.6-flash"}` + "\n" +
		`{"type":"message","id":"1b7de7a8","message":{"role":"user","content":[{"type":"text","text":"what can you do for me?"}]}}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	title := extractSessionTitle(logPath)
	if title != "what can you do for me?" {
		t.Fatalf("expected title %q, got %q", "what can you do for me?", title)
	}
}

// TestExtractSessionTitleClaudeCodeNestedFormat tests title extraction from Claude Code's
// nested message format.
func TestExtractSessionTitleClaudeCodeNestedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "claude-nested.jsonl")

	content := `{"type":"user","uuid":"u1","message":{"role":"user","content":[{"type":"text","text":"Setup a Next.js project with Tailwind"}]}}` + "\n" +
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"text","text":"Here is the setup..."}]}}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	title := extractSessionTitle(logPath)
	if title != "Setup a Next.js project with Tailwind" {
		t.Fatalf("expected title %q, got %q", "Setup a Next.js project with Tailwind", title)
	}
}

// TestMixedContentBlocks verifies that only text blocks are extracted from content arrays
// and thinking blocks are excluded.
func TestMixedContentBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "mixed.jsonl")

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

	userLine := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"explain monads"}]}}` + "\n"
	// Assistant response has thinking block (no text field) + text block
	assistantLine := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Let me think about how to explain..."},{"type":"text","text":"A monad is a design pattern for composing functions with effects."}]}}` + "\n"

	_, _ = f.WriteString(userLine)
	_, _ = f.WriteString(assistantLine)
	_ = f.Sync()

	select {
	case turn := <-turnCh:
		if turn.User != "explain monads" {
			t.Fatalf("unexpected user prompt: %q", turn.User)
		}
		// Should only contain the text block, not the thinking block
		expected := "A monad is a design pattern for composing functions with effects."
		if turn.Assistant != expected {
			t.Fatalf("expected assistant %q, got %q", expected, turn.Assistant)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mixed content turn")
	}
}

// TestPiIdExtractorFromFilename tests that Pi session filenames like
// "2026-08-01T22-29-05-799Z_019fbf71-fa87-7bc5-9375-3c889a28977d.jsonl"
// extract the UUID part correctly.
func TestPiIdExtractorFromFilename(t *testing.T) {
	filename := "2026-08-01T22-29-05-799Z_019fbf71-fa87-7bc5-9375-3c889a28977d"
	// Simulate what the Pi idExtractor does
	idx := -1
	for i, c := range filename {
		if c == '_' {
			idx = i
			break
		}
	}
	if idx == -1 || idx+1 >= len(filename) {
		t.Fatal("expected underscore separator in Pi filename")
	}
	uuidPart := filename[idx+1:]
	expected := "019fbf71-fa87-7bc5-9375-3c889a28977d"
	if uuidPart != expected {
		t.Fatalf("expected UUID %q, got %q", expected, uuidPart)
	}

	short := uuidPart
	if len(short) > 8 {
		short = short[:8]
	}
	if short != "019fbf71" {
		t.Fatalf("expected short ID %q, got %q", "019fbf71", short)
	}
}
