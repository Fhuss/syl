# syl

> **A live reference pane for AI conversations.**
> Named after Sylphrena (*The Stormlight Archive*), the loyal companion who adapts its shape to whatever tool you need.

`syl` sits quietly beside your AI chat in your terminal multiplexer (cmux, tmux, or Zellij) and automatically maintains the useful artifacts you need during a conversation: **procedures, commands, architecture decisions, and open questions**.

---

## The Problem

An assistant gives you a useful 6-step AWS deployment procedure. You ask 4 follow-up questions, debug an authentication error, and clarify your target region. The procedure has now scrolled far off-screen.

If a later answer corrected step 3 or changed a configuration parameter, you must remember which version in the transcript history is safe to run. Conversational scrollback causes **procedural decay**.

## How `syl` Works

1. **Passive Turn Observation**: Observes completed turns from AI sessions (Antigravity transcripts, Claude Code logs, or custom streams). No *"jot this down"* command required.
2. **Live In-Place Mutation**: When a follow-up answer fixes an error or changes a region, `syl` patches the existing procedure card rather than polluting your workspace with duplicate or outdated instructions. Disproven commands are marked invalidated.
3. **Ergonomic Terminal Pane**: Runs in an adjacent pane (Glance view for quick scanning, Working view for syntax-highlighted markdown steps).
4. **Instant Action**: Press `y` to yank a command to your clipboard, `p` to pin, or `s` to send context back into the conversation.

---

## Architecture Overview

```
┌──────────────────┐
│  Client Adapter  │  (e.g., Antigravity transcript.jsonl / Claude Code)
└────────┬─────────┘
         │ turn events (user + assistant)
         ▼
┌──────────────────┐
│   Reconciler     │  (Fast LLM: Gemini Flash / Claude Haiku / Local Qwen / Mock)
└────────┬─────────┘
         │ delta operations (ADD, UPDATE, INVALIDATE)
         ▼
┌──────────────────┐
│   Card Store     │  (In-memory versioned cards)
└────────┬─────────┘
         │ reactive updates
         ▼
┌──────────────────┐
│    TUI Engine    │  (Bubbletea + Glamour ANSI Markdown + Lipgloss)
└──────────────────┘
```

---

## Quickstart

### 1. Interactive Demo Mode (Zero Config)
To test the live mutation mechanics without an active AI session:

```bash
syl demo
```
This replays a 3-turn AWS deployment scenario to show live card patching in action.

### 2. Live Session Companion
Watch an Antigravity or custom transcript:

```bash
syl watch
# or watch a specific transcript file:
syl watch --file /path/to/transcript.jsonl
```

---

## Keyboard Shortcuts

| Key | Action |
| :--- | :--- |
| `j` / `k` / `↑` / `↓` | Navigate cards |
| `Enter` | Toggle full markdown view / collapse |
| `y` | Yank code block / snippet to clipboard |
| `p` | Pin card (prevents auto-eviction) |
| `h` | View revision history & diff reasons |
| `s` | Send card snippet back to chat (cmux / tmux / clipboard) |
| `q` / `Esc` | Quit |

---

## Configuration

Configuration is stored at `~/.config/syl/config.yaml` or `.syl.yaml`:

```yaml
llm:
  provider: gemini # gemini | anthropic | openai | ollama | mock
  model: gemini-2.5-flash
  api_key: ${GEMINI_API_KEY}

watcher:
  source: antigravity # antigravity | claude_code | generic_jsonl
  poll_interval_ms: 300

ui:
  theme: dark
  glance_max_items: 8
```
