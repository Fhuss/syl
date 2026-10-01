package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/watcher"
)

func TestFileSessionLabelKeepsTranscriptsSeparate(t *testing.T) {
	t.Setenv("SYL_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	one := filepath.Join(root, "one", "transcript.jsonl")
	two := filepath.Join(root, "two", "transcript.jsonl")
	for _, path := range []string{one, two} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := fileSessionLabel(one)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fileSessionLabel(two)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == "demo" || second == "demo" {
		t.Fatalf("different transcript paths shared snapshot key: %q, %q", first, second)
	}
	alias := filepath.Join(root, "alias.jsonl")
	if err := os.Symlink(one, alias); err != nil {
		t.Fatal(err)
	}
	aliased, err := fileSessionLabel(alias)
	if err != nil || first != aliased {
		t.Fatalf("symlink should reuse the same snapshot: %q, %v", aliased, err)
	}
	store := card.NewStore()
	store.Add(&card.Card{ID: "one", Content: "first transcript"})
	path, err := card.SessionFilePath(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSnapshot(path, first, 4); err != nil {
		t.Fatal(err)
	}
	otherPath, err := card.SessionFilePath(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(otherPath); !os.IsNotExist(err) {
		t.Fatalf("second transcript loaded first snapshot: %v", err)
	}
}

func TestSelectAutoSessionPeerCWDMatching(t *testing.T) {
	// If peer/search CWD is set to a directory with no sessions,
	// SelectAutoSession should return nil rather than arbitrarily picking a session
	// from an unrelated project.
	emptyWorkspace := t.TempDir()
	sess, err := watcher.SelectAutoSession(emptyWorkspace, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess != nil && sess.CWD != "" && sess.CWD != emptyWorkspace {
		t.Fatalf("expected nil or matching session, got session for CWD %q", sess.CWD)
	}
}

