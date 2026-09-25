package multiplexer

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/atotto/clipboard"
)

var (
	// Matches lines from `cmux list-panels`:
	// * surface:5  terminal  [focused]  "agyd"
	//   surface:6  terminal  "syl --harness agy"
	cmuxPanelRegex = regexp.MustCompile(`^\s*(\*?)\s*(surface:\S+)\s+(\S+)(.*)`)
)

// Target holds the discovered target for sending reference cards.
type Target struct {
	Type        string // "cmux", "tmux", "clipboard"
	Description string // e.g. "cmux surface:5", "tmux %0", "clipboard"
	Surface     string // surface or pane identifier
}

// DiscoverPeer attempts to find an adjacent chat terminal in cmux, tmux, or defaults to clipboard.
func DiscoverPeer(overrideTarget string) Target {
	if overrideTarget != "" {
		if strings.HasPrefix(overrideTarget, "surface:") {
			return Target{Type: "cmux", Description: overrideTarget, Surface: overrideTarget}
		}
		if strings.HasPrefix(overrideTarget, "%") {
			return Target{Type: "tmux", Description: overrideTarget, Surface: overrideTarget}
		}
	}

	// 1. Try cmux
	if cmuxPeer, ok := findCmuxPeer(); ok {
		return Target{
			Type:        "cmux",
			Description: cmuxPeer,
			Surface:     cmuxPeer,
		}
	}

	// 2. Try tmux
	if tmuxPeer, ok := findTmuxPeer(); ok {
		return Target{
			Type:        "tmux",
			Description: tmuxPeer,
			Surface:     tmuxPeer,
		}
	}

	// 3. Fallback to system clipboard
	return Target{
		Type:        "clipboard",
		Description: "system clipboard",
	}
}

// Send injects text into the peer terminal or copies to system clipboard.
func Send(text string, overrideTarget string) (string, error) {
	target := DiscoverPeer(overrideTarget)

	switch target.Type {
	case "cmux":
		setCmd := exec.Command("cmux", "set-buffer", text)
		pasteCmd := exec.Command("cmux", "paste-buffer", "--surface", target.Surface)
		if setCmd.Run() == nil && pasteCmd.Run() == nil {
			return fmt.Sprintf("✓ Pasted to %s", target.Description), nil
		}
		cmd := exec.Command("cmux", "send", "--surface", target.Surface, text)
		if err := cmd.Run(); err == nil {
			return fmt.Sprintf("✓ Sent to %s", target.Description), nil
		}
		// If cmux failed, fall through to clipboard
	case "tmux":
		setCmd := exec.Command("tmux", "set-buffer", "--", text)
		pasteCmd := exec.Command("tmux", "paste-buffer", "-t", target.Surface)
		if setCmd.Run() == nil && pasteCmd.Run() == nil {
			return fmt.Sprintf("✓ Sent to tmux pane %s", target.Surface), nil
		}
		// If tmux failed, fall through to clipboard
	}

	if err := clipboard.WriteAll(text); err != nil {
		return "", fmt.Errorf("copy to clipboard: %w", err)
	}

	if target.Type == "clipboard" {
		return "✓ Copied to clipboard (Paste into chat)", nil
	}
	return "✓ Fallback: Copied to clipboard", nil
}

// findCmuxPeer parses `cmux list-panels` to locate the adjacent terminal panel.
func findCmuxPeer() (string, bool) {
	if _, err := exec.LookPath("cmux"); err != nil {
		return "", false
	}

	out, err := exec.Command("cmux", "list-panels").Output()
	if err != nil {
		return "", false
	}

	return ParseCmuxPanels(string(out), os.Getenv("CMUX_SURFACE_ID"))
}

// ParseCmuxPanels parses output from cmux list-panels, picking the best peer terminal.
func ParseCmuxPanels(output string, currentSurface string) (string, bool) {
	var fallbackPeer string
	var focusedPeer string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := cmuxPanelRegex.FindStringSubmatch(line)
		if len(matches) < 4 {
			continue
		}

		isFocusedMarker := matches[1] == "*"
		surfaceID := matches[2]
		panelType := matches[3]
		rest := matches[4]

		// Only target terminal surfaces
		if panelType != "terminal" {
			continue
		}

		// Don't target our own surface
		if currentSurface != "" && surfaceID == currentSurface {
			continue
		}
		if strings.Contains(rest, `"syl`) {
			continue
		}

		if isFocusedMarker || strings.Contains(rest, "[focused]") {
			focusedPeer = surfaceID
		} else if fallbackPeer == "" {
			fallbackPeer = surfaceID
		}
	}

	if focusedPeer != "" {
		return focusedPeer, true
	}
	if fallbackPeer != "" {
		return fallbackPeer, true
	}
	return "", false
}

// findTmuxPeer locates the peer pane in the current tmux window.
func findTmuxPeer() (string, bool) {
	if os.Getenv("TMUX") == "" {
		return "", false
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return "", false
	}

	currentPane := os.Getenv("TMUX_PANE")
	out, err := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_active}").Output()
	if err != nil {
		return "", false
	}

	return ParseTmuxPanes(string(out), currentPane)
}

// ParseTmuxPanes extracts the adjacent pane from `tmux list-panes`.
func ParseTmuxPanes(output string, currentPane string) (string, bool) {
	var fallbackPane string
	var activePane string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		paneID := fields[0]
		if paneID == currentPane {
			continue
		}

		isActive := len(fields) > 1 && fields[1] == "1"
		if isActive {
			activePane = paneID
		} else if fallbackPane == "" {
			fallbackPane = paneID
		}
	}

	if activePane != "" {
		return activePane, true
	}
	if fallbackPane != "" {
		return fallbackPane, true
	}
	return "", false
}
