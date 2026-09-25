# syl

> **A live reference pane for AI conversations.**
> Named after Sylphrena (*The Stormlight Archive*), the loyal companion who adapts its shape to whatever tool you need.

`syl` sits quietly beside your AI chat in a terminal multiplexer (cmux, tmux, or Zellij) and automatically extracts and maintains the useful reference material from each conversation turn: **procedures, commands, architecture decisions, and open questions** — persisted across restarts.

---

## The Problem

An assistant gives you a useful 6-step AWS deployment procedure. You ask 4 follow-up questions, debug an authentication error, and clarify your target region. The procedure has now scrolled far off-screen.

If a later answer corrected step 3 or changed a configuration parameter, you must remember which version in the transcript history is safe to run. Conversational scrollback causes **procedural decay**.

## How `syl` Works

1. **Passive turn observation** — Watches completed turns from AI sessions (Antigravity, Claude Code, Pi, or custom streams) via a tail-following file watcher. No _"jot this down"_ command required.
2. **Live in-place mutation** — When a follow-up answer fixes an error or changes a configuration, `syl` patches the existing card in-place rather than adding a duplicate. Disproven steps are marked invalidated.
3. **AI-summarized titles** — A configurable harness (`agy`, `claude`, `heuristic`, …) generates a short, meaningful topic title for each card instead of showing a raw prompt truncation.
4. **Session persistence** — Cards, titles, pinned state, and revision history are saved to `~/.config/syl/sessions/<session-id>.json` on every change and on quit. On restart, the full reference pane is restored instantly with no replay needed.
5. **Ergonomic terminal pane** — Split view with a scrollable turn list on the left and a Glamour-rendered markdown preview on the right. Tab to focus on just the preview.

---

## Architecture

```
┌──────────────────────┐
│   File Watcher       │  Tails transcript.jsonl (Antigravity / Claude Code / custom)
└──────────┬───────────┘
           │ turn events (user prompt + assistant response)
           ▼
┌──────────────────────┐
│   Reconciler         │  LLM or heuristic: emits ADD / UPDATE / INVALIDATE mutations
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
│   Harness            │  Calls agy / claude / heuristic to generate succinct titles
│                      │  Title cache → ~/.config/syl/titles.json
└──────────┬───────────┘
           │ TitleSummarizedMsg
           ▼
┌──────────────────────┐
│   TUI Engine         │  Bubbletea + Glamour markdown + Lipgloss
│                      │  Split view | Focus view | Raw mode | Help modal
└──────────────────────┘
```

---

## Quickstart

### Demo mode (no AI session required)

```bash
syl demo
```

Replays a simulated multi-turn scenario to demonstrate live card patching and in-place mutation. Press `Space` to advance turns.

### Attach to an active session

```bash
# Auto-detect the most recent Antigravity transcript
syl

# Specify a harness for AI title summarization
syl --harness agy        # Antigravity
syl --harness claude     # Claude Code
syl --harness heuristic  # Fast offline fallback (no AI calls)

# Point at a specific transcript file
syl --file /path/to/transcript.jsonl

# Attach to a specific session by ID or prefix
syl --session 40c1c89b

# List recent sessions to choose from
syl sessions
```

### Configuration

Generate a starter config:

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

Press `?` inside `syl` to open the interactive help glossary.

| Key | Action |
| :--- | :--- |
| `↑` / `↓` | Scroll preview content up / down |
| `PgUp` / `PgDn` | Scroll preview page up / down |
| `Ctrl+u` / `Ctrl+d` | Half-page scroll |
| `j` / `k` | Switch to next / previous card |
| `←` / `→` | Switch to next / previous card |
| `[` / `]` | Switch to previous / next card |
| `Tab` | Toggle focus view (hide/show turns column) |
| `c` | Copy code snippet(s) from card to clipboard |
| `C` | Copy full card markdown to clipboard |
| `r` | Toggle raw text mode (clean for mouse selection) |
| `p` | Pin / unpin card |
| `x` | Drop / delete card from reference pane |
| `e` | Export all cards to `./syl-reference.md` |
| `h` | Toggle revision history for selected card |
| `s` | Send card reference to peer terminal (cmux, tmux, or clipboard) |
| `?` / `F1` | Toggle help glossary modal |
| `Space` | Advance turn (demo mode only) |
| `q` / `Ctrl+C` | Quit (saves session snapshot to disk) |

---

## Session Persistence

`syl` automatically saves the full state of your reference pane to:

```
~/.config/syl/sessions/<session-id>.json
```

This includes all cards, their AI-generated titles, version history, and pinned/dropped status. On the next run against the same session, everything is restored immediately — no waiting for turn replay.

AI-generated titles are additionally cached in `~/.config/syl/titles.json` to avoid redundant summarization calls across restarts.

---

## CLI Reference

```
syl [command|flags]

Commands:
  demo           Interactive demo simulating live card mutation
  sessions       List recent conversation sessions across Antigravity, Claude Code, etc.
  init-config    Write default config to ~/.config/syl/config.yaml
  version        Print version

Flags:
  -s, --session string    Attach to session by ID or prefix
  -f, --file string       Path to transcript.jsonl
  -c, --config string     Path to config file
  -p, --provider string   LLM provider override (mock, openai, gemini, anthropic, ollama)
      --harness string    Harness override (auto, agy, claude, pi, ollama, heuristic)
  -h, --help              Show help
```
