package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTitleCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "titles.json")

	cache := NewTitleCache(cacheFile)

	// Initial get returns false
	if _, ok := cache.Get("session-1", 10); ok {
		t.Fatalf("expected miss for non-existent key")
	}

	// Set and get
	cache.Set("session-1", 10, "Stockholm Weather Query")
	if info, err := os.Stat(cacheFile); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("title cache must be private (0600): info=%v err=%v", info, err)
	}
	got, ok := cache.Get("session-1", 10)
	if !ok || got != "Stockholm Weather Query" {
		t.Fatalf("expected 'Stockholm Weather Query', got %q (ok=%v)", got, ok)
	}

	// By prompt
	cache.SetByPrompt("how is the weather in Stockholm?", "Stockholm Weather")
	gotPrompt, okPrompt := cache.GetByPrompt("How is the weather in Stockholm? ")
	if !okPrompt || gotPrompt != "Stockholm Weather" {
		t.Fatalf("expected prompt match 'Stockholm Weather', got %q (ok=%v)", gotPrompt, okPrompt)
	}

	// Reload from disk into new cache instance
	cache2 := NewTitleCache(cacheFile)
	got2, ok2 := cache2.Get("session-1", 10)
	if !ok2 || got2 != "Stockholm Weather Query" {
		t.Fatalf("expected reloaded title 'Stockholm Weather Query', got %q", got2)
	}

	// Try downgrading with a truncated fallback ending in "..."
	cache2.Set("session-1", 10, "how is the weather...")
	got3, _ := cache2.Get("session-1", 10)
	if got3 != "Stockholm Weather Query" {
		t.Errorf("expected definitive title not to be downgraded by truncated fallback, got %q", got3)
	}
}

func TestLoadingOldCacheTightensPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "titles.json")
	if err := os.WriteFile(path, []byte(`{"session:1":"Title"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	cache := NewTitleCache(path)
	if title, ok := cache.Get("session", 1); !ok || title != "Title" {
		t.Fatalf("could not read old cache: %q, %v", title, ok)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("old cache should become private: info=%v err=%v", info, err)
	}
}

func TestIsDefinitiveTitle(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"Stockholm Weather", true},
		{"How Go Handles Packages", true},
		{"did you create the...", false},
		{"summarize the...", false},
		{"Turn #3", false},
		{"", false},
		{"   ", false},
	}

	for _, tc := range cases {
		got := IsDefinitiveTitle(tc.input)
		if got != tc.want {
			t.Errorf("IsDefinitiveTitle(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
