package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchCheckDoesNotConfusePiTitleWithSylProcess(t *testing.T) {
	dir := t.TempDir()
	for name, script := range map[string]string{
		"cmux":  "#!/bin/sh\ncase \"$1\" in ping) exit 0;; list-panels) printf '%s\\n' \"$TEST_PANELS\";; esac\n",
		"pgrep": "#!/bin/sh\nexit 1\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scriptPath := filepath.Join(dir, "init-syl.sh")
	if err := os.WriteFile(scriptPath, []byte(InitScriptContent), 0o755); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, panels string
		found        bool
	}{
		{"Pi title only", `* surface:34 terminal "π - Fix Syl session detection"`, false},
		{"Syl process", `  surface:34 terminal "π - Fix Syl session detection"` + "\n" + `* surface:36 terminal [focused] "/Users/fabian/.local/bin/syl"`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("bash", scriptPath, "--check")
			cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "SYL_BIN="+binary, "TEST_PANELS="+tt.panels)
			out, err := cmd.CombinedOutput()
			if tt.found && (err != nil || !strings.Contains(string(out), "surface:36")) {
				t.Fatalf("expected Syl on surface:36: %s (%v)", out, err)
			}
			if !tt.found && (err == nil || !strings.Contains(string(out), "not currently running")) {
				t.Fatalf("Pi title must not count as Syl: %s (%v)", out, err)
			}
		})
	}
}
