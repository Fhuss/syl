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
7. **Session Persistence** — Cards, titles, pinned state, revision history, and the last 10 dropped cards are saved privately and atomically to `~/.config/syl/sessions/<session-id>.json` and restored on restart. Dropping is not secure erasure: the retained cards can be restored with `U` until evicted by later drops. If a snapshot is unreadable, Syl pauses capture and leaves the file untouched; back it up and repair or restore it before restarting.

---

## Key Features

### ⌨️ Dual-Pane Vim Navigation (`w`, `j`/`k`)
Toggle focus between the **`[CARDS]`** turns column and the **`[PREVIEW]`** markdown buffer using **`w`** or **`Ctrl+w`**:
* **In `[CARDS]`**: `j` / `k` navigates between conversation turns.
* **In `[PREVIEW]`**: `j` / `k` scrolls the markdown buffer line-by-line.
* **Universal Motions**: `Ctrl+e`/`Ctrl+y` (1-line nudge), `Ctrl+d`/`Ctrl+u` (half page), `Ctrl+f`/`Ctrl+b` (full page), `{`/`}` (5-line paragraph jump), and `gg`/`G` (top/bottom).
* **In-Buffer Search**: Press **`/`** to find matches, then **`n`** / **`N`** to jump to next / previous match in the card preview.

### 📝 External Editor Roundtrip (`v` / `E`)
Press **`v`** or **`E`** to suspend Syl and open the selected card in your external editor (`$EDITOR`, `nvim`, `vim`, or `nano`). `$EDITOR` can include arguments such as `code --wait`; quote executable paths containing spaces. Any edits you save are synchronized back to the original card and persisted upon exit.

### ☑️ Interactive Markdown Task Toggle (`␣` / `Space`)
Press **`Space`** while on a card containing task items (`- [ ]`) to automatically check off the next pending item (`- [x]`). Once all tasks are checked, pressing `Space` resets them back to unchecked for repeatable workflows.

### 🗂️ Code Block Folding (`za` / `Z`)
Press **`za`** or **`Z`** to toggle code block folding on the active card. Bulky multi-line code fences are collapsed into concise markdown quotes (`> ▾ [bash] 12 lines folded`), keeping your reference pane neat and readable.

### 📦 Surgical Block & Step Picker (`b`)
Press **`b`** on any card to open the actionable block modal:
* **Code Blocks:** Tagged by language (`[BASH]`, `[SQL]`, `[DOCKER]`, `[YAML]`).
* **Numbered Steps:** Extracts inline commands from steps (e.g. `[STEP 2]`).
* **URLs:** Direct links (`[URL]`).
* **Files:** Detects workspace files, `file://` links, and paths (`[GO]`, `[PY]`, `[TS]`, `[YAML]`, `[FILE]`).
* **Key Actions:** Press **`1`**–**`9`** or **`0`** for instant 1-key yank to clipboard, **`s`** to send to your active terminal surface, or **`o`** to open URLs in your browser or preview files with your default application.

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
# Attaches to the adjacent Pi session in cmux when available;
# otherwise prefers a Pi session from the current directory, then the latest session
syl
```

### Demo Mode (Interactive Feature Tour)

```bash
syl demo
```
Launches an interactive, zero-dependency tutorial showcasing live card mutations, tips & tricks, and all navigation controls:
* **Interactive Checklist (Card #2)**: Press **`Space`** to toggle task items live.
* **Code Folding (Card #3)**: Press **`za`** or **`Z`** to collapse large blocks.
* **Live Turn Reconciler**: Press **`d`** or click **`[Next Turn]`** to advance turns and watch cards mutate from `v1` → `v2`.

### Testing & Verification

Run the automated test suite covering all dual-pane motions, in-buffer searching, folding, task toggling, and external editor integration:

```bash
./scripts/test-vim-features.sh
# or run via go test
go test -v -run "TestVim" ./pkg/ui
```

### CLI Options

```bash
# (Optional) Explicit harness override
syl --harness agy        # Force Antigravity
syl --harness claude     # Force Claude Code
syl --harness pi         # Force Pi Agent
syl --harness heuristic  # Fast offline fallback (no AI calls)

# Point at a specific transcript file (persisted separately per file path)
syl --file /path/to/transcript.jsonl

# Attach to a specific session by ID or prefix (overrides auto-detection)
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
| `w` / `Ctrl+w` | Toggle dual-pane focus (**[CARDS]** vs **[PREVIEW]**) |
| `j` / `k` | Contextual: navigate turns in Cards pane; scroll buffer in Preview pane |
| `↑` / `↓` | Scroll preview content up / down |
| `PgUp` / `PgDn` | Scroll preview page up / down |
| `Ctrl+e` / `Ctrl+y` | Vim 1-line nudge down / up |
| `Ctrl+d` / `Ctrl+u` | Half-page scroll down / up |
| `Ctrl+f` / `Ctrl+b` | Full-page scroll down / up |
| `{` / `}` | Paragraph scroll jump (5 lines up / down) |
| `gg` / `G` | Jump to top / bottom of preview buffer |
| `n` / `N` | Jump to next / previous search match in preview buffer |
| `v` / `E` | Open active card in `$EDITOR` (`nvim`/`vim`), reloading changes on exit |
| `za` / `Z` | Fold / unfold fenced code blocks to reduce clutter |
| `␣` (`Space`) | Toggle next markdown checklist item (`- [ ]` → `- [x]`) |
| `←` / `→` | Switch to next / previous card |
| `[` / `]` | Switch to previous / next card |
| `Tab` / `Enter` | Toggle focus view (hide/show turns column) |
| `b` | Surgical block & step picker (yank `1`–`9`, run `s`, open URL/file `o`) |
| `u` | Open card URL directly in browser |
| `/` | Quick search & filter cards interactively |
| `S` | Switch active conversation session without quitting |
| `R` | Retry failed card capture (later turns wait until it succeeds) |
| `c` | Copy code snippet(s) from card to clipboard |
| `C` / `y` | Copy full card markdown to clipboard |
| `r` | Toggle raw text mode (clean for mouse selection) |
| `p` | Pin / unpin card |
| `x` / `d` | Drop a card from the reference pane |
| `U` | Undo the most recent drop (up to 10, including after restart) |
| `e` | Export all session cards to `./syl-reference.md` |
| `h` | Toggle revision history for selected card |
| `s` | Send card reference or block to peer terminal (`cmux`, `tmux`, clipboard) |
| `?` / `F1` | Toggle help glossary modal |
| `q` / `Ctrl+C` | Quit (saves session snapshot; `q` stays open if saving fails, `Ctrl+C` force-quits) |

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
