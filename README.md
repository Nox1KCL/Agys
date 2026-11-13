# Agys

A terminal TUI for browsing, searching, organizing, and resuming **Google Antigravity CLI** sessions.

## Features

- **Automatic CLI session discovery** from `~/.gemini/antigravity-cli/` (`brain/`, `knowledge/`, `annotations/`).
- **Live search (`/`)** across session titles, working directories, first and last user prompts, Knowledge Items, and subsessions.
- **Omarchy-aligned styling**: reads your active Omarchy palette (`~/.local/state/omarchy/current/theme/colors.toml` or `~/.config/omarchy/current/theme/colors.toml`) with live reload on focus, preserves transparent terminal backgrounds, and falls back to ANSI 16 terminal colors elsewhere.
- **Right-hand preview pane**: displays metadata (pin/archive flags, relative timestamp, last visited directory, UUID, subsessions), first and last user prompts, Knowledge Items, and artifacts (`task.md` with checklist progress, `implementation_plan.md`, `walkthrough.md`). Switches via `Tab` on narrow terminals (`< 96` columns).
- **Pin & archive workflow**:
  - `p` toggles pinning (`★`), keeping pinned sessions at the top of the list.
  - `a` archives or restores a session (archived sessions are hidden by default).
  - `A` toggles visibility of archived sessions.
  - State persists in `~/.config/agys/state.json` (or `$XDG_CONFIG_HOME/agys/state.json`) layered over `annotations/*.pbtxt` without modifying Antigravity CLI files.
- **Minimal footer & `?` which-key overlay**: shows only essential bindings in the bottom bar while pressing `?` opens a floating LazyVim/Yazi-style keybindings menu on top.
- **Subsession tree navigation** (`l`/`h` or `→`/`←`) and instant session resume via `agy --conversation=<UUID>`.

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

## Keybindings

Press `?` anywhere outside search input to open the floating keybindings overlay.

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | Move selection or scroll preview |
| `PgUp` / `Ctrl+U`, `PgDn` / `Ctrl+D` | Page up / down |
| `g` / `Home`, `G` / `End` | Jump to top / bottom |
| `/` | Enter live search mode |
| `l` / `→`, `h` / `←` | Expand / collapse subsessions |
| `Tab` | Focus preview pane / return to list |
| `p` | Pin / unpin session (`★`) |
| `a` | Archive / restore session |
| `A` | Show / hide archived sessions |
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
│   ├── config/              # Paths for ~/.gemini/antigravity-cli and ~/.config/agys/state.json
│   ├── session/             # Scanners for brain/, knowledge/, annotations/, state.json, and search
│   ├── tui/                 # Root dual-pane Bubble Tea model and key handling
│   │   ├── components/      # Session list, preview formatter, and floating which-key overlay
│   │   └── styles/          # ANSI terminal palette Lip Gloss styles
│   └── utils/               # Date, home path, UUID, and truncation helpers
├── install.sh               # Linux installer (GitHub Releases / local binary + PATH)
├── Makefile
├── go.mod
└── README.md
```

## Development Commands

```bash
make run      # Run in development mode
make build    # Build binary into ./bin/agys
make install  # Build locally and install via ./install.sh --local
make test     # Run test suite
make fmt      # Format Go sources (gofmt)
make vet      # Run go vet
```
