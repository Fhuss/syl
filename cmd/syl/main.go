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

func findLatestAntigravityTranscript() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	pattern := filepath.Join(home, ".gemini", "antigravity-cli", "brain", "*", ".system_generated", "logs", "transcript.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("no active Antigravity session transcript found")
	}

	var latestFile string
	var latestTime time.Time

	for _, m := range matches {
		info, err := os.Stat(m)
		if err == nil && info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestFile = m
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("no valid transcript file found")
	}

	return latestFile, nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `syl — A live reference pane for AI conversations (%s)

Usage:
  syl [command|flags]

Commands:
  demo               Run an interactive simulation demonstrating live card mutation
  init-config        Generate a default configuration file in ~/.config/syl/config.yaml
  version            Print version information

Flags:
  -f, --file string  Path to transcript.jsonl (auto-detected if omitted)
  -c, --config string Path to config file (default ~/.config/syl/config.yaml)
  -p, --provider string LLM provider override (mock, openai, gemini)
  -h, --help         Show this help message

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
		fileFlag     string
		configFlag   string
		providerFlag string
		showVersion  bool
	)

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

	if !isDemo {
		transcriptPath := fileFlag
		if transcriptPath == "" {
			transcriptPath = cfg.Watcher.TranscriptPath
		}
		if transcriptPath == "" {
			autoPath, err := findLatestAntigravityTranscript()
			if err == nil {
				transcriptPath = autoPath
				fmt.Printf("Auto-discovered active transcript: %s\n", transcriptPath)
			}
		}

		if transcriptPath != "" {
			turnCh = make(chan *watcher.Turn, 10)
			fw := watcher.NewFileWatcher(transcriptPath)
			go func() {
				if err := fw.Watch(ctx, turnCh); err != nil {
					fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
				}
			}()
		}
	}

	model := ui.NewModel(store, rec, turnCh, isDemo)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running syl: %v\n", err)
		os.Exit(1)
	}
}
