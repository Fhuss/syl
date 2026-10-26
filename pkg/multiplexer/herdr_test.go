package multiplexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeHerdr(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "herdr")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TEST_HERDR_LOG"
case "$1 $2" in
  'pane layout') printf '%s\n' "$TEST_HERDR_LAYOUT" ;;
  'pane neighbor') printf '%s\n' "{\"result\":{\"neighbor\":{\"pane_id\":\"$TEST_HERDR_NEIGHBOR\"}}}" ;;
  'pane get')
    case "$3" in
      w1:p1) printf '%s\n' '{"result":{"pane":{"pane_id":"w1:p1","agent":"pi","cwd":"/tmp/project","agent_session":{"kind":"path","value":"/tmp/current.jsonl"}}}}' ;;
      w1:p2) printf '%s\n' '{"result":{"pane":{"pane_id":"w1:p2","cwd":"/tmp/project"}}}' ;;
      w1:p3) printf '%s\n' '{"result":{"pane":{"pane_id":"w1:p3","agent":"claude","cwd":"/tmp/other"}}}' ;;
    esac ;;
  'pane process-info') printf '%s\n' '{"result":{"process_info":{"foreground_processes":[]}}}' ;;
  'pane send-text') if [ "$TEST_HERDR_FAIL" = 1 ]; then echo 'rejected' >&2; exit 1; fi ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls")
	t.Setenv("TEST_HERDR_LOG", log)
	t.Setenv("TEST_HERDR_LAYOUT", `{"result":{"layout":{"panes":[{"pane_id":"w1:p1"},{"pane_id":"w1:p2"},{"pane_id":"w1:p3"}]}}}`)
	t.Setenv("TEST_HERDR_NEIGHBOR", "w1:p2")
	t.Setenv("HERDR_BIN_PATH", path)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PANE_ID", "w1:p2")
	return log
}

func TestHerdrPeerAndExactSession(t *testing.T) {
	log := fakeHerdr(t)
	// The second agent must be disambiguated; never attach to a random tab peer.
	if _, err := HerdrPeer(); err == nil || !strings.Contains(err.Error(), "multiple agents") {
		t.Fatalf("expected ambiguity, got %v", err)
	}
	if got := HerdrPeerSessionFile(); got != "" {
		t.Fatalf("ambiguous session: %q", got)
	}
	if got := DiscoverPeer(""); got.Type != "clipboard" {
		t.Fatalf("ambiguous send target: %+v", got)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "--current") {
		t.Fatal("Herdr must use explicit pane IDs, not global focus")
	}
}

func TestHerdrAdjacentAgent(t *testing.T) {
	fakeHerdr(t)
	t.Setenv("TEST_HERDR_NEIGHBOR", "w1:p1")
	if got := HerdrPeerSessionFile(); got != "/tmp/current.jsonl" {
		t.Fatalf("adjacent agent session: %q", got)
	}
}

func TestHerdrExecAcceptsEmptySuccessAndReportsFailure(t *testing.T) {
	log := fakeHerdr(t)
	if err := HerdrExec("pane", "send-text", "w1:p1", "hello"); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "pane send-text w1:p1 hello") {
		t.Fatalf("command not sent: %s", calls)
	}
	t.Setenv("TEST_HERDR_FAIL", "1")
	if err := HerdrExec("pane", "send-text", "w1:p1", "private text"); err == nil || strings.Contains(err.Error(), "private text") {
		t.Fatalf("expected safe error, got %v", err)
	}
}

func TestHerdrSinglePeer(t *testing.T) {
	log := fakeHerdr(t)
	// Simulate a tab with only one AI agent, plus Syl.
	t.Setenv("TEST_HERDR_LAYOUT", `{"result":{"layout":{"panes":[{"pane_id":"w1:p1"},{"pane_id":"w1:p2"}]}}}`)
	if got := HerdrPeerSessionFile(); got != "/tmp/current.jsonl" {
		t.Fatalf("exact session: %q", got)
	}
	if got := DetectPeerCWD(); got != "/tmp/project" {
		t.Fatalf("cwd: %q", got)
	}
	if got := DetectPeerAgent(); got != "pi" {
		t.Fatalf("agent: %q", got)
	}
	target := DiscoverPeer("")
	if target.Type != "herdr" || target.Surface != "w1:p1" {
		t.Fatalf("peer target: %+v", target)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "--current") {
		t.Fatal("Herdr must use explicit pane IDs")
	}
}
