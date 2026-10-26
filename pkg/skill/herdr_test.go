package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubHerdr(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "herdr")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TEST_HERDR_LOG"
case "$1 $2" in
  'pane get') printf '%s\n' "{\"result\":{\"pane\":{\"pane_id\":\"wA:p1\",\"cwd\":\"/tmp/project\",\"agent\":\"pi\",\"agent_session\":{\"kind\":\"$TEST_SESSION_KIND\",\"value\":\"$TEST_SESSION_VALUE\"}}}}" ;;
  'pane layout') printf '%s\n' '{"result":{"layout":{"panes":[{"pane_id":"wA:p1"},{"pane_id":"wA:p2"}]}}}' ;;
  'pane process-info') printf '%s\n' "{\"result\":{\"process_info\":{\"foreground_processes\":$TEST_PROCESSES}}}" ;;
  'pane split') printf '%s\n' '{"result":{"pane":{"pane_id":"wA:p3"}}}' ;;
  'pane run') printf '%s\n' '{"result":{"type":"pane_run"}}' ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls")
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PANE_ID", "wA:p1")
	t.Setenv("HERDR_BIN_PATH", path)
	t.Setenv("TEST_HERDR_LOG", log)
	t.Setenv("TEST_PROCESSES", `[]`)
	t.Setenv("TEST_SESSION_KIND", "path")
	t.Setenv("TEST_SESSION_VALUE", "/tmp/session.jsonl")
	return log
}

func TestLaunchHerdrInCurrentTab(t *testing.T) {
	log := stubHerdr(t)
	if err := Launch(false, false); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	text := string(calls)
	for _, want := range []string{"pane layout --pane wA:p1", "pane process-info --pane wA:p2", "pane split wA:p1 --direction right --cwd /tmp/project --no-focus", "pane run wA:p3"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "--new") || strings.Contains(text, "--current") {
		t.Fatalf("active Pi session should attach via exact peer; explicit pane ids required: %s", text)
	}
}

func TestLaunchHerdrNewConversation(t *testing.T) {
	log := stubHerdr(t)
	t.Setenv("TEST_SESSION_KIND", "")
	t.Setenv("TEST_SESSION_VALUE", "")
	if err := Launch(false, false); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "pane run wA:p3") || !strings.Contains(string(calls), "--new") {
		t.Fatalf("expected waiting companion: %s", calls)
	}
}

func TestLaunchHerdrCheckIsScopedToTab(t *testing.T) {
	log := stubHerdr(t)
	t.Setenv("TEST_PROCESSES", `[{"name":"syl","argv0":"/usr/local/bin/syl"}]`)
	if err := Launch(false, true); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "pane split") {
		t.Fatalf("check must not split: %s", calls)
	}
	if err := Launch(false, false); err != nil {
		t.Fatal(err)
	}
	calls, _ = os.ReadFile(log)
	if strings.Contains(string(calls), "pane split") {
		t.Fatalf("existing Syl must not split: %s", calls)
	}
}

func TestLaunchHerdrCheckAbsent(t *testing.T) {
	log := stubHerdr(t)
	if err := Launch(false, true); err == nil {
		t.Fatal("expected check failure")
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "pane split") {
		t.Fatalf("check must not split: %s", calls)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/tmp/Syl's bin"); got != `'/tmp/Syl'\''s bin'` {
		t.Fatalf("unsafe quote: %q", got)
	}
}
