// Command agys provides a Bubble Tea TUI for browsing, searching, and resuming Google Antigravity CLI sessions.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev"

type cliOptions struct {
	dataDir      string
	stateFile    string
	searchQuery  string
	showArchived bool
	exitOnResume bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "agys: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, done, err := parseCLI(args, stdout, stderr)
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	cfg, err := config.DefaultWithOverrides(opts.dataDir, opts.stateFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	model := tui.NewWithOptions(cfg, tui.Options{
		ShowArchived: opts.showArchived,
		InitialQuery: opts.searchQuery,
		ExitOnResume: opts.exitOnResume,
	})

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithReportFocus())
	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("run tui: %w", err)
	}

	if m, ok := finalModel.(tui.Model); ok {
		if target, hasTarget := m.PendingResume(); hasTarget {
			return session.ExecResume(target.ID, target.WorkDir)
		}
	}

	return nil
}

func parseCLI(args []string, stdout, stderr io.Writer) (cliOptions, bool, error) {
	var opts cliOptions
	var showHelp bool
	var showVersion bool

	fs := flag.NewFlagSet("agys", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.StringVar(&opts.dataDir, "dir", "", "Antigravity CLI data directory")
	fs.StringVar(&opts.dataDir, "d", "", "Antigravity CLI data directory")
	fs.StringVar(&opts.stateFile, "state", "", "Agys state file path")
	fs.StringVar(&opts.searchQuery, "search", "", "Initial search query")
	fs.StringVar(&opts.searchQuery, "s", "", "Initial search query")
	fs.BoolVar(&opts.showArchived, "archived", false, "Show archived sessions on startup")
	fs.BoolVar(&opts.showArchived, "a", false, "Show archived sessions on startup")
	fs.BoolVar(&opts.exitOnResume, "exit-on-resume", false, "Exit agys and replace with agy on resume")
	fs.BoolVar(&opts.exitOnResume, "e", false, "Exit agys and replace with agy on resume")
	fs.BoolVar(&showVersion, "version", false, "Show version information")
	fs.BoolVar(&showVersion, "v", false, "Show version information")
	fs.BoolVar(&showHelp, "help", false, "Show help message")
	fs.BoolVar(&showHelp, "h", false, "Show help message")

	fs.Usage = func() {
		printUsage(stderr)
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cliOptions{}, true, nil
		}
		return cliOptions{}, false, err
	}

	if showHelp {
		printUsage(stdout)
		return cliOptions{}, true, nil
	}
	if showVersion {
		fmt.Fprintln(stdout, versionString())
		return cliOptions{}, true, nil
	}
	if fs.NArg() > 0 {
		printUsage(stderr)
		return cliOptions{}, false, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return opts, false, nil
}

func versionString() string {
	return fmt.Sprintf("agys %s (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `agys - Terminal TUI for browsing, searching, and resuming Google Antigravity CLI sessions

Usage:
  agys [flags]

Flags:
  -d, --dir <path>         Antigravity CLI data directory (default: ~/.gemini/antigravity-cli)
      --state <path>       Agys user state file path (default: ~/.config/agys/state.json)
  -s, --search <query>     Start with an initial search filter
  -a, --archived           Include archived sessions on startup
  -e, --exit-on-resume     Exit agys and hand terminal to agy on Enter
  -v, --version            Print version information and exit
  -h, --help               Show this help message and exit

TUI Keybindings (press '?' inside agys for the full overlay):
  ↑/k, ↓/j    Navigate or scroll      /     Live search
  l/→, h/←    Expand subsessions      r     Rename session (saved in state.json)
  Tab         Focus preview pane      p     Pin / unpin session
  a / A       Archive / show all      Enter Resume session in its directory
  ?           Which-key overlay       q     Quit
`)
}
