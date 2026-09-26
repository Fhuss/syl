package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesExistingFilePrivately(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new state")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new state" {
		t.Fatalf("got %q, err=%v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("new file must replace public old file with mode 0600: info=%v err=%v", info, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file was left behind: %v, err=%v", entries, err)
	}
}
