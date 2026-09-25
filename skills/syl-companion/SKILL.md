---
name: syl-companion
description: "Companion skill for Syl (live reference pane). Formats actionable steps and code for Syl, handles [Ref: ...] card context from the user, and manages companion terminal setup."
risk: safe
source: local
---

# Syl Companion

This skill guides the AI assistant in collaborating with **Syl**, the live terminal reference pane for LLM conversations.

---

## 1. Recognizing Syl Injected References (`[Ref: ...]`)

When the user's message contains or starts with a reference marker:

```text
[Ref: <Card Title> (v<N>)]
<referenced code or content snippet>
```

This indicates the user pressed **`s` (Send to Peer)** or copied content directly from their adjacent Syl reference pane.

### Rules for handling `[Ref: ...]`:
1. **Acknowledge the specific reference**: Immediately ground your answer in the referenced card, command, or step.
2. **Zero friction**: Do not ask the user which command, card, or tool they are talking about—you already have the exact card title, version, and snippet.
3. **Targeted debugging & iteration**: If the user reports an error or asks for changes (e.g., *"this failed with exit code 1"*, *"change port to 8080"*), refer back to that specific step and provide the patched command or explanation.

---

## 2. Syl-Optimized Output Protocol

Syl's real-time parser automatically analyzes assistant turns and extracts:
- **Reference Cards** (Procedures, Commands, Decisions, Questions, Info)
- **Actionable Blocks** (`b` picker: Code snippets, numbered steps, URLs)
- **Definitive Titles** (summarized into 2–4 words)

To ensure maximum fidelity and 1-key execution in Syl, structure responses according to these formatting rules:

### A. Numbered Procedure Steps
* Use clean heading levels: `### Step <N>: <Action Description>`
* Keep step titles punchy and verb-first (e.g. `### Step 1: Provision Infrastructure Containers`).
* If a step has an inline command, enclose it in backticks: ``Run migration with `pnpm prisma migrate deploy` ``.

### B. Isolated Executable Code Blocks
* **One command per block**: Never bundle independent sequential steps into one monolithic code block. Put each distinct command in its own fenced block so Syl's block picker can assign individual shortcut keys (`1`, `2`, `3`).
* **Explicit language tags**: Always specify language tags (````bash````, ````sql````, ````yaml````, ````json````, ````python````) so Syl can display colored badges (`[BASH]`, `[SQL]`).
* **Clean runnable text**: Keep explanations, caveats, and markdown formatting *outside* the code block. The code block should contain only runnable commands or clean config.

### C. URLs and Web Verification
* Put documentation and dashboard links on their own lines or in clear markdown links.
* Syl's URL launcher (`u`) automatically extracts these for direct 1-key browser opening (`open <url>`).

### D. Multi-Turn Card Updates
* When amending an earlier card (e.g., user asks to change a database port or fix a failed step), preserve the overall step structure so Syl's reconciler can seamlessly update the existing card into version `v2` or `v3` rather than creating duplicate cards.

---

## 3. Companion Terminal Setup & Orchestration

When the user asks how to launch or set up Syl alongside their assistant:

### Inside `cmux`:
```bash
# Split the current workspace and launch Syl in an adjacent surface
cmux new-surface --command "syl"
```

### Inside `tmux`:
```bash
# Split the current window vertically (or horizontally) with Syl taking 35%
tmux split-window -h -p 35 "syl"
```

### Standalone Terminal (Split pane in iTerm2 / Ghostty / Warp):
```bash
# Run in the adjacent split pane (auto-detects Antigravity/Claude Code):
syl
```

---

## 4. Key Quick Reference for Syl Users

If the user asks about Syl keyboard shortcuts while chatting:
* **`b`**: Open **Actionable Blocks Picker** &rarr; tap `1`–`9` to yank single steps, `s` to send to peer, `o` to open URL.
* **`u`**: Instant **URL Launcher** (opens links in default browser).
* **`/`**: Live **Fuzzy Search** across all conversation turns.
* **`S`** (`Shift+S`): **Session Switcher** to jump between conversations without quitting.
* **`Tab` / `Enter`**: Toggle **Focus View** (hide/show turns column).
* **`c`**: Copy extracted code blocks.
* **`C` / `y`**: Copy full markdown card.
* **`s`**: Send card context into chat surface (`[Ref: ...]`).
* **`p`**: Pin card to prevent deletion.
* **`x`**: Drop / delete card.
* **`?`**: Help glossary modal.
