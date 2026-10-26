package multiplexer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// DiscoverPeer attempts to find an adjacent chat terminal in Herdr, cmux or tmux, or defaults to clipboard.
func DiscoverPeer(overrideTarget string) Target {
	if overrideTarget != "" {
		if strings.HasPrefix(overrideTarget, "surface:") {
			return Target{Type: "cmux", Description: overrideTarget, Surface: overrideTarget}
		}
		if strings.HasPrefix(overrideTarget, "%") {
			return Target{Type: "tmux", Description: overrideTarget, Surface: overrideTarget}
		}
	}

	// Herdr owns the inner layout even when its client runs inside Ghostty/cmux.
	if HerdrAvailable() {
		if peer, err := HerdrPeer(); err == nil && peer.PaneID != "" {
			return Target{Type: "herdr", Description: "herdr " + peer.PaneID, Surface: peer.PaneID}
		}
		return Target{Type: "clipboard", Description: "system clipboard"}
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
	if os.Getenv("SYL_MULTIPLEXER_DISABLED") == "1" {
		return "✓ Test: Multiplexer disabled", nil
	}

	// Always write to system clipboard so user has an instant Cmd+V fallback
	_ = clipboard.WriteAll(text)

	target := DiscoverPeer(overrideTarget)

	switch target.Type {
	case "herdr":
		if err := HerdrExec("pane", "send-text", target.Surface, text); err == nil {
			return fmt.Sprintf("✓ Sent to %s (+ copied)", target.Description), nil
		}
		// If Herdr failed, leave the reference on the clipboard.
	case "cmux":
		setCmd := exec.Command("cmux", "set-buffer", "--name", "default", "--", text)
		pasteCmd := exec.Command("cmux", "paste-buffer", "--surface", target.Surface)
		if setCmd.Run() == nil && pasteCmd.Run() == nil {
			return fmt.Sprintf("✓ Pasted to %s (+ copied)", target.Description), nil
		}
		cmd := exec.Command("cmux", "send", "--surface", target.Surface, "--", text)
		if err := cmd.Run(); err == nil {
			return fmt.Sprintf("✓ Sent to %s (+ copied)", target.Description), nil
		}
		// If cmux failed, fall through to clipboard
	case "tmux":
		setCmd := exec.Command("tmux", "set-buffer", "--", text)
		// -p enables bracketed paste mode in tmux so multi-line text is pasted safely
		pasteCmd := exec.Command("tmux", "paste-buffer", "-p", "-t", target.Surface)
		if setCmd.Run() == nil && pasteCmd.Run() == nil {
			return fmt.Sprintf("✓ Sent to tmux pane %s (+ copied)", target.Surface), nil
		}
		// If tmux failed, fall through to clipboard
	}

	if target.Type == "clipboard" {
		return "✓ Copied to clipboard (Paste into chat)", nil
	}
	return "✓ Copied to clipboard", nil
}

// getCmuxCurrentSurface queries `cmux identify` to find the caller's actual surface_ref (e.g. "surface:6").
func getCmuxCurrentSurface() string {
	if out, err := exec.Command("cmux", "identify").Output(); err == nil {
		var info struct {
			Caller struct {
				SurfaceRef string `json:"surface_ref"`
			} `json:"caller"`
		}
		if json.Unmarshal(out, &info) == nil && info.Caller.SurfaceRef != "" {
			return info.Caller.SurfaceRef
		}
	}
	return os.Getenv("CMUX_SURFACE_ID")
}

func findCmuxPeer() (string, bool) {
	if _, err := exec.LookPath("cmux"); err != nil {
		return "", false
	}
	current := getCmuxCurrentSurface()
	if current == "" && os.Getenv("CMUX_SURFACE_ID") == "" && os.Getenv("CMUX_WORKSPACE_ID") == "" {
		return "", false
	}

	out, err := exec.Command("cmux", "list-panels").Output()
	if err != nil {
		return "", false
	}

	return ParseCmuxPanels(string(out), current)
}

// cmuxPanelRunsSyl checks the displayed command, not arbitrary words in the
// terminal title (e.g. a Pi chat titled "Fix Syl session detection").
func cmuxPanelRunsSyl(rest string) bool {
	title := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "[focused]"))
	title = strings.TrimPrefix(title, "\"")
	fields := strings.Fields(title)
	if len(fields) == 0 {
		return false
	}
	return filepath.Base(strings.TrimSuffix(fields[0], "\"")) == "syl"
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
		if currentSurface != "" && (surfaceID == currentSurface || strings.EqualFold(surfaceID, currentSurface)) {
			continue
		}
		// Skip any panel running syl (e.g. "syl", "./bin/syl", "bin/syl", "/path/to/syl")
		if cmuxPanelRunsSyl(rest) {
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

// ParseCmuxResumeSession returns the transcript of a Pi session bound to a cmux
// surface. Other restore records (including arbitrary shell commands) are not
// conversation transcripts.
func ParseCmuxResumeSession(data []byte) string {
	var record struct {
		Cleared       bool `json:"cleared"`
		RestoreRecord struct {
			Kind         string `json:"kind"`
			CheckpointID string `json:"checkpoint_id"`
		} `json:"restore_record"`
	}
	if json.Unmarshal(data, &record) != nil || record.Cleared || record.RestoreRecord.Kind != "pi" {
		return ""
	}
	path := record.RestoreRecord.CheckpointID
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".jsonl" {
		return ""
	}
	return path
}

// PeerSessionFile finds the exact session bound to the adjacent agent pane.
// Herdr reports the live agent session; cmux may have a Pi restore checkpoint.
func PeerSessionFile() string {
	if HerdrAvailable() {
		return HerdrPeerSessionFile()
	}
	peer, ok := findCmuxPeer()
	if !ok {
		return ""
	}
	out, err := exec.Command("cmux", "surface", "resume", "show", "--surface", peer, "--json").Output()
	if err != nil {
		return ""
	}
	return ParseCmuxResumeSession(out)
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

// ParseCmuxAgent inspects the peer terminal surfaces to detect if an AI assistant CLI is running.
func ParseCmuxAgent(output string, currentSurface string) string {
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

		surfaceID := matches[2]
		panelType := matches[3]
		rest := strings.ToLower(matches[4])

		if panelType != "terminal" {
			continue
		}
		if currentSurface != "" && (surfaceID == currentSurface || strings.EqualFold(surfaceID, currentSurface)) {
			continue
		}
		if cmuxPanelRunsSyl(rest) {
			continue
		}

		if strings.Contains(rest, "agy") || strings.Contains(rest, "antigravity") {
			return "antigravity"
		}
		if strings.Contains(rest, "claude") {
			return "claude"
		}
		if strings.Contains(rest, "pi") {
			return "pi"
		}
	}
	return ""
}

// ParseTmuxAgent inspects tmux panes to detect if an AI assistant CLI is running.
func ParseTmuxAgent(output string, currentPane string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		paneID := fields[0]
		if paneID == currentPane {
			continue
		}
		cmd := strings.ToLower(fields[2])
		if strings.Contains(cmd, "agy") || strings.Contains(cmd, "antigravity") {
			return "antigravity"
		}
		if strings.Contains(cmd, "claude") {
			return "claude"
		}
		if strings.Contains(cmd, "pi") {
			return "pi"
		}
	}
	return ""
}

