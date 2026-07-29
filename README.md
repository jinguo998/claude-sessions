# claude-sessions

[![CI](https://github.com/jinguo998/claude-sessions/actions/workflows/go.yml/badge.svg)](https://github.com/jinguo998/claude-sessions/actions/workflows/go.yml)
[![CodeQL](https://github.com/jinguo998/claude-sessions/actions/workflows/codeql.yml/badge.svg)](https://github.com/jinguo998/claude-sessions/actions/workflows/codeql.yml)

TUI dashboard for browsing, previewing, searching, and resuming Claude Code, Codex CLI, and OpenCode sessions across all projects.

## Install

Install the prebuilt binary with Homebrew:

```bash
brew install jinguo998/tap/claude-sessions
```

Or download a release directly.

Prebuilt binaries are available for Linux and macOS on both amd64 and arm64:

[Download the latest release](https://github.com/jinguo998/claude-sessions/releases/latest)

Each release includes a `checksums.txt` file for SHA-256 verification.

Or install with Go:

```bash
go install github.com/jinguo998/claude-sessions/cmd/claude-sessions@latest
```

Install a pinned release when tags are available:

```bash
# replace v0.1.0 with the release tag you want
go install github.com/jinguo998/claude-sessions/cmd/claude-sessions@v0.1.0
```

Or build from source:

```bash
git clone git@github.com:jinguo998/claude-sessions.git
cd claude-sessions
make build
```

Check the installed version:

```bash
claude-sessions --version
```

## Setup (optional)

For "cd to project" to work in the current shell, add to `~/.zshrc`:

```bash
eval "$(claude-sessions init zsh)"
```

This creates a `cs` alias. Without it, "cd to project" opens a new shell in the target directory.

## Usage

```bash
claude-sessions
```

### List View

| Key | Action |
|-----|--------|
| `↑/↓` `j/k` | Navigate |
| `Enter` / `p` | Preview session |
| `r` | Resume session with the source default permission mode |
| `R` | Resume session safely |
| `Ctrl+r` | Refresh session list |
| `n` | Fork session (new conversation from here) |
| `d` | Archive session to trash |
| `m` | Context menu |
| `t` | Open stats for the current filtered session set |
| `T` | Open trash for archived sessions |
| `/` | Search sessions |
| `s` | Cycle sort (Recent / Project / Messages / Tools / Tokens) |
| `f` | Open project filter |
| `F` | Cycle source filter (All / Claude / Codex / OpenCode) |
| `M` | Toggle mouse mode for terminal text selection |
| `q` / `Esc` | Quit |

Claude uses fast resume by default (`--dangerously-skip-permissions`). Codex and OpenCode use safe resume by default.

### Preview View

| Key | Action |
|-----|--------|
| `j/k` | Scroll |
| `gg` / `G` | Top / Bottom |
| `Ctrl+d/u` | Half page down/up |
| `Ctrl+f/b` | Full page down/up |
| `/` | Search in preview |
| `n` (in search) / `N` | Next / Previous match |
| `r` | Resume this session with the source default permission mode |
| `R` | Resume this session safely |
| `n` | Fork this session |
| `v` | Toggle verbose mode (full tool content) |
| `m` | Toggle Markdown rendering / raw text |
| `Esc` / `q` / `p` | Back to list |

### Stats View

Stats use the current list filters and search. They show token totals, average turns and tool calls, activity buckets, and ranked sessions to resume.

| Key | Action |
|-----|--------|
| `[` / `]` | Cycle stats time range (All / 30d / 7d / 24h) |
| `1` / `2` / `3` / `4` | Jump to 24h / 7d / 30d / All |
| `j/k` | Move in resume queue |
| `Enter` / `p` | Preview selected queued session |
| `r` | Resume selected session with the source default permission mode |
| `R` | Resume selected session safely |
| `f` | Filter the app to the selected session's project |
| `l` | Jump back to the list focused on the selected session |
| `y` | Copy selected session ID |
| `o` | Open selected JSONL in `$EDITOR` |
| `q` / `Esc` / `t` | Back to list |

### Trash View

Archived Claude and Codex sessions live under `~/.claude-sessions-trash` with their original JSONL, side directory, and `metadata.json`.

OpenCode sessions are backed by SQLite and are not archived by this tool yet. The UI hides Archive for OpenCode context menus and shows a short unsupported message for the `d` shortcut.

| Key | Action |
|-----|--------|
| `j/k` | Move |
| `p` | Preview archived session |
| `Enter` / `r` | Restore selected session |
| `x` | Permanently delete selected archived session (with confirmation) |
| `D` | Empty trash (with confirmation) |
| `q` / `Esc` / `T` | Back to list |

### Mouse

- **Click** to select a session
- **Double-click** to preview
- **Right-click** or **m** for context menu. Available actions depend on the selected source's capabilities.
- In stats, **click** selects a resume queue row and **double-click** previews it
- In trash, **click** selects an archived session and **double-click** previews it
- **Scroll wheel** on left panel scrolls list, on right panel scrolls preview
- **M** toggles mouse reporting off/on. When mouse mode is off, drag to select terminal text; press **M** again to restore click and scroll.
- **⌥+Drag** (Option+drag) also selects text in iTerm2 while mouse mode is on

### Layout

- Narrow terminal: single-column list with detail panel
- Wide terminal (120+ cols): split view with live preview on the right
- The right preview initially loads recent messages quickly. Scroll to the top to load full history on demand.

## Architecture

```
cmd/claude-sessions/       Entry point, shell integration, composition root
internal/
  domain/                  Source-agnostic session model and resume plans
  source/                  Source capability interfaces
    claude/                Claude Code adapter
    codex/                 Codex CLI adapter
    opencode/              OpenCode adapter
  app/                     Scan, preview, query, stats, archive, resume services
  storage/                 Cache and trash interfaces/implementations
  ui/
    tui/                   Full-screen Bubble Tea UI
    picker/                Inline picker used by shell integration
  platform/                Shell, clipboard, editor, and terminal implementations
```

`cmd/claude-sessions` wires source adapters, storage, app services, platform implementations, and UI models. UI packages consume app services/view models and do not instantiate source, storage, or platform implementations directly.

See [docs/modules.md](docs/modules.md) for dependency rules and module boundaries.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
