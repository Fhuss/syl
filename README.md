# syl

> **A live reference pane for AI conversations.**

`syl` sits quietly beside your AI chat in a terminal multiplexer (`cmux`, `tmux`, or split terminals) and automatically extracts and maintains the useful reference material from each conversation turn: **procedures, runnable commands, architecture decisions, and open questions** — persisted across restarts.

---

## The Problem

An assistant gives you a useful 6-step AWS deployment procedure. You ask 4 follow-up questions, debug an authentication error, and clarify your target region. The procedure has now scrolled far off-screen.

If a later answer corrected step 3 or changed a configuration parameter, you must remember which version in the transcript history is safe to run. Conversational scrollback causes **procedural decay**.

---

## How `syl` Works

1. **Passive Turn Observation** — Watches completed turns from AI sessions (Antigravity, Claude Code, Pi, or custom streams) via a tail-following file watcher. No _"jot this down"_ command required.
2. **Live In-Place Mutation & Versioning** — When a follow-up answer fixes an error or changes a configuration, `syl` patches the existing card in-place and increments the version (`v1` → `v2` → `v3`) rather than cluttering your view with duplicates. Disproven steps are marked invalidated.
3. **Surgical Block & Step Picker (`b`)** — Scans the active card and assigns 1-key badges (`[1]`, `[2]`, `[3]`) to discrete code blocks, numbered procedure steps, and URLs for instantaneous copying or sending.
4. **Two-Way Terminal Bridge (`s`)** — Injects card references (`[Ref: Title (vN)] `) or isolated commands directly into your adjacent chat terminal or multiplexer pane (`cmux`, `tmux`) with automatic clipboard backup.
5. **Smart Reconciler & Companion Skill** — Recognizes `[Ref: ...]` headers and smoothly appends elaborations and patched steps without losing original context. Paired with the `syl-companion` agent skill for maximum conversational symbiosis.
6. **Zero-Config Auto-Detection** — Automatically resolves the active session and host agent harness (`agy`, `claude`, `pi`, `heuristic`).
7. **Session Persistence** — Cards, titles, pinned state, and revision history are saved to `~/.config/syl/sessions/<session-id>.json` on every change and restored instantly on restart.

---

## Key Features

### 📦 Surgical Block & Step Picker (`b`)
Press **`b`** on any card to open the actionable block modal:
* **Code Blocks:** Tagged by language (`[BASH]`, `[SQL]`, `[DOCKER]`, `[YAML]`).
* **Numbered Steps:** Extracts inline commands from steps (e.g. `[STEP 2]`).
* **URLs:** Direct links (`[URL]`).
* **Key Actions:** Press **`1`**–**`9`** or **`0`** for instant 1-key yank to clipboard, **`s`** to send to your active terminal surface, or **`o`** to open in your browser.

### 🌐 Direct Browser Opener (`u`)
Press **`u`** to immediately launch detected URLs in your default browser (`open <url>`). If multiple URLs exist, opens a quick selector.

### 🔍 Interactive Live Search (`/`)
Press **`/`** to open a real-time search bar at the bottom. Instant fuzzy filtering across card titles, commands, and content to jump to any turn in seconds.

### 🔄 In-TUI Session Switcher (`S`)
Press **`S`** to open an interactive session switcher modal listing recent conversations across Antigravity, Claude Code, and Pi. Switch active sessions on the fly without exiting `syl`.

### 🚀 Two-Way Bridge (`s`)
Press **`s`** on any card to paste a clean `[Ref: Card Title (vN)] ` reference directly into your chat terminal. Your cursor is placed right after the tag so you can type questions or report errors without accidental submits. Inside the Block Picker (`b`), pressing **`s`** sends the highlighted command.

---

## Architecture

