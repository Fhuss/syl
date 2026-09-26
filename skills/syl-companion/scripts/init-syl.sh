#!/usr/bin/env bash
# ==============================================================================
# init-syl.sh — Initialize Syl companion pane to the right of the active agent
# ==============================================================================
# Usage:
#   init-syl.sh [--force] [--check]
#
# Supported multiplexers and environments:
#   1. cmux (Ghostty / cmux socket) -> cmux new-split right
#   2. tmux -> tmux split-window -h -p 35
#   3. iTerm2 -> AppleScript vertical split
#   4. macOS Terminal.app -> side-by-side companion window
# ==============================================================================

set -euo pipefail

FORCE=0
CHECK_ONLY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --force|-f)
      FORCE=1
      shift
      ;;
    --check|-c)
      CHECK_ONLY=1
      shift
      ;;
    -h|--help)
      echo "Usage: init-syl.sh [--force] [--check]"
      echo "Initializes Syl in a split pane to the right of your current assistant session."
      exit 0
      ;;
    *)
      shift
      ;;
  esac
done

# ------------------------------------------------------------------------------
# 1. Locate syl executable
# ------------------------------------------------------------------------------
find_syl() {
  if [[ -n "${SYL_BIN:-}" && -x "${SYL_BIN:-}" ]]; then
    echo "$SYL_BIN"
    return 0
  fi
  if command -v syl &>/dev/null; then
    command -v syl
    return 0
  fi
  if [[ -x "$HOME/.local/bin/syl" ]]; then
    echo "$HOME/.local/bin/syl"
    return 0
  fi
  local git_root
  git_root=$(git rev-parse --show-toplevel 2>/dev/null || true)
  if [[ -n "$git_root" && -x "$git_root/bin/syl" ]]; then
    echo "$git_root/bin/syl"
    return 0
  fi
  return 1
}

SYL_PATH=$(find_syl || true)

if [[ -z "$SYL_PATH" ]]; then
  echo "⚠ syl executable not found in PATH or ~/.local/bin."
  local_root=$(git rev-parse --show-toplevel 2>/dev/null || true)
  if [[ -n "$local_root" && -f "$local_root/cmd/syl/main.go" ]]; then
    echo "→ Building syl from local repository source..."
    mkdir -p "$HOME/.local/bin"
    go build -o "$HOME/.local/bin/syl" "$local_root/cmd/syl"
    SYL_PATH="$HOME/.local/bin/syl"
    echo "✓ Built and installed to $SYL_PATH"
  else
    echo "Please build syl first: go build -o ~/.local/bin/syl ./cmd/syl"
    exit 1
  fi
fi

# ------------------------------------------------------------------------------
# 2. Check if Syl is already running
# ------------------------------------------------------------------------------
check_existing_cmux() {
  if command -v cmux &>/dev/null && cmux ping &>/dev/null; then
    local panels
    panels=$(cmux list-panels 2>/dev/null || true)
    if echo "$panels" | grep -q "syl"; then
      local surface
      surface=$(echo "$panels" | grep "syl" | awk '{print $1}' | tr -d '*')
      echo "cmux surface ($surface)"
      return 0
    fi
  fi
  return 1
}

check_existing_tmux() {
  if [[ -n "${TMUX:-}" ]] && command -v tmux &>/dev/null; then
    local panes
    panes=$(tmux list-panes -F "#{pane_id} #{pane_current_command}" 2>/dev/null || true)
    if echo "$panes" | grep -qi "syl"; then
      local pane_id
      pane_id=$(echo "$panes" | grep -i "syl" | awk '{print $1}')
      echo "tmux pane ($pane_id)"
      return 0
    fi
  fi
  return 1
}

if [[ $CHECK_ONLY -eq 1 ]]; then
  if running_target=$(check_existing_cmux); then
    echo "✓ Syl is currently active in $running_target"
    exit 0
  elif running_target=$(check_existing_tmux); then
    echo "✓ Syl is currently active in $running_target"
    exit 0
  elif pgrep -x syl &>/dev/null; then
    echo "✓ Syl process is running (PID: $(pgrep -x syl | head -1))"
    exit 0
  else
    echo "○ Syl is not currently running."
    exit 1
  fi
fi

if [[ $FORCE -eq 0 ]]; then
  if running_target=$(check_existing_cmux); then
    echo "✓ Syl is already running in $running_target. Pass --force to open an additional pane."
    exit 0
  elif running_target=$(check_existing_tmux); then
    echo "✓ Syl is already running in $running_target. Pass --force to open an additional pane."
    exit 0
  fi
fi

# ------------------------------------------------------------------------------
# 3. Environment Detection & Splitting
# ------------------------------------------------------------------------------

# Case A: cmux (Ghostty multiplexer)
if command -v cmux &>/dev/null && cmux ping &>/dev/null; then
  echo "→ Detected cmux session. Creating right-hand split..."

  # Record existing surface IDs before split
  surfaces_before=$(cmux list-panels 2>/dev/null | awk '{print $1}' | tr -d '*' | sort || true)

  # Create split to the right (keeping focus in current agent window)
  cmux new-split right --focus false 2>/dev/null || cmux new-split right

  # Find the new surface ID
  surfaces_after=$(cmux list-panels 2>/dev/null | awk '{print $1}' | tr -d '*' | sort || true)
  new_surface=$(comm -13 <(echo "$surfaces_before") <(echo "$surfaces_after") | head -1 || true)

  if [[ -n "$new_surface" ]]; then
    # Launch syl inside the newly created surface
    cmux send --surface "$new_surface" "$SYL_PATH\n"
    echo "✓ Launched Syl in cmux split ($new_surface) to the right!"
    exit 0
  else
    # Fallback: send to focused or active split
    cmux send "$SYL_PATH\n"
    echo "✓ Launched Syl in cmux right split!"
    exit 0
  fi
fi

# Case B: tmux
if [[ -n "${TMUX:-}" ]] && command -v tmux &>/dev/null; then
  echo "→ Detected tmux session. Creating vertical split (35% width)..."
  tmux split-window -h -p 35 "$SYL_PATH"
  echo "✓ Launched Syl in tmux pane to the right!"
  exit 0
fi

# Case C: iTerm2 on macOS
if [[ "${TERM_PROGRAM:-}" == "iTerm.app" ]] && [[ "$(uname)" == "Darwin" ]]; then
  echo "→ Detected iTerm2. Splitting session vertically..."
  osascript <<EOF >/dev/null 2>&1
tell application "iTerm2"
  tell current session of current window
    split vertically with default profile command "$SYL_PATH"
  end tell
end tell
EOF
  echo "✓ Launched Syl in iTerm2 split to the right!"
  exit 0
fi

# Case D: macOS Terminal.app / Generic GUI Fallback
if [[ "$(uname)" == "Darwin" ]]; then
  echo "→ Opening companion Terminal window on the right side of the screen..."
  osascript <<EOF >/dev/null 2>&1
tell application "Terminal"
  do script "$SYL_PATH"
  activate
  set theBounds to bounds of front window
  -- Position on right half of display (assuming standard ~1440+ width)
  set item 1 of theBounds to 800
  set item 2 of theBounds to 50
  set item 3 of theBounds to 1440
  set item 4 of theBounds to 900
  set bounds of front window to theBounds
end tell
EOF
  echo "✓ Launched Syl in companion Terminal window to the right!"
  exit 0
fi

# Generic fallback
echo "⚠ No supported multiplexer (cmux, tmux, iTerm2) detected."
echo "You can run Syl manually in an adjacent terminal:"
echo "  $SYL_PATH"
exit 1
