package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fhuss/syl/pkg/multiplexer"
)

// launchHerdr keeps Syl in the same Herdr tab as the invoking pane, so a
// workspace switch moves the agent and its companion together.
func launchHerdr(force, check bool) error {
	current := os.Getenv("HERDR_PANE_ID")
	if current == "" {
		return fmt.Errorf("HERDR_PANE_ID is missing; run syl launch from a Herdr pane")
	}
	pane, err := multiplexer.HerdrGetPane(current)
	if err != nil {
		return err
	}
	if !force || check {
		existing, err := multiplexer.HerdrSylPane(current)
		if err != nil {
			return err
		}
		if existing != "" {
			fmt.Printf("✓ Syl is already running in Herdr pane %s. Pass --force to open another.\n", existing)
			return nil
		}
	}
	if check {
		return fmt.Errorf("Syl is not running in this Herdr tab")
	}

	binary, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(binary); err == nil {
		binary = resolved
	}
	cwd := pane.ForegroundCWD
	if cwd == "" {
		cwd = pane.CWD
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	var split struct {
		Pane struct {
			PaneID string `json:"pane_id"`
		} `json:"pane"`
	}
	if err := multiplexer.HerdrCall(&split, "pane", "split", current, "--direction", "right", "--cwd", cwd, "--no-focus"); err != nil {
		return err
	}
	if split.Pane.PaneID == "" {
		return fmt.Errorf("herdr split returned no new pane ID")
	}

	// With a live transcript, Syl resolves the exact adjacent agent session via
	// Herdr pane metadata. Without one, wait for the first new conversation
	// rather than mistakenly attaching to an old transcript in the same cwd.
	command := shellQuote(binary)
	if pane.AgentSession.Kind != "path" || pane.AgentSession.Value == "" {
		command += " --new"
	}
	if err := multiplexer.HerdrExec("pane", "run", split.Pane.PaneID, command); err != nil {
		return fmt.Errorf("created Herdr pane %s but could not start Syl: %w", split.Pane.PaneID, err)
	}
	fmt.Printf("✓ Launched Syl in Herdr pane %s beside %s.\n", split.Pane.PaneID, current)
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
