package multiplexer

import (
	"testing"
)

func TestParseCmuxPanels(t *testing.T) {
	output := `* surface:5  terminal  [focused]  "agyd"
  surface:6  terminal  "syl --harness agy"`

	// When current surface is surface:6, should pick surface:5
	peer, ok := ParseCmuxPanels(output, "surface:6")
	if !ok {
		t.Fatalf("expected to find peer surface")
	}
	if peer != "surface:5" {
		t.Errorf("expected surface:5, got %s", peer)
	}

	// When current surface is not set, should skip syl and pick surface:5
	peerNoCurrent, ok := ParseCmuxPanels(output, "")
	if !ok || peerNoCurrent != "surface:5" {
		t.Errorf("expected surface:5 when currentSurface is empty, got %s", peerNoCurrent)
	}

	// Mixed with browser surface
	mixedOutput := `  surface:1  browser   "Documentation"
  surface:2  terminal  "syl"
* surface:3  terminal  [focused]  "claude"`

	peerMixed, ok := ParseCmuxPanels(mixedOutput, "surface:2")
	if !ok || peerMixed != "surface:3" {
		t.Errorf("expected surface:3 from mixed output, got %s", peerMixed)
	}

	// Only syl running
	onlySyl := `  surface:6  terminal  "syl --harness agy"`
	_, okOnlySyl := ParseCmuxPanels(onlySyl, "surface:6")
	if okOnlySyl {
		t.Errorf("expected no peer found when only syl is running")
	}
}

func TestParseTmuxPanes(t *testing.T) {
	output := `%0 1 zsh
%1 0 syl`

	// Current pane is %1, should pick %0
	peer, ok := ParseTmuxPanes(output, "%1")
	if !ok {
		t.Fatalf("expected to find tmux peer pane")
	}
	if peer != "%0" {
		t.Errorf("expected %%0, got %s", peer)
	}

	// Only 1 pane
	singleOutput := `%1 1 syl`
	_, okSingle := ParseTmuxPanes(singleOutput, "%1")
	if okSingle {
		t.Errorf("expected no peer found when single pane")
	}
}

func TestDiscoverPeerFallback(t *testing.T) {
	// With empty target and simulated non-multiplexer env
	t.Setenv("TMUX", "")
	t.Setenv("CMUX_SURFACE_ID", "")
	t.Setenv("CMUX_WORKSPACE_ID", "")

	target := DiscoverPeer("")
	if target.Type != "clipboard" && target.Type != "cmux" {
		t.Errorf("expected clipboard or cmux target, got %s", target.Type)
	}
}
