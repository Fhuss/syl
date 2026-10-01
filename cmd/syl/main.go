package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/config"
	"github.com/Fhuss/syl/pkg/harness"
	"github.com/Fhuss/syl/pkg/multiplexer"
	"github.com/Fhuss/syl/pkg/reconciler"
	"github.com/Fhuss/syl/pkg/skill"
	"github.com/Fhuss/syl/pkg/ui"
	"github.com/Fhuss/syl/pkg/watcher"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "0.1.0-dev"

func formatTimeAgo(t time.Time) string {
	return watcher.FormatTimeAgo(t)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `syl — A live reference pane for AI conversations (%s)

Usage:
  syl [command|flags]

Commands:
  new / clear        Clear reference pane and wait for new conversation
  demo               Run an interactive simulation demonstrating live card mutation
  sessions           List active and recent conversation sessions to choose from
  install-skill      Install companion skill into Pi Agent, Claude Code, or Antigravity
  launch / start     Launch Syl in an adjacent right-hand split (cmux/tmux/terminal)
  init-config        Generate a default configuration file in ~/.config/syl/config.yaml
  version            Print version information

Flags:
  -n, --new            Start a new session / clear pane and wait for conversation
  -s, --session string Attach to a specific conversation session by ID or prefix
  -f, --file string    Path to transcript.jsonl (auto-detected if omitted)
  -c, --config string  Path to config file (default ~/.config/syl/config.yaml)
  -p, --provider string LLM provider override (mock, openai, gemini)
  --harness string     Subagent harness override (auto, agy, claude, pi, ollama, heuristic)
  -h, --help           Show this help message

Keys in TUI:
  [↑/↓]        Scroll preview content (PgUp/PgDn, Ctrl+u/d)
  [j/k, ←/→]   Navigate reference turns / cards ([ and ] also work)
  [Tab/Enter]  Toggle focus view (hide/show turns column)
  [b]          Surgical block & step picker (yank 1-9, run, open URL)
  [u]          Open card URL directly in browser
  [/]          Quick search & filter cards interactively
  [S]          Switch active conversation session (Antigravity/Claude/Pi)
  [N, Ctrl+n]  Clear pane / start new session (auto-attaches on prompt)
  [R]          Retry failed card capture before processing later turns
  [c]          Copy card code snippet(s) to system clipboard
  [C / y]      Copy full card markdown content to clipboard
  [r]          Toggle raw text mode (clean for mouse selection)
  [p]          Pin / unpin card
  [x / d]      Drop a card from the reference pane
  [U]          Undo the most recent drop (up to 10, even after restart)
  [T]          Browse Trash; Enter restores, x then y purges selected drop
  [e]          Export session cards to markdown (./syl-reference.md)
  [h]          Preview prior content; Left/Right and Enter restore as a new version
  [s]          Send card context to chat surface (cmux / clipboard)
  [?]          Toggle help glossary modal
  [Space]      Advance turn (in demo mode)
  [q, Ctrl+C]  Quit
`, version)
	}

	var (
		sessionFlag  string
		fileFlag     string
		configFlag   string
		providerFlag string
		harnessFlag  string
		showVersion  bool
		newFlag      bool
	)

	flag.StringVar(&sessionFlag, "session", "", "Session ID or prefix to watch")
	flag.StringVar(&sessionFlag, "s", "", "Session ID or prefix to watch (shorthand)")
	flag.StringVar(&fileFlag, "file", "", "Path to transcript.jsonl file")
	flag.StringVar(&fileFlag, "f", "", "Path to transcript.jsonl file (shorthand)")
	flag.StringVar(&configFlag, "config", "", "Path to config file")
	flag.StringVar(&configFlag, "c", "", "Path to config file (shorthand)")
	flag.StringVar(&providerFlag, "provider", "", "LLM provider override (mock, openai, gemini, anthropic, ollama)")
	flag.StringVar(&providerFlag, "p", "", "LLM provider override (shorthand)")
	flag.StringVar(&harnessFlag, "harness", "", "Subagent harness override (auto, agy, claude, pi, ollama, heuristic)")
	flag.BoolVar(&newFlag, "new", false, "Start a new session / clear pane and wait for conversation")
	flag.BoolVar(&newFlag, "n", false, "Start a new session / clear pane (shorthand)")
	flag.BoolVar(&newFlag, "clear", false, "Clear pane and wait for conversation")
	flag.BoolVar(&showVersion, "version", false, "Print version")
	flag.Parse()

	if showVersion {
		fmt.Printf("syl version %s\n", version)
		os.Exit(0)
	}

	args := flag.Args()
	subcommand := ""
	if len(args) > 0 {
		subcommand = args[0]
	}
	isNew := newFlag || subcommand == "new" || subcommand == "clear"

	if subcommand == "version" {
		fmt.Printf("syl version %s\n", version)
		os.Exit(0)
	}

	if subcommand == "sessions" {
		sessions, err := watcher.ListSessions(15)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No active or past sessions found.")
			os.Exit(0)
		}
		fmt.Printf("Recent Conversation Sessions:\n\n")
		for _, s := range sessions {
			agentTag := fmt.Sprintf("[%s]", s.Agent)
			if s.Agent == "" {
				agentTag = "[agy]"
			}
			fmt.Printf("  %-14s  [%s]  %-48s  (%s)\n", agentTag, s.ShortID, s.Title, formatTimeAgo(s.ModTime))
		}
		fmt.Printf("\nRun 'syl -s <id>' to watch a specific session.\n")
		os.Exit(0)
	}

	if subcommand == "init-config" {
		dir, err := config.ConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config dir: %v\n", err)
			os.Exit(1)
		}
		target := filepath.Join(dir, "config.yaml")
		if err := config.SaveDefaultConfig(target); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Default configuration written to %s\n", target)
		os.Exit(0)
	}

	if subcommand == "install-skill" {
		all := false
		specific := ""
		for _, a := range args[1:] {
			if a == "--all" || a == "-a" {
				all = true
			} else if a == "--pi" {
				specific = "Pi Agent (global)"
			} else if a == "--claude" {
				specific = "Claude Code"
			} else if a == "--gemini" || a == "--antigravity" {
				specific = "Antigravity Plugin"
			}
		}
		installed, err := skill.InstallSkill(all, specific)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error installing skill: %v\n", err)
			os.Exit(1)
		}
		if len(installed) == 0 {
			fmt.Println("No active agent skill directories found. Run 'syl install-skill --all' to force install into all standard locations.")
			os.Exit(0)
		}
		fmt.Printf("✓ Successfully installed Syl Companion Skill into:\n")
		for _, p := range installed {
			fmt.Printf("  • %s\n", p)
		}
		fmt.Printf("\nYour agent (Pi / Claude / Antigravity) will now recognize 'start syl' and [Ref: ...] references!\n")
		os.Exit(0)
	}

	if subcommand == "launch" || subcommand == "start" {
		force := false
		check := false
		for _, a := range args[1:] {
			if a == "--force" || a == "-f" {
				force = true
			} else if a == "--check" || a == "-c" {
				check = true
			}
		}
		if err := skill.Launch(force, check); err != nil {
			fmt.Fprintf(os.Stderr, "Launch error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	isDemo := subcommand == "demo"

	cfg, err := config.LoadConfig(configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning loading config: %v (using defaults)\n", err)
		cfg = config.DefaultConfig()
	}

	if providerFlag != "" {
		cfg.LLM.Provider = providerFlag
	}

	store := card.NewStore()

	// Choose reconciler
	var rec reconciler.Reconciler
	if isDemo {
		rec = reconciler.NewMockReconciler()
	} else if (cfg.LLM.APIKey != "" || cfg.LLM.Provider == "ollama") && cfg.LLM.Provider != "mock" {
		rec = reconciler.NewLLMReconciler(reconciler.LLMConfig{
			Provider: cfg.LLM.Provider,
			Endpoint: cfg.LLM.Endpoint,
			APIKey:   cfg.LLM.APIKey,
			Model:    cfg.LLM.Model,
		})
	} else {
		rec = reconciler.NewHeuristicReconciler()
	}

	transcriptPath := ""
	sessionLabel := ""
	if isDemo {
		sessionLabel = "demo"
	}
	var detectedAgent string
	var searchCWD string
	if !isDemo {
		// Detect peer CWD from adjacent terminal (cmux/tmux) or fallback to current dir
		if peerCWD := multiplexer.DetectPeerCWD(); peerCWD != "" {
			searchCWD = peerCWD
		} else {
			searchCWD, _ = os.Getwd()
		}

		// Detect peer agent from adjacent terminal (cmux/tmux)
		if peer := multiplexer.DetectPeerAgent(); peer != "" {
			detectedAgent = peer
		}

		if isNew {
			// Explicitly starting a new session: do not auto-attach to an existing transcript
			transcriptPath = ""
			sessionLabel = ""
		} else {
			transcriptPath = fileFlag
			if transcriptPath != "" {
				detectedAgent = watcher.DetectAgentFromPath(transcriptPath)
			}

			if transcriptPath == "" && sessionFlag != "" {
				session, err := watcher.FindSessionByID(sessionFlag)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Session error: %v\n", err)
					os.Exit(1)
				}
				transcriptPath = session.Path
				sessionLabel = session.ShortID
				detectedAgent = session.Agent
			}

			if transcriptPath == "" {
				transcriptPath = cfg.Watcher.TranscriptPath
				if transcriptPath != "" {
					detectedAgent = watcher.DetectAgentFromPath(transcriptPath)
				}
			}

			if transcriptPath == "" {
				session, err := watcher.SelectAutoSession(searchCWD, multiplexer.PeerSessionFile())
				if err == nil && session != nil {
					transcriptPath = session.Path
					sessionLabel = session.ShortID
					detectedAgent = session.Agent
				}
			}

			if transcriptPath != "" && sessionLabel == "" {
				var err error
				sessionLabel, err = fileSessionLabel(transcriptPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid transcript path: %v\n", err)
					os.Exit(1)
				}
			}
		}

		// Fall back to checking adjacent terminal in cmux or tmux if agent is still unknown
		if detectedAgent == "" || detectedAgent == "unknown" || detectedAgent == "file" {
			if peer := multiplexer.DetectPeerAgent(); peer != "" {
				detectedAgent = peer
			}
		}

	}

	// Choose subagent harness
	harnessChoice := cfg.Subagent.Harness
	if harnessFlag != "" {
		// Explicit CLI flag always wins
		harnessChoice = harnessFlag
	} else if harnessChoice == "" || harnessChoice == "auto" {
		// Auto-detect based on session agent or multiplexer peer
		switch strings.ToLower(detectedAgent) {
		case "antigravity", "agy":
			harnessChoice = "agy"
		case "claude":
			harnessChoice = "claude"
		case "pi":
			harnessChoice = "pi"
		default:
			harnessChoice = "auto"
		}
	}

	var subagentHarness harness.Harness
	if !cfg.Subagent.Disabled {
		subagentHarness = harness.NewAutoHarness(harnessChoice, cfg.Subagent.Model)
	}

	model := ui.NewModel(store, rec, subagentHarness, nil, nil, isDemo, sessionLabel)
	if !isDemo {
		if transcriptPath != "" {
			model.SetWaitingForSession(false, searchCWD, detectedAgent)
			model.StartWatching(transcriptPath)
		} else {
			model.SetWaitingForSession(true, searchCWD, detectedAgent)
		}
	}
	defer model.StopWatching()
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running syl: %v\n", err)
		os.Exit(1)
	}
}
