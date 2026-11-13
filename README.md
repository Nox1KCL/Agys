# Agys

A terminal TUI for browsing, searching, organizing, and resuming **Google Antigravity CLI** sessions.

## Features

- **Automatic CLI session discovery** from `~/.gemini/antigravity-cli/` (`brain/`, `knowledge/`, `annotations/`).
- **Incremental index cache**: stores parsed session and Knowledge Item metadata in `~/.cache/agys/index.gob` (or `$XDG_CACHE_HOME/agys/index.gob`) invalidated via file-stat fingerprints for fast startups without re-reading unchanged logs.
- **Live search (`/`)** across session titles, working directories, first and last user prompts, Knowledge Items, and subsessions backed by a pre-normalized in-memory search index.
- **Omarchy-aligned styling**: reads your active Omarchy palette (`~/.local/state/omarchy/current/theme/colors.toml` or `~/.config/omarchy/current/theme/colors.toml`) with live reload on focus, preserves transparent terminal backgrounds, and falls back to ANSI 16 terminal colors elsewhere.
- **Multi-pane preview & account usage layout**: wide terminals (`>= 138` columns) place **Preview** on the left, **Sessions** in the center, and **Account** **Quota Limits** on the right (`Preview | Sessions | Account`). Preview displays metadata (pin/archive flags, relative timestamp, last visited directory, UUID, subsessions), first and last user prompts, Knowledge Items, and artifacts (`task.md` with checklist progress, `implementation_plan.md`, `walkthrough.md`). Medium terminals show Preview left and Sessions right (`Tab` focuses preview), while `u` always opens/focuses the Account panel at any terminal width and `U` refreshes live quotas via `agy -p /usage --output-format json`.
- **Pin, archive, rename & delete**:
  - `p` toggles pinning (`★`), keeping pinned sessions at the top of the list.
  - `a` archives or restores a session (archived sessions are hidden by default).
  - `A` toggles visibility of archived sessions.
  - `r` renames a session (submit empty to reset back to the auto-detected title).
  - `d` moves a session to Trash (`~/.local/share/Trash`) after a confirmation dialog; `D` permanently deletes it after a danger confirmation dialog.
  - Pin, archive, and rename state persists in `~/.config/agys/state.json` (or `$XDG_CONFIG_HOME/agys/state.json`) layered over `annotations/*.pbtxt` without modifying Antigravity CLI files.
