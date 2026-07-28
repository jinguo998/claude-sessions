# Fuzzy Picker Design

## Goal

Add an fzf-style inline fuzzy search picker (`claude-sessions pick`) that lets users quickly find and resume a session via a shell hotkey, without entering the full-screen TUI dashboard.

## User Flow

1. User presses `ctrl+s` (configurable) in their shell
2. A half-height inline picker appears at the bottom of the terminal (no alt screen)
3. User types keywords — list filters in real-time with fuzzy matching
4. User moves selection with `↑/↓` or `ctrl+p/ctrl+n`, presses `Enter` to resume
5. Terminal restores, `os.Chdir` + `syscall.Exec("claude"/"codex")` replaces the process

`Esc` or `ctrl+c` cancels — terminal returns to normal, nothing happens.

## Interface Layout

```
> query_                                     12/128
> C  3h ago   claude-sessions  除了这个 terminal 以外，我还想做一个类似 [fzf]...
  C  5d ago   dotfiles         ...匹配: "install [fzf] and ripgrep"
  X  1d ago   my-project       fix the login bug
  ...
```

- **Line 1:** `> ` + text input + right-aligned match count / total count
- **Lines 2+:** Result list. Selected row marked with `>` prefix + bold/reverse style
- **Height:** terminal height / 2. Line 1 is the input; the rest is the list.

### Row Format

`源(C/X)` + `相对时间(~10ch)` + `项目名(~15ch)` + `FirstMsg(剩余宽度，截断)`

- Source badge: `C` (blue) for Claude, `X` (green) for Codex — same colors as existing TUI
- Matched characters highlighted in bold yellow (same as existing `highlightStyle`)
- Relative time and project name in dim color
- All colors use `lipgloss.AdaptiveColor` for light/dark terminal support

### Context Snippet

When the fuzzy match hits SearchText but NOT FirstMsg, the row appends a context snippet:

```
  C  5d ago   dotfiles   setup dotfiles for new mac  ...匹配: "install [fzf] and ripgrep"
```

- Extract ~20 characters before and after the first matched character in SearchText
- Truncate to word boundaries
- Highlight matched characters in the snippet
- Max snippet width: ~40 characters

## Architecture

### New Files

- **`internal/picker/picker.go`** — `PickerModel` (Bubble Tea component): text input + fuzzy-filtered list + rendering. Uses `tea.NewProgram()` without `tea.WithAltScreen()` for inline mode.
- **`internal/picker/fuzzy.go`** — Wraps `sahilm/fuzzy` library. Provides match execution, result sorting, and context snippet extraction.

### Modified Files

- **`cmd/claude-sessions/main.go`** — New `pick` subcommand branch. Creates Bubble Tea program (no alt screen), exit flow identical to existing TUI (os.Chdir + syscall.Exec dispatched by session.Source).
- **`cmd/claude-sessions/main.go`** — `shellInit` split into zsh-specific and bash-specific output, each including the `ctrl+s` widget binding alongside the existing `cs()` function.

### Reused Code

- `scanner.ScanAllSessions()` — loads all Claude + Codex sessions
- `session.Session` — data struct (SearchText for matching, Source for resume dispatch)
- No dependency on `internal/tui/`

### New Dependency

- `github.com/sahilm/fuzzy` — Pure Go fuzzy matching library, zero transitive dependencies. Returns `MatchedIndexes` (character positions) for highlight rendering.

### Data Flow

```
scanner.ScanAllSessions() → []Session
        │
  User types query → fuzzy.Find(query, sessions via SearchText) → []Match{Index, MatchedIndexes}
        │
  Render: highlight matched chars in FirstMsg or extract context snippet
        │
  Enter → session → os.Chdir + syscall.Exec("claude"/"codex")
```

## Fuzzy Matching

### Algorithm

`sahilm/fuzzy` implements Smith-Waterman-like scoring: non-contiguous character matches are allowed, consecutive matches and word-boundary matches score higher.

### Match Target

Each session's `SearchText` field (all user messages joined, lowercased) — same field the existing TUI search uses.

### Display Logic

1. Run fuzzy match against `SearchText` to get overall match + score
2. Run a second fuzzy match against `FirstMsg` alone
3. If FirstMsg matches: highlight matched characters directly in FirstMsg
4. If FirstMsg does NOT match: display FirstMsg normally, append context snippet from SearchText showing where the match occurred

### Sort Order

- Empty query (just opened): all sessions sorted by `LastTime` descending (most recent first)
- Non-empty query: sorted by fuzzy match score (best match first)

## Shell Integration

### zsh

```zsh
_claude_sessions_pick() {
    claude-sessions pick
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
    zle reset-prompt
}
zle -N _claude_sessions_pick
bindkey '^s' _claude_sessions_pick
```

### bash

```bash
_claude_sessions_pick() {
    claude-sessions pick
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}
bind -x '"\C-s": _claude_sessions_pick'
```

`claude-sessions init zsh` outputs zsh version (with zle widget + bindkey). `claude-sessions init bash` outputs bash version (with bind -x). Both include the existing `cs()` function.

`zle reset-prompt` ensures the zsh prompt redraws correctly after the picker exits.

## Rendering Details

### Colors

Reuse the `adaptive()` helper from `internal/tui/styles.go` — or define equivalent adaptive styles in the picker package. No raw ANSI needed: the picker renders each line as a single string (no nested lipgloss Render wrapping row backgrounds around inline badges), so lipgloss works fine here.

- Matched characters: bold yellow (`highlightStyle` equivalent)
- Source badge: C blue / X green (same as existing TUI)
- Selected row: reverse video
- Time, project name: dim
- Input prompt `>`: bold

### Width Handling

- Project name: fixed ~15 characters, truncated with ellipsis
- FirstMsg: fills remaining width, truncated with ellipsis
- Context snippet: max ~40 characters, appended after FirstMsg when needed
- CJK/fullwidth characters counted as width 2 (using `go-runewidth`, already a dependency)

## Keybindings

| Key | Action |
|-----|--------|
| Any character | Append to query, re-filter |
| `Backspace` | Delete last character |
| `↑` / `ctrl+p` | Move selection up |
| `↓` / `ctrl+n` | Move selection down |
| `Enter` | Resume selected session |
| `Esc` / `ctrl+c` | Cancel, exit |

No other actions. Preview, fork, delete — all left to the full TUI.

## Testing

### Testable Pure Logic (`internal/picker/`)

- **`fuzzy_test.go`**: Given query + sessions → verify correct matches returned, correct ordering, correct MatchedIndexes
- **`picker_test.go`**: Given session + match info + terminal width → verify formatted row string (badge, truncation, snippet, highlight positions)
- Context snippet extraction: given SearchText + match positions → verify extracted snippet text and bounds

### Not Tested

- Bubble Tea rendering and keyboard interaction (requires real TTY)
- Shell widget integration
