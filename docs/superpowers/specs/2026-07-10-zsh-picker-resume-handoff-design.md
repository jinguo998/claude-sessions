# Zsh Picker Resume Handoff Design

## Problem

The `Ctrl+S` Zsh integration runs `claude-sessions pick` from a ZLE widget. In
that context, file descriptor 0 may not be a terminal, especially inside tmux.
Bubble Tea detects the non-terminal stdin and opens `/dev/tty` for the picker,
but that file is private to `Program.Run` and is closed when the picker exits.
The subsequent `syscall.Exec` therefore starts Codex with the widget's original
non-terminal stdin, and Codex exits with `stdin is not a terminal`.

The previous fix changed the widget to run `claude-sessions pick </dev/tty`.
That makes the resumed process inherit a TTY, but still starts Codex while the
ZLE widget owns the command lifecycle. On affected Codex/crossterm combinations
this causes terminal event initialization to panic with `reader source not
set`. The redirect must be removed rather than refined.

## Scope

- Fix resume launches selected through the Zsh `Ctrl+S` picker.
- Preserve the visible workflow: `Ctrl+S`, select a session, press the resume
  key, and enter the selected CLI.
- Preserve the user's existing ZLE input buffer across the resumed CLI.
- Keep direct `claude-sessions pick`, the full-screen TUI, and Bash integration
  behavior unchanged.
- Do not redirect the picker or resumed CLI stdin to `/dev/tty` in the shell
  integration.

## Design

### Picker handoff mode

Add an internal `pick --handoff <path>` mode. The picker scans and renders in
exactly the same way as direct `pick`. After a selection it plans the resume but
writes the resulting `domain.ResumePlan` as JSON to the supplied handoff file
instead of calling `syscall.Exec`. Cancellation leaves no executable handoff.

The handoff file is created by `mktemp` in the Zsh widget, written with mode
`0600`, and removed on cancellation, malformed input, or consumption. A hidden
`__exec-handoff <path>` command reads and removes the file before passing the
plan to the existing `shell.ExecPlan` function.

### ZLE lifecycle

The generated Zsh widget will:

1. Copy the current `BUFFER` and `CURSOR` into local variables.
2. Create a unique resume handoff file.
3. Run `claude-sessions pick --handoff <path>` without stdin redirection.
4. If a resume plan was written, restore the saved buffer, push it for the next
   prompt with `zle push-input`, replace `BUFFER` with the handoff command, and
   call `zle accept-line`.
5. If the picker was cancelled or failed, delete the temporary file and restore
   the saved buffer and cursor immediately.

`accept-line` takes effect after the widget returns. Consequently ZLE has
finished and the normal shell command lifecycle owns fd 0 before
`__exec-handoff` executes Codex. This is the key boundary that neither
`syscall.Exec` inside the picker nor `/dev/tty` redirection provides.

### Invisible scheduling follow-up

The accepted line must be empty. Putting `cs __exec-handoff <path>` in `BUFFER`
causes the implementation command to be echoed and recorded like user input.
Instead, the widget stores the path in a global pending variable, pushes the
user's original input, clears `BUFFER`, and accepts the empty line. A registered
`precmd` hook consumes the pending variable and invokes `cs __exec-handoff`
before Zsh draws the next prompt. The hook runs after ZLE has released the
terminal while keeping the implementation command out of the visible buffer
and shell history.

### Direct picker behavior

Without `--handoff`, `claude-sessions pick` continues to call
`shell.ExecPlan`. Users running it as a normal shell command see no behavior
change. The full-screen TUI also continues using the direct exec path.

## Error Handling

- A missing `--handoff` path is a command-line error.
- Handoff serialization or write failures are printed and return non-zero.
- Empty, missing, or malformed handoff files never execute a process and are
  removed when possible.
- A cancelled picker removes its temporary handoff and restores the prompt.
- Existing resume planning and executable lookup errors retain their current
  messages.

## Verification

- Unit tests assert the generated Zsh integration contains no `/dev/tty`
  redirect and uses handoff plus `zle accept-line`.
- Unit tests cover handoff JSON round-trip, `0600` permissions, malformed input,
  and removal after consumption.
- Command-routing tests prove handoff mode does not execute the plan inside the
  picker process and direct picker behavior remains unchanged.
- A real tmux plus Zsh/ZLE smoke test selects a Codex session and verifies the
  resumed process sees a TTY on stdin.
- A real Codex resume smoke test confirms neither `stdin is not a terminal` nor
  `reader source not set` is produced.
- Run the complete Go test, architecture, vet, and lint commands before merge.
