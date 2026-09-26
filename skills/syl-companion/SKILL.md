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
Execute the companion init script via `run_command`:

```bash
# Check if Syl is already running or launch it to the right:
./skills/syl-companion/scripts/init-syl.sh
```

### What this script does automatically:
1. **Detects active multiplexer**:
   * **`cmux` (Ghostty)**: Runs `cmux new-split right --focus false` and starts `syl` in the new right-hand surface.
   * **`tmux`**: Runs `tmux split-window -h -p 35 "syl"`.
   * **`iTerm2`**: Splits current session vertically to the right via AppleScript.
   * **macOS Terminal.app**: Opens a companion Terminal window placed on the right half of the display.
2. **Prevents duplicate panes**: Checks if Syl is already running in an active panel/pane and warns rather than cluttering your workspace with duplicate splits (override with `--force`).
3. **Auto-build fallback**: If `syl` is not in `$PATH`, automatically builds it from local source into `~/.local/bin/syl`.

### Checking Status:
```bash
./skills/syl-companion/scripts/init-syl.sh --check
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
* **`b`**: Open **Actionable Blocks Picker** &rarr; tap `1`–`9` to yank single steps/files, `s` to send to peer, `o` to open URL in browser or preview file.
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