```
┌──────────────────────┐
│   File Watcher       │  Tails transcript.jsonl (Antigravity / Claude Code / custom)
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
│   Harness            │  Auto-detects agy / claude / pi / heuristic for titles
│                      │  Title cache → ~/.config/syl/titles.json
└──────────┬───────────┘
           │ TitleSummarizedMsg
           ▼
┌──────────────────────┐
│   TUI Engine         │  Bubble Tea + Glamour markdown + Lipgloss
│                      │  Split view | Focus view | Block picker | Live search
└──────────────────────┘
```

---

## Installation & Setup

### Build from source

```bash
git clone https://github.com/Fhuss/syl.git
cd syl
go build -o ~/.local/bin/syl ./cmd/syl
```

*(Ensure `~/.local/bin` is in your `$PATH` to run `syl` from any directory).*

---

## Quickstart

### Run with auto-detection (Zero-Config)

```bash
# Automatically attaches to your latest active AI conversation
syl
```

### Demo Mode (No AI session required)

```bash
syl demo
```
Replays an interactive multi-turn scenario to demonstrate live card patching and in-place versioning. Press `Space` to advance turns.

### CLI Options

```bash
# (Optional) Explicit harness override
syl --harness agy        # Force Antigravity
syl --harness claude     # Force Claude Code
syl --harness heuristic  # Fast offline fallback (no AI calls)

# Point at a specific transcript file
syl --file /path/to/transcript.jsonl

# Attach to a specific session by ID or prefix
syl --session 40c1c89b

# List recent sessions across all assistants
syl sessions
```

### Configuration

Generate a starter configuration file:

```bash
syl init-config
# writes to ~/.config/syl/config.yaml
```

```yaml
llm:
  provider: gemini          # gemini | anthropic | openai | ollama | mock
  model: gemini-2.0-flash
  api_key: ${GEMINI_API_KEY}

subagent:
  harness: auto             # auto | agy | claude | pi | ollama | heuristic
  disabled: false
```

---

## Keyboard Reference

Press `?` inside `syl` anytime to open the interactive help glossary.

| Key | Action |
| :--- | :--- |
| `↑` / `↓` | Scroll preview content up / down |
| `PgUp` / `PgDn` | Scroll preview page up / down |
| `Ctrl+u` / `Ctrl+d` | Half-page scroll |
| `j` / `k` | Switch to next / previous card |
| `←` / `→` | Switch to next / previous card |
| `[` / `]` | Switch to previous / next card |
| `Tab` / `Enter` | Toggle focus view (hide/show turns column) |
| `b` | Surgical block & step picker (yank `1`–`9`, run `s`, open URL `o`) |
| `u` | Open card URL directly in browser |
| `/` | Quick search & filter cards interactively |
| `S` | Switch active conversation session without quitting |
| `c` | Copy code snippet(s) from card to clipboard |
| `C` / `y` | Copy full card markdown to clipboard |
| `r` | Toggle raw text mode (clean for mouse selection) |
| `p` | Pin / unpin card |
| `x` / `d` | Drop / delete card from reference pane |
| `e` | Export all session cards to `./syl-reference.md` |
| `h` | Toggle revision history for selected card |
| `s` | Send card reference or block to peer terminal (`cmux`, `tmux`, clipboard) |
| `?` / `F1` | Toggle help glossary modal |
| `Space` | Advance turn (demo mode only) |
| `q` / `Ctrl+C` | Quit (saves session snapshot to disk) |

---

## Agent Companion Skill

Syl comes with a symbiotic agent skill (`syl-companion`) located in [`skills/syl-companion/SKILL.md`](skills/syl-companion/SKILL.md).

When installed into your assistant environment (e.g. Antigravity, Claude Code), it guides the model to:
1. Format commands and procedures into discrete, isolated runnable blocks for Syl's Block Picker (`b`).
2. Recognize incoming `[Ref: Card Title (vN)]` markers when you press **`s`**, immediately grounding replies in that card without friction.
3. Preserve step numbering during revisions to enable clean in-place card updates (`v1` → `v2`).

---

## License

MIT
