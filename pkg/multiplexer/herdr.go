package multiplexer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// HerdrPane is the workspace-scoped terminal metadata needed by Syl.
type HerdrPane struct {
	PaneID        string `json:"pane_id"`
	Agent         string `json:"agent"`
	CWD           string `json:"cwd"`
	ForegroundCWD string `json:"foreground_cwd"`
	AgentSession  struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	} `json:"agent_session"`
}

func HerdrAvailable() bool {
	return os.Getenv("HERDR_ENV") == "1" && os.Getenv("HERDR_PANE_ID") != ""
}

func herdrOutput(args ...string) ([]byte, error) {
	binary := os.Getenv("HERDR_BIN_PATH")
	if binary == "" {
		binary = "herdr"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		// Do not include a pane's submitted command or chat text in errors.
		return nil, fmt.Errorf("herdr %s: %w: %s", strings.Join(args[:min(2, len(args))], " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// HerdrExec accepts successful commands that print no JSON, such as pane run
// and pane send-text. A successful exit confirms only queue admission.
func HerdrExec(args ...string) error {
	_, err := herdrOutput(args...)
	return err
}

// HerdrCall runs a Herdr CLI command and decodes its JSON result.
func HerdrCall(result any, args ...string) error {
	out, err := herdrOutput(args...)
	if err != nil {
		return err
	}
	var response struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil || len(response.Result) == 0 {
		return fmt.Errorf("invalid herdr %s response: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf("decode herdr %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// HerdrTabPanes uses an explicit pane ID, never the server's globally focused pane.
// This matters when multiple Herdr clients are attached to the same server.
func HerdrTabPanes(paneID string) ([]string, error) {
	var result struct {
		Layout struct {
			Panes []struct {
				PaneID string `json:"pane_id"`
			} `json:"panes"`
		} `json:"layout"`
	}
	if err := HerdrCall(&result, "pane", "layout", "--pane", paneID); err != nil {
		return nil, err
	}
	if len(result.Layout.Panes) == 0 {
		return nil, fmt.Errorf("herdr returned no panes for %s", paneID)
	}
	panes := make([]string, 0, len(result.Layout.Panes))
	for _, pane := range result.Layout.Panes {
		if pane.PaneID != "" {
			panes = append(panes, pane.PaneID)
		}
	}
	return panes, nil
}

func HerdrGetPane(paneID string) (HerdrPane, error) {
	var result struct {
		Pane HerdrPane `json:"pane"`
	}
	if err := HerdrCall(&result, "pane", "get", paneID); err != nil {
		return HerdrPane{}, err
	}
	if result.Pane.PaneID != paneID {
		return HerdrPane{}, fmt.Errorf("herdr returned unexpected pane for %s", paneID)
	}
	return result.Pane, nil
}

// HerdrPeer finds an agent in the same tab as Syl. Refuse to guess between
// multiple agents; that could attach Syl to another conversation or send text
// to an unrelated terminal.
func HerdrPeer() (HerdrPane, error) {
	if !HerdrAvailable() {
		return HerdrPane{}, fmt.Errorf("not in a Herdr pane")
	}
	self := os.Getenv("HERDR_PANE_ID")
	panes, err := HerdrTabPanes(self)
	if err != nil {
		return HerdrPane{}, err
	}
	// A companion split to the right of its agent can coexist with other
	// agents in the tab. Prefer the immediate left neighbor when it is one.
	var neighbor struct {
		Neighbor struct {
			PaneID string `json:"pane_id"`
		} `json:"neighbor"`
	}
	if err := HerdrCall(&neighbor, "pane", "neighbor", "--pane", self, "--direction", "left"); err == nil && neighbor.Neighbor.PaneID != self {
		for _, id := range panes {
			if id == neighbor.Neighbor.PaneID {
				candidate, err := HerdrGetPane(id)
				if err == nil && candidate.Agent != "" {
					return candidate, nil
				}
				break
			}
		}
	}
	var peer HerdrPane
	for _, id := range panes {
		if id == self {
			continue
		}
		candidate, err := HerdrGetPane(id)
		if err != nil {
			return HerdrPane{}, err
		}
		if candidate.Agent == "" {
			continue
		}
		if peer.PaneID != "" {
			return HerdrPane{}, fmt.Errorf("multiple agents in Herdr tab; cannot choose Syl peer")
		}
		peer = candidate
	}
	return peer, nil
}

func HerdrPeerSessionFile() string {
	peer, err := HerdrPeer()
	if err != nil || peer.AgentSession.Kind != "path" {
		return ""
	}
	path := peer.AgentSession.Value
	if filepath.IsAbs(path) && filepath.Ext(path) == ".jsonl" {
		return path
	}
	return ""
}

// HerdrSylPane finds a foreground Syl process in this pane's tab. A title
// mentioning Syl is not sufficient evidence that Syl is actually running.
func HerdrSylPane(paneID string) (string, error) {
	panes, err := HerdrTabPanes(paneID)
	if err != nil {
		return "", err
	}
	for _, id := range panes {
		if id == paneID {
			continue
		}
		var result struct {
			ProcessInfo struct {
				ForegroundProcesses []struct {
					Name  string `json:"name"`
					Argv0 string `json:"argv0"`
				} `json:"foreground_processes"`
			} `json:"process_info"`
		}
		if err := HerdrCall(&result, "pane", "process-info", "--pane", id); err != nil {
			return "", err
		}
		for _, process := range result.ProcessInfo.ForegroundProcesses {
			if filepath.Base(process.Name) == "syl" || filepath.Base(process.Argv0) == "syl" {
				return id, nil
			}
		}
	}
	return "", nil
}
