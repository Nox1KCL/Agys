// Command agys provides a Bubble Tea TUI for browsing, searching, and resuming Google Antigravity CLI sessions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

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
	pluginState  bool
	refreshUsage bool
	action       string
	targetID     string
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

	if opts.pluginState {
		return runPluginState(cfg, opts.refreshUsage, stdout)
	}
	if opts.action != "" {
		return runPluginAction(cfg, opts.action, opts.targetID, stdout)
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
	fs.BoolVar(&opts.pluginState, "plugin-state", false, "Output headless JSON snapshot for shell plugins")
	fs.BoolVar(&opts.refreshUsage, "refresh-usage", false, "Force quota refresh during --plugin-state")
	fs.StringVar(&opts.action, "action", "", "Run headless session action (pin, unpin, archive, unarchive, trash, restore, purge)")
	fs.StringVar(&opts.targetID, "id", "", "Session UUID or trash entry name for --action")
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

	opts.action = strings.TrimSpace(opts.action)
	opts.targetID = strings.TrimSpace(opts.targetID)
	if opts.pluginState && opts.action != "" {
		return cliOptions{}, false, errors.New("--plugin-state and --action are mutually exclusive")
	}
	if opts.action != "" && opts.targetID == "" {
		return cliOptions{}, false, errors.New("--action requires --id")
	}
	if opts.targetID != "" && opts.action == "" {
		return cliOptions{}, false, errors.New("--id requires --action")
	}

	return opts, false, nil
}

func runPluginState(cfg config.Config, refreshUsage bool, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	snapshot, err := session.BuildPluginSnapshot(ctx, cfg, refreshUsage, time.Now())
	if err != nil {
		return err
	}
	return writeJSON(stdout, snapshot)
}

func runPluginAction(cfg config.Config, action, targetID string, stdout io.Writer) error {
	res, err := session.RunPluginAction(cfg, action, targetID)
	if err != nil {
		return err
	}
	return writeJSON(stdout, res)
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
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
      --plugin-state       Print headless JSON snapshot for shell widgets and exit
      --refresh-usage      Force fresh quota fetch when used with --plugin-state
      --action <name>      Run a headless session action (pin, unpin, archive, unarchive, trash, restore, purge)
      --id <target>        Session UUID or trash entry name used with --action
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
