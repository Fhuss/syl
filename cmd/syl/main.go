package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/config"
	"github.com/Fhuss/syl/pkg/reconciler"
	"github.com/Fhuss/syl/pkg/ui"
	"github.com/Fhuss/syl/pkg/watcher"
)

const version = "0.1.0"

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Format("Jan 02 15:04")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `syl — A live reference pane for AI conversations (%s)

Usage:
  syl [command|flags]

Commands:
  demo               Run an interactive simulation demonstrating live card mutation
  sessions           List active and recent conversation sessions to choose from
  init-config        Generate a default configuration file in ~/.config/syl/config.yaml
  version            Print version information

Flags:
  -s, --session string Attach to a specific conversation session by ID or prefix
  -f, --file string    Path to transcript.jsonl (auto-detected if omitted)
  -c, --config string  Path to config file (default ~/.config/syl/config.yaml)
  -p, --provider string LLM provider override (mock, openai, gemini)
  -h, --help           Show this help message

Keys in TUI:
  [j/k, ↑/↓]   Navigate reference cards
  [Enter]      Toggle full-width detail / split view
  [c / y]      Copy card code snippet to system clipboard
  [p]          Pin / unpin card
  [h]          Toggle version revision history
  [s]          Send card context to chat surface (cmux / clipboard)
  [Space/d]    Advance turn (in demo mode)
  [q, Ctrl+C]  Quit
`, version)
	}

	var (
		sessionFlag  string
		fileFlag     string
		configFlag   string
		providerFlag string
		showVersion  bool
	)

	flag.StringVar(&sessionFlag, "session", "", "Session ID or prefix to watch")
	flag.StringVar(&sessionFlag, "s", "", "Session ID or prefix to watch (shorthand)")
	flag.StringVar(&fileFlag, "file", "", "Path to transcript.jsonl file")
	flag.StringVar(&fileFlag, "f", "", "Path to transcript.jsonl file (shorthand)")
	flag.StringVar(&configFlag, "config", "", "Path to config file")
	flag.StringVar(&configFlag, "c", "", "Path to config file (shorthand)")
	flag.StringVar(&providerFlag, "provider", "", "LLM provider override (mock, openai, gemini)")
	flag.StringVar(&providerFlag, "p", "", "LLM provider override (shorthand)")
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
		fmt.Printf("Recent Antigravity Conversation Sessions:\n\n")
		for _, s := range sessions {
			fmt.Printf("  [%s]  %-52s  (%s)\n", s.ShortID, s.Title, formatTimeAgo(s.ModTime))
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
	} else if cfg.LLM.APIKey != "" && cfg.LLM.Provider != "mock" {
		rec = reconciler.NewLLMReconciler(reconciler.LLMConfig{
			Endpoint: cfg.LLM.Endpoint,
			APIKey:   cfg.LLM.APIKey,
			Model:    cfg.LLM.Model,
		})
	} else {
		rec = reconciler.NewHeuristicReconciler()
	}

	var turnCh chan *watcher.Turn
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var activityCh chan watcher.AgentActivity
	sessionLabel := "demo"
	if !isDemo {
		transcriptPath := fileFlag
		if transcriptPath == "" && sessionFlag != "" {
			session, err := watcher.FindSessionByID(sessionFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Session error: %v\n", err)
				os.Exit(1)
			}
			transcriptPath = session.Path
			sessionLabel = session.ShortID
		}

		if transcriptPath == "" {
			transcriptPath = cfg.Watcher.TranscriptPath
		}

		if transcriptPath == "" {
			sessions, err := watcher.ListSessions(1)
			if err == nil && len(sessions) > 0 {
				transcriptPath = sessions[0].Path
				sessionLabel = sessions[0].ShortID
			}
		}

		if transcriptPath != "" {
			turnCh = make(chan *watcher.Turn, 100)
			activityCh = make(chan watcher.AgentActivity, 20)
			fw := watcher.NewFileWatcher(transcriptPath)
			go func() {
				if err := fw.Watch(ctx, turnCh, activityCh); err != nil {
					fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
				}
			}()
		}
	}

	model := ui.NewModel(store, rec, turnCh, activityCh, isDemo, sessionLabel)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running syl: %v\n", err)
		os.Exit(1)
	}
}
