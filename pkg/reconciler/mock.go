package reconciler

import (
	"context"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/watcher"
)

// MockReconciler delivers deterministic mutations for testing and demo runs.
type MockReconciler struct {
	turnIndex int
}

func NewMockReconciler() *MockReconciler {
	return &MockReconciler{turnIndex: 0}
}

func (m *MockReconciler) Reconcile(ctx context.Context, currentCards []*card.Card, turn *watcher.Turn) ([]Mutation, error) {
	m.turnIndex++

	switch m.turnIndex {
	case 1:
		// Turn 1: Welcome tour, tips & tricks, interactive checklist, code showcase, and vim navigation
		return []Mutation{
			{
				Op:     OpAdd,
				CardID: "proc-syl-welcome",
				Type:   card.TypeProcedure,
				Title:  "Welcome to Syl & Quickstart",
				Content: `# ⚡ Welcome to Syl
Live reference companion for AI coding assistants ([GitHub: Fhuss/syl](https://github.com/Fhuss/syl)).

Syl sits in an adjacent terminal split and extracts procedures, commands, decisions, and files from your AI conversation in real time.

### 💡 Pro-Tips & Hotkeys
* **` + "`w` / `Ctrl+w`" + `**: Toggle dual-pane focus between **[CARDS]** and **[PREVIEW]**
* **` + "`j` / `k`" + `**: Contextual navigation:
  - In **[CARDS]**: switch selected card
  - In **[PREVIEW]**: scroll markdown buffer line-by-line
* **` + "`b`" + `**: Open **Surgical Block Picker** to extract isolated commands, files, and URLs
* **` + "`u`" + `**: Open first URL directly in your browser (https://github.com/Fhuss/syl)
* **` + "`Tab`" + `**: Toggle full-width **Focus View** (hides turns column)
* **` + "`za` / `Z`" + `**: Fold and unfold code blocks
* **` + "`v` / `E`" + `**: Open active card in ` + "`$EDITOR` (`nvim`, `vim`, `nano`)" + `
* **` + "`?`" + `**: Open the interactive Help Glossary modal

👉 *Select Card #2 (press 'j') to try the interactive task checklist!*`,
				Reason: "Extracted Syl quickstart and tips from conversation",
			},
			{
				Op:     OpAdd,
				CardID: "proc-vim-tour",
				Type:   card.TypeProcedure,
				Title:  "Interactive Features Checklist",
				Content: `### 🎯 Try Syl Features Live!
Press **` + "`Space`" + `** while on this card to check off completed tasks. Pressing Space resets all once complete!

- [ ] 1. Press 'w' to toggle focus between [CARDS] and [PREVIEW]
- [ ] 2. Focus [PREVIEW] and press 'j' / 'k' to scroll line-by-line
- [ ] 3. Press 'za' or 'Z' to fold code blocks on Card #3
- [ ] 4. Press 'b' to open the Surgical Block Picker (yank with 1-9)
- [ ] 5. Press '/' and type 'syl', then use 'n' / 'N' to jump between matches
- [ ] 6. Press 'v' or 'E' to edit this card in $EDITOR (nvim/vim)
- [ ] 7. Press 'p' to pin a card to the top
- [ ] 8. Press 'Tab' to toggle full-width Focus View
- [ ] 9. Visit https://github.com/Fhuss/syl and star the repository!`,
				Reason: "Generated interactive feature verification checklist",
			},
			{
				Op:     OpAdd,
				CardID: "proc-code-showcase",
				Type:   card.TypeProcedure,
				Title:  "Code Folding & Block Picker",
				Content: `### 📦 Code Blocks & File References
Test code folding with **` + "`za`" + `** or **` + "`Z`" + `** to collapse large blocks into clean 1-line summaries.
Press **` + "`b`" + `** to open the Surgical Block Picker and press **` + "`o`" + `** to open files!

#### 1. Configuration (` + "`~/.config/syl/config.yaml`" + `)
` + "```yaml\nllm:\n  provider: gemini\n  model: gemini-2.0-flash\nsubagent:\n  harness: auto\n  disabled: false\nwatcher:\n  debounce_ms: 150\n```" + `

#### 2. Launching Syl alongside your assistant
` + "```bash\n# Launch Syl in a right split with Ghostty cmux\ncmux new-split right --focus false && cmux send-keys \"syl\" Enter\n\n# Or launch with tmux\ntmux split-window -h -p 38 \"syl\"\n```" + `

#### 3. Core Source Files
* ` + "`cmd/syl/main.go`" + ` - CLI entrypoint and mode dispatcher
* ` + "`pkg/ui/model.go`" + ` - Bubble Tea dual-pane TUI model
* ` + "`pkg/card/block.go`" + ` - Surgical block and checklist parser
* ` + "`skills/syl-companion/SKILL.md`" + ` - Agent symbiotic companion skill`,
				Reason: "Showcase configuration and code blocks",
			},
			{
				Op:     OpAdd,
				CardID: "ref-vim-motions",
				Type:   card.TypeProcedure,
				Title:  "Vim Motions & Cheatsheet",
				Content: `### 🧭 Deep-Buffer Vim Navigation
For power users who prefer keyboard-driven navigation without touching the mouse.

#### Single-Line Motions
* ` + "`Ctrl+e`" + ` - Nudge viewport 1 line down
* ` + "`Ctrl+y`" + ` - Nudge viewport 1 line up
* ` + "`j`" + ` - Scroll down 1 line (when in [PREVIEW] pane)
* ` + "`k`" + ` - Scroll up 1 line (when in [PREVIEW] pane)

#### Jumping Motions
* ` + "`{`" + ` - Jump 5 lines up (paragraph jump)
* ` + "`}`" + ` - Jump 5 lines down (paragraph jump)
* ` + "`Ctrl+u`" + ` - Jump half-page up
* ` + "`Ctrl+d`" + ` - Jump half-page down
* ` + "`Ctrl+b`" + ` - Jump full page up (or PgUp)
* ` + "`Ctrl+f`" + ` - Jump full page down (or PgDn)
* ` + "`gg`" + ` - Jump instantly to top of buffer
* ` + "`G`" + ` - Jump instantly to bottom of buffer

#### In-Card Search Matches
1. Press ` + "`/`" + ` and type a search pattern (e.g. ` + "`jump`" + ` or ` + "`buffer`" + `)
2. Press ` + "`Enter`" + ` to commit the search
3. Press ` + "`n`" + ` to jump to the **next** match in this preview buffer
4. Press ` + "`N`" + ` to jump to the **previous** match in this preview buffer

#### External Editor Integration
* Press ` + "`v` or `E`" + ` to suspend Syl and open the selected card in ` + "`$EDITOR`" + `.
* Any edits you write and save in Vim/Neovim are instantly re-read into Syl!

Visit the official repository at https://github.com/Fhuss/syl for updates.`,
				Reason: "Extracted Vim motion navigation reference guide",
			},
		}, nil

	case 2:
		// Turn 2: Simulate live card mutation (v1 -> v2)
		return []Mutation{
			{
				Op:     OpUpdate,
				CardID: "proc-syl-welcome",
				Type:   card.TypeProcedure,
				Title:  "Welcome to Syl & Quickstart (Updated)",
				Content: `# ⚡ Welcome to Syl (v2)
Live reference companion for AI coding assistants ([GitHub: Fhuss/syl](https://github.com/Fhuss/syl)).

Syl sits in an adjacent terminal split and extracts procedures, commands, decisions, and files from your AI conversation in real time.

### 💡 Pro-Tips & Hotkeys
* **` + "`w` / `Ctrl+w`" + `**: Toggle dual-pane focus between **[CARDS]** and **[PREVIEW]**
* **` + "`j` / `k`" + `**: Contextual navigation (cards list vs preview scroll)
* **` + "`b`" + `**: Open **Surgical Block Picker** to extract isolated commands, files, and URLs
* **` + "`u`" + `**: Open first URL directly in your browser (https://github.com/Fhuss/syl)
* **` + "`za` / `Z`" + `**: Fold and unfold code blocks
* **` + "`v` / `E`" + `**: Open active card in ` + "`$EDITOR`" + `
* **` + "`h`" + `**: Inspect card revision history (see earlier versions!)

### 🤖 Symbiotic Agent Companion Skill
Install the Syl companion skill into your assistant environment:
` + "```bash\ncp -r skills/syl-companion ~/.gemini/config/plugins/awesome-skills-plugin/skills/\n```" + `
When active, your assistant automatically generates discrete runnable blocks and recognizes ` + "`[Ref: ...]`" + ` context when you press ` + "`s`" + `!`,
				Reason: "Demonstrated live card update and version history tracking (v1 -> v2)",
			},
		}, nil

	case 3:
		// Turn 3: Record an architectural decision card and update code showcase
		return []Mutation{
			{
				Op:     OpUpdate,
				CardID: "proc-code-showcase",
				Type:   card.TypeProcedure,
				Title:  "Code Folding & Block Picker (v2)",
				Content: `### 📦 Code Blocks & File References
Test code folding with **` + "`za`" + `** or **` + "`Z`" + `** to collapse large blocks into clean 1-line summaries.
Press **` + "`b`" + `** to open the Surgical Block Picker and press **` + "`o`" + `** to open files!

#### 1. Zero-Config Multiplexer Discovery
` + "```bash\n# Detect active peer pane\ncmux identify || tmux list-panes\n```" + `

#### 2. Official GitHub Repository
* ` + "`https://github.com/Fhuss/syl`" + `
* Star the repo or report feedback on the issues page!`,
				Reason: "Updated code blocks with multiplexer discovery commands",
			},
			{
				Op:     OpAdd,
				CardID: "dec-multiplexer-support",
				Type:   card.TypeDecision,
				Title:  "Multiplexer Architecture Decision",
				Content: `# 🏛️ Architecture Decision: Zero-Config Multiplexing
**Repository**: [github.com/Fhuss/syl](https://github.com/Fhuss/syl)
**Status**: Accepted

### Context
Developers use a wide variety of terminal environments: Ghostty (` + "`cmux`" + `), ` + "`tmux`" + `, iTerm2, and native terminal emulators.

### Decision
Syl natively discovers peer panes via ` + "`cmux identify`" + ` and ` + "`tmux list-panes`" + `:
1. **Ghostty**: Interacts via ` + "`cmux send-keys --surface <id>`" + `
2. **tmux**: Interacts via ` + "`tmux send-keys -t <pane>`" + `
3. **Fallback**: Sends to system clipboard with a helpful toast

Press **` + "`s`" + `** to send any card reference or command block directly to your active agent terminal!`,
				Reason: "Recorded multiplexer architecture decision",
			},
		}, nil

	default:
		// Conversational filler -> NOOP
		return []Mutation{
			{
				Op:     OpNoop,
				Reason: "Conversational discussion without new or revised procedures",
			},
		}, nil
	}
}
