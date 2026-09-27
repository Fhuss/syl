---
name: syl-companion
description: "Companion skill for Syl (live reference pane). Automatically initializes Syl in an adjacent split pane to the right, formats actionable steps/code/files for Syl, handles [Ref: ...] card context, and manages companion terminal setups across cmux, tmux, iTerm2, and macOS Terminal."
risk: safe
source: local
---

# Syl Companion

This skill guides the AI assistant in collaborating symbiotically with **Syl**, the live terminal reference pane for AI conversations.

---

## 1. Automated Initialization (Launch Syl to the Right)

When the user asks to start, initialize, or launch Syl (e.g. *"init syl"*, *"launch syl"*, *"open syl pane"*, or on session startup):

### One-Command Automated Setup:
Execute `syl launch` via `run_command` (or terminal):

```bash
# Check if Syl is already running or launch it in a right split:
syl launch

# Or check if Syl is already active:
syl launch --check
```

---

## 2. Recognizing Syl Injected References (`[Ref: ...]`)

When the user's message contains or starts with a reference marker:

```text
[Ref: <Card Title> (v<N>)]
<referenced code, file path, or content snippet>
```

This indicates the user pressed **`s` (Send to Peer)** or copied content directly from their adjacent Syl reference pane.

### Rules for handling `[Ref: ...]`:
1. **Acknowledge the specific reference**: Immediately ground your answer in the referenced card, command, file, or step.
2. **Zero friction**: Do not ask the user which command, card, or tool they are talking about—you already have the exact card title, version, and snippet.
3. **Targeted debugging & iteration**: If the user reports an error or asks for changes (e.g., *"this failed with exit code 1"*, *"change port to 8080"*), refer back to that specific step and provide the patched command or explanation.

---

## 3. Syl-Optimized Output Protocol

Syl's real-time parser automatically analyzes assistant turns and extracts:
- **Reference Cards** (Procedures, Commands, Decisions, Questions, Info)
- **Actionable Blocks** (`b` picker: Code snippets, numbered steps, URLs, and files)
- **Definitive Titles** (summarized into 2–4 words)

To ensure maximum fidelity and 1-key execution in Syl, structure responses according to these formatting rules:

### A. Numbered Procedure Steps
* Use clean heading levels: `### Step <N>: <Action Description>`
* Keep step titles punchy and verb-first (e.g. `### Step 1: Provision Infrastructure Containers`).
* If a step has an inline command, enclose it in backticks: ``Run migration with `pnpm prisma migrate deploy` ``.

### B. Isolated Executable Code Blocks
* **One command per block**: Never bundle independent sequential steps into one monolithic code block. Put each distinct command in its own fenced block so Syl's block picker can assign individual shortcut keys (`1`, `2`, `3`).
* **Explicit language tags**: Always specify language tags (`bash`, `sql`, `yaml`, `json`, `python`, `go`) so Syl can display colored badges (`[BASH]`, `[SQL]`, `[GO]`).
* **Clean runnable text**: Keep explanations, caveats, and markdown formatting *outside* the code block. The code block should contain only runnable commands or clean config.

### C. Workspace File Links & Paths
* Format workspace files as markdown file links or clear paths:
  * `[model.go](file:///Users/fabian/Dev/syl/pkg/ui/model.go)`
  * `~/.config/syl/config.yaml`
  * `/etc/nginx/nginx.conf`
* Syl's Surgical Block Picker (**`b`**) detects these as `[FILE]`, `[GO]`, `[YAML]`, etc. blocks.
* The user can press **`b`** &rarr; **`o`** to preview or open the file in their default editor/app, or press **`1`**–**`9`** to copy the path.

### D. URLs and Web Verification
* Put documentation and dashboard links on their own lines or in clear markdown links.
* Syl's URL launcher (**`u`**) and Block Picker (**`b`**) automatically extract these for direct 1-key browser opening (`open <url>`).

### E. Multi-Turn Card Updates
* When amending an earlier card (e.g., user asks to change a database port or fix a failed step), preserve the overall step structure so Syl's reconciler can seamlessly update the existing card into version `v2` or `v3` rather than creating duplicate cards.

---

## 4. Key Quick Reference for Syl Users

If the user asks about Syl keyboard shortcuts while chatting:
* **`w` / `Ctrl+w`**: Toggle pane focus between **[CARDS]** turns list and **[PREVIEW]** buffer.
* **`j` / `k`**: Contextual navigation (`j`/`k` navigates cards when in Cards pane, scrolls buffer when in Preview pane).
* **`Ctrl+e` / `Ctrl+y`**: Vim 1-line nudge down / up.
* **`Ctrl+d` / `Ctrl+u`**: Half-page scroll down / up.
* **`{` / `}`**: Paragraph scroll jump (5 lines up / down).
* **`gg` / `G`**: Jump to top / bottom of preview buffer.
* **`n` / `N`**: Jump to next / previous match in active card (from `/` search).
* **`v` / `E`**: Open active card in `$EDITOR` (`nvim`/`vim`), saving changes on exit.
* **`za` / `Z`**: Fold / unfold code blocks to minimize clutter.
* **`␣` (`Space`)**: Toggle interactive markdown checklist task (`- [ ]` → `- [x]`).
* **`b`**: Open **Actionable Blocks Picker** → tap `1`–`9` to yank single steps/files, `s` to send to peer, `o` to open URL in browser or preview file.
* **`u`**: Instant **URL Launcher** (opens links in default browser).
* **`/`**: Live **Fuzzy Search** across all conversation turns.
* **`S`** (`Shift+S`): **Session Switcher** to jump between conversations without quitting.
* **`Tab` / `Enter`**: Toggle **Focus View** (hide/show turns column).
* **`c`**: Copy extracted code blocks.
* **`C` / `y`**: Copy full markdown card.
* **`s`**: Send card context into chat surface (`[Ref: ...]`).
* **`p`**: Pin card to prevent deletion.
* **`x`**: Drop a card (not permanent deletion; last 10 drops are retained).
* **`U`**: Undo the most recent drop, including after a restart.
* **`?`**: Help glossary modal.