// DetectPeerAgent inspects adjacent multiplexer panes to identify
// which AI assistant (e.g. "antigravity", "claude", "pi") is running alongside syl.
func DetectPeerAgent() string {
	if HerdrAvailable() {
		if peer, err := HerdrPeer(); err == nil {
			return peer.Agent
		}
		return ""
	}
	// 1. Check cmux
	if _, err := exec.LookPath("cmux"); err == nil {
		if out, err := exec.Command("cmux", "list-panels").Output(); err == nil {
			if agent := ParseCmuxAgent(string(out), getCmuxCurrentSurface()); agent != "" {
				return agent
			}
		}
	}

	// 2. Check tmux
	if os.Getenv("TMUX") != "" {
		if _, err := exec.LookPath("tmux"); err == nil {
			if out, err := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_active} #{pane_current_command}").Output(); err == nil {
				if agent := ParseTmuxAgent(string(out), os.Getenv("TMUX_PANE")); agent != "" {
					return agent
				}
			}
		}
	}

	return ""
}

// ParseCmuxPeerCWD extracts the working directory of the peer terminal surface from `cmux debug-terminals`.
func ParseCmuxPeerCWD(output string, peerSurface string) string {
	if peerSurface == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	inTargetSurface := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.Contains(line, peerSurface) {
			inTargetSurface = true
		} else if inTargetSurface && strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "surface:") {
			inTargetSurface = false
		}

		if inTargetSurface {
			fields := strings.Fields(line)
			for _, f := range fields {
				if strings.HasPrefix(f, "cwd=") {
					return strings.TrimPrefix(f, "cwd=")
				}
			}
		}
	}
	return ""
}

// ParseTmuxPeerCWD extracts the working directory of the peer tmux pane from `tmux list-panes -F "#{pane_id} #{pane_current_path}"`.
func ParseTmuxPeerCWD(output string, peerPane string) string {
	if peerPane == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == peerPane {
			return fields[1]
		}
	}
	return ""
}

// DetectPeerCWD inspects adjacent multiplexer panes to identify
// the current working directory of the terminal running alongside syl.
func DetectPeerCWD() string {
	if HerdrAvailable() {
		if peer, err := HerdrPeer(); err == nil {
			if peer.ForegroundCWD != "" {
				return peer.ForegroundCWD
			}
			return peer.CWD
		}
		return ""
	}
	// 1. Check cmux
	if _, err := exec.LookPath("cmux"); err == nil {
		if peer, ok := findCmuxPeer(); ok {
			if out, err := exec.Command("cmux", "debug-terminals").Output(); err == nil {
				if cwd := ParseCmuxPeerCWD(string(out), peer); cwd != "" {
					return cwd
				}
			}
		}
	}

	// 2. Check tmux
	if os.Getenv("TMUX") != "" {
		if _, err := exec.LookPath("tmux"); err == nil {
			if peer, ok := findTmuxPeer(); ok {
				if out, err := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_current_path}").Output(); err == nil {
					if cwd := ParseTmuxPeerCWD(string(out), peer); cwd != "" {
						return cwd
					}
				}
			}
		}
	}

	return ""
}
