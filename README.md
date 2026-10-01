# syl

<div align="center">

**A live reference pane for AI conversations.**

[![CI](https://github.com/Fhuss/syl/actions/workflows/ci.yml/badge.svg)](https://github.com/Fhuss/syl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Fhuss/syl?color=blue&logo=github)](https://github.com/Fhuss/syl/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Fhuss/syl)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

*Conversations scroll away. Reference material shouldn't.*

</div>

---

`syl` is a terminal companion that sits quietly beside your AI coding assistant (in `cmux`, `tmux`, or split terminal panes). It passively observes completed conversation turns and extracts, structures, and continuously maintains **procedures, runnable commands, architecture decisions, and open questions** — persisted across restarts.

```
┌───────────────────────────────────────┬────────────────────────────────────────────────────────┐
│  AI Coding Terminal                   │  syl (Reference Companion Pane)                        │
│                                       ├──────────────────────────┬─────────────────────────────┤
│  > deploy the app with docker compose │  [CARDS]                 │  [PREVIEW]                  │
│                                       │                          │                             │
│  Assistant:                           │  ▶ 1. Docker Deployment  │  # Docker Deployment (v2)   │
│  Here is the updated command:         │     2. AWS Credentials   │  Run the compose service:   │
│  $ docker compose up -d               │     3. Redis Setup       │  ```bash                    │
│                                       │                          │  docker compose up -d       │
│  ... (30 messages scroll past) ...    │                          │  ```                        │
│                                       │                          │  Press [b] to yank 1-key    │
└───────────────────────────────────────┴──────────────────────────┴─────────────────────────────┘
```

---

## The Problem: Procedural Decay

An AI assistant gives you a useful 6-step deployment procedure. You ask 4 follow-up questions, debug an authentication error, and clarify your target region. By the time you are ready to deploy, the original instructions have scrolled far off-screen.

If a later answer corrected step 3 or modified a configuration parameter, you must scroll back and manually deduce which version across your conversation history is safe to run. 

Conversational scrollback causes **procedural decay**. `syl` solves this by keeping a synchronized, living reference pane pinned right beside your terminal.

---

## Core Capabilities

### ⚡ Passive Observation & Zero-Prompt Curation
`syl` requires no special commands like *"jot this down"* or *"save to notes"*. It tails conversation transcripts (Antigravity, Claude Code, Pi, or custom logs) and synthesizes clean reference cards in real-time as your assistant answers.

### 🔄 In-Place Mutation & Versioning (`v1` → `v2`)
When a follow-up answer fixes an error or updates a flag, `syl` patches the existing reference card in-place and increments its revision (`v1` → `v2` → `v3`). Disproven steps are marked invalidated rather than cluttering your screen with duplicate cards.

### 🎯 Adjacent Workspace Awareness & Auto-Attach
When launched in an adjacent terminal (via `cmux` or `tmux`), `syl` automatically detects the peer pane's working directory. It connects directly to conversations in that workspace without confusing unrelated projects.

### ⏳ Waiting for Conversation Mode (`syl -n` / `N`)
Start a brand new session anytime by running `syl -n` or pressing **`N`** inside the TUI. `syl` clears the reference pane and displays a listening companion banner. As soon as you type your first prompt in the adjacent terminal, `syl` automatically detects the conversation and begins streaming cards.

### 📦 Surgical Block & Step Picker (`b`)
Press **`b`** on any card to scan and isolate actionable commands, numbered procedure steps, files, and URLs.
* **1-Key Yanking**: Press **`1`**–**`9`** to copy a specific block to your system clipboard instantly.
* **Direct Execution**: Press **`s`** to send the command directly into your active chat surface.
* **Browser & File Opener**: Press **`o`** to open detected URLs or preview workspace files.

### 🚀 Two-Way Terminal Bridge (`s`)
Press **`s`** on any card to inject a reference tag (`[Ref: Card Title (vN)] `) into your adjacent chat pane. Your cursor is placed directly after the tag so you can ask follow-ups or report error logs with zero copy-pasting.

### ⌨️ Dual-Pane Vim Navigation (`w`, `j`/`k`)
Seamless keyboard ergonomics built for developers:
* **Focus Switch (`w`)**: Toggle focus between the **`[CARDS]`** turns column and the **`[PREVIEW]`** buffer.
* **Vim Motions**: `Ctrl+d`/`Ctrl+u` (half page), `Ctrl+e`/`Ctrl+y` (1-line nudge), `{`/`}` (paragraph jump), `gg`/`G` (top/bottom).
* **In-Buffer Search (`/`)**: Find matches within the active card preview and jump with **`n`** / **`N`**.

### 📝 External `$EDITOR` Roundtrip (`v` / `E`)
Press **`v`** or **`E`** to suspend `syl` and open the current card in your editor of choice (`nvim`, `vim`, `nano`, or `code --wait`). Saved edits are automatically reloaded and persisted.

### ☑️ Interactive Markdown Checklist (`Space`)
Press **`Space`** on cards containing task items (`- [ ]`) to check off steps interactively (`- [x]`). Once completed, pressing `Space` resets tasks back for repeatable workflows.

### 🗂️ Code Block Folding (`za` / `Z`)
Press **`za`** or **`Z`** to toggle code block folding on the active card, collapsing bulky multi-line blocks into concise quotes (`> ▾ [bash] 14 lines folded`).

### 💾 Safe Persistence & Trash Lifecycle
Cards, titles, pinned state, revision history, and the last 10 dropped cards are saved atomically to `~/.config/syl/sessions/<session-id>.json`:
* **Undo Drops (`U`)**: Instantly restore the most recently dropped card, even across restarts.
* **Trash Inspector (`T`)**: Browse dropped cards, restore with `Enter`, or purge with `x` then `y`.
* **Revision Time-Travel (`h`)**: Browse prior card contents with `←`/`→` and restore selected historical content with `Enter`.

---

## Installation

### Pre-built Binaries (GitHub Releases)
Download the latest pre-compiled binary for your architecture from [GitHub Releases](https://github.com/Fhuss/syl/releases):

```bash
# macOS (Apple Silicon arm64)
curl -fsSL https://github.com/Fhuss/syl/releases/latest/download/syl_Darwin_arm64.tar.gz | tar -xz
sudo mv syl /usr/local/bin/

# macOS (Intel x86_64)
curl -fsSL https://github.com/Fhuss/syl/releases/latest/download/syl_Darwin_x86_64.tar.gz | tar -xz
sudo mv syl /usr/local/bin/

# Linux (amd64)
curl -fsSL https://github.com/Fhuss/syl/releases/latest/download/syl_Linux_x86_64.tar.gz | tar -xz
sudo mv syl /usr/local/bin/
```

### Install with Go
```bash
go install github.com/Fhuss/syl/cmd/syl@latest
```

### Build From Source
```bash
git clone https://github.com/Fhuss/syl.git
cd syl
go build -o ~/.local/bin/syl ./cmd/syl
```

---

## Quickstart

### 1. Companion Agent Skill Setup
Install Syl's companion skill into your installed AI assistants with one command:

```bash
syl install-skill
```
*Supports Antigravity, Claude Code, and Pi Agent.*

### 2. Launching Syl
Open a split pane adjacent to your AI terminal and run:

```bash
# Auto-detects adjacent workspace and resumes where you were
syl

# Or start fresh: clear pane and wait for your next prompt
syl --new
```

### 3. Interactive Feature Tour (Demo Mode)
Explore live card mutation, checklist toggles, and shortcuts without needing an active agent:

```bash
syl demo
```

---

## CLI Usage

```
Usage:
  syl [command|flags]

Commands:
  new / clear        Clear reference pane and wait for new conversation
  demo               Run interactive simulation demonstrating live card mutation
  sessions           List active and recent conversation sessions to choose from
  install-skill      Install companion skill into Pi Agent, Claude Code, or Antigravity
  launch / start     Launch Syl in an adjacent right-hand split (cmux/tmux)
  init-config        Generate default config file in ~/.config/syl/config.yaml
  version            Print version information

Flags:
  -n, --new            Start a new session / clear pane and wait for conversation
  -s, --session string Attach to a specific conversation session by ID or prefix
  -f, --file string    Path to transcript.jsonl (auto-detected if omitted)
  -c, --config string  Path to config file (default ~/.config/syl/config.yaml)
  -p, --provider string LLM provider override (mock, openai, gemini, anthropic, ollama)
  --harness string     Subagent harness override (auto, agy, claude, pi, ollama, heuristic)
  -h, --help           Show help message
```

---

## Keyboard Reference

Press **`?`** inside `syl` anytime to open the interactive help glossary.

| Key | Action |
| :--- | :--- |
| `w` / `Ctrl+w` | Toggle dual-pane focus (**[CARDS]** vs **[PREVIEW]**) |
| `j` / `k` | Navigate turns in Cards pane; scroll buffer in Preview pane |
| `↑` / `↓` | Scroll preview content line-by-line |
| `PgUp` / `PgDn` | Scroll preview page up / down |
| `Ctrl+d` / `Ctrl+u` | Half-page scroll down / up |
| `Ctrl+e` / `Ctrl+y` | Vim 1-line nudge down / up |
| `{` / `}` | Paragraph scroll jump (5 lines up / down) |
| `gg` / `G` | Jump to top / bottom of preview buffer |
| `/` | Live fuzzy search across card titles, commands, and content |
| `n` / `N` | Jump to next / previous search match in preview buffer |
| `N` / `Ctrl+n` | Clear pane and wait for new conversation (auto-attaches upon prompt) |
| `b` | Surgical block & step picker (yank `1`–`9`, run `s`, open URL/file `o`) |
| `s` | Send card reference or block to peer terminal (`cmux`, `tmux`, clipboard) |
| `v` / `E` | Open active card in `$EDITOR` (`nvim`/`vim`), reloading changes on exit |
| `za` / `Z` | Fold / unfold fenced code blocks |
| `␣` (`Space`) | Toggle markdown checklist item (`- [ ]` → `- [x]`) |
| `S` | Switch active conversation session without quitting |
| `u` | Open card URL directly in default browser |
| `c` | Copy code snippet(s) from card to clipboard |
| `C` / `y` | Copy full card markdown to clipboard |
| `r` | Toggle raw text mode (clean for mouse selection) |
| `p` | Pin / unpin card |
| `x` / `d` | Drop a card from the reference pane |
| `U` | Undo the most recent drop (up to 10, including after restart) |
| `T` | Browse Trash; restore with `Enter` or purge with `x` then `y` |
| `h` | Preview card revisions; `←`/`→` to browse, `Enter` to restore |
| `e` | Export all session cards to `./syl-reference.md` |
| `R` | Retry failed card capture |
| `?` / `F1` | Open help modal |
| `q` / `Ctrl+C` | Quit (persists session snapshot) |

---

## Architecture

```
┌──────────────────────┐
│   File Watcher       │  Tails transcript.jsonl (Antigravity / Claude Code / Pi / custom)
└──────────┬───────────┘
           │ turn events (user prompt + assistant response)
           ▼
┌──────────────────────┐
│   Reconciler         │  Emits ADD / UPDATE / INVALIDATE mutations
│                      │  Ref matching, step preservation & elaboration appending
└──────────┬───────────┘
           │ delta operations
           ▼
┌──────────────────────┐
│   Card Store         │  Versioned cards with revision history, pin/drop state
│                      │  Persisted → ~/.config/syl/sessions/<session-id>.json
└──────────┬───────────┘
           │ reactive updates
           ▼
┌──────────────────────┐
│   TUI Engine         │  Bubble Tea + Glamour markdown + Lipgloss
│                      │  Split view | Focus view | Block picker | Live search
└──────────────────────┘
```

---

## Contributing

Contributions, bug reports, and feature requests are very welcome! Please check out [CONTRIBUTING.md](CONTRIBUTING.md) for local development setup, testing, and pull request guidelines.

---

## License

[MIT](LICENSE) © 2026 Fabian Huss
