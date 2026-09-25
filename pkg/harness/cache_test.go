package harness

import (
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
}