- **Minimal footer & `?` which-key overlay**: shows only essential bindings in the bottom bar while pressing `?` opens a floating LazyVim/Yazi-style keybindings menu on top.
- **Subsession tree navigation** (`l`/`h` or `→`/`←`) and instant session resume via `agy --conversation=<UUID>` (auto-resolving `agy` and launching inside the session's `Last visited` directory when present).

## Installation (Linux)

### Using `install.sh`

Supports **Linux only** (`amd64` / `arm64`). The installer:
1. Uses `curl` (when available) to download the latest GitHub release (`Nox1KCL/Agys`).
2. Falls back to checking a local binary (`./bin/agys`, `./agys`) or building from source with `go build`.
3. Installs to `~/.local/bin/agys` and idempotently adds `~/.local/bin` to `PATH` (`fish`, `zsh`, `bash`, or `~/.profile`).

```bash
curl -fsSL https://raw.githubusercontent.com/Nox1KCL/Agys/main/install.sh | bash
```

Or install locally from a cloned repository:

```bash
./install.sh
# or build locally and skip GitHub Releases:
make install
```

Optional environment variables for `install.sh`:
- `AGYS_REPO` — GitHub repository slug (default: `Nox1KCL/Agys`)
- `AGYS_VERSION` — release tag or `latest`
- `AGYS_INSTALL_DIR` — target directory (default: `~/.local/bin`)

Expected GitHub Release asset names:
- `agys_linux_<arch>.tar.gz` (containing the `agys` executable), or
- `agys-linux-<arch>` / `agys_linux_<arch>` (`<arch>` = `amd64` or `arm64`).

### Manual Build

```bash
git clone https://github.com/Nox1KCL/Agys.git
cd Agys
make build    # builds ./bin/agys
```

## CLI Flags

```text
-d, --dir <path>       Antigravity CLI data directory (default: ~/.gemini/antigravity-cli)
    --state <path>     Agys state file path (default: ~/.config/agys/state.json)
-s, --search <query>   Start with an initial search filter
-a, --archived         Include archived sessions on startup
-e, --exit-on-resume   Exit agys and replace with agy on Enter (returns to shell on /exit)
    --plugin-state     Print headless JSON snapshot for shell integrations and exit
    --refresh-usage    Force fresh quota fetch when used with --plugin-state
    --action <name>    Run headless session action (pin, unpin, archive, unarchive, trash, restore, purge)
    --id <target>      Session UUID or trash entry name used with --action
-v, --version          Print version information and exit
-h, --help             Show help message and exit
```

## Omarchy Bar Plugin (`nox.agys`)

This repository also ships a lightweight native [Omarchy](https://omarchy.org/) shell bar plugin in `plugin/nox.agys/` powered directly by the `agys` binary (`--plugin-state` / `--action`):

- **Bar indicator**: theme-colored star icon with a compact corner orb displaying active session count and pulsing when working.
- **Quota Limits**: live model group limits and reset countdowns.
- **Paginated sessions**: dedicated **Pinned** and **Recent** lists paginated in chunks of 5 (`‹ 1 2 3 … ›`), plus **Archive** and **Trash** views.
- **Hover actions**: pin/unpin, archive/unarchive, move to Trash, restore, and permanent delete (with inline confirmation).
- **Low-quota desktop alerts**: notifications via `omarchy-notification-send` below the configured threshold.

Install the plugin after installing `agys`:

```bash
make install install-plugin
omarchy restart shell
```

Then add `nox.agys` to `~/.config/omarchy/shell.json` under `bar.layout.right`:

```json
{
  "id": "nox.agys",
  "refreshIntervalSec": 60,
  "pageSize": 5,
  "showBadge": true,
  "enableQuotaAlerts": true,
  "quotaAlertThreshold": 15,
  "terminalCommand": ""
}
```

## Keybindings

Press `?` anywhere outside text input to open the floating keybindings overlay.

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | Move selection or scroll preview |
| `PgUp` / `Ctrl+U`, `PgDn` / `Ctrl+D` | Page up / down |
| `g` / `Home`, `G` / `End` | Jump to top / bottom |
| `/` | Enter live search mode |
| `l` / `→`, `h` / `←` | Expand / collapse subsessions |
| `Tab` | Focus preview pane / return to list |
| `u` | Toggle / focus Account Quota Limits pane |
| `U` | Refresh account quota limits live (`agy /usage`) |
| `r` | Rename selected session (`state.json` override) |
| `p` | Pin / unpin session (`★`) |
| `a` | Archive / restore session |
| `A` | Show / hide archived sessions |
| `d` | Move selected session to Trash (with confirmation) |
| `D` | Permanently delete selected session (with confirmation) |
| `Enter` | Resume selected session (`agy --conversation=<UUID>`) |
| `Esc` | Clear search / exit preview focus / close overlay |
| `?` | Toggle floating which-key menu |
| `q` / `Ctrl+C` | Quit |

## Stack

- Go
- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)
- [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles)
- `path/filepath` (stdlib)

## Project Structure

```text
Agys/
├── cmd/
│   └── agys/
│       └── main.go          # CLI entrypoint
├── internal/
│   ├── config/              # Paths for ~/.gemini/antigravity-cli, state.json, and ~/.cache/agys/index.gob
│   ├── session/             # Scanners, incremental gob cache, usage parser, state.json, and search index
│   ├── tui/                 # Root multi-pane Bubble Tea model and key handling
│   │   ├── components/      # Session list, preview & account formatters, dialogs, and which-key overlay
│   │   └── styles/          # ANSI terminal palette Lip Gloss styles
│   └── utils/               # Date, home path, UUID, and truncation helpers
├── install.sh               # Linux installer (GitHub Releases / local binary + PATH)
├── plugin/
│   └── nox.agys/            # Omarchy bar-widget QML plugin backed by agys
├── Makefile
├── go.mod
└── README.md
```

## Development Commands

```bash
make run             # Run in development mode
make build           # Build binary into ./bin/agys
make install         # Build locally and install via ./install.sh --local
make install-plugin  # Install nox.agys into ~/.config/omarchy/plugins/nox.agys
make test            # Run test suite
make fmt             # Format Go sources (gofmt)
make vet             # Run go vet
```
