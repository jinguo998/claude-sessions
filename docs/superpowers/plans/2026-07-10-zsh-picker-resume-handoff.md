# Zsh Picker Resume Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Start a session selected from the Zsh `Ctrl+S` picker only after ZLE has returned control to the normal shell, so Codex receives terminal stdin without `/dev/tty` redirection or crossterm panic.

**Architecture:** Add a one-shot JSON handoff for `domain.ResumePlan`. Zsh picker mode writes the plan and exits; the widget uses `zle accept-line` to run a hidden consumer after ZLE finishes. Direct picker and full-screen TUI execution remain unchanged.

**Tech Stack:** Go 1.22+, Bubble Tea, Zsh ZLE, tmux, JSON handoff files.

## Global Constraints

- Remove `claude-sessions pick </dev/tty`.
- Preserve `Ctrl+S`, select, resume and the user's existing ZLE input buffer.
- Keep direct picker, full-screen TUI, and Bash behavior unchanged.
- Handoff files are unique, mode `0600`, one-shot, and removed after use.
- Do not modify the user's dirty `codex/tool-display-summaries` checkout.

---

### Task 1: One-shot resume plan handoff

**Files:**
- Create: `internal/platform/shell/handoff.go`
- Create: `internal/platform/shell/handoff_test.go`

**Interfaces:**
- Consumes: `domain.ResumePlan`.
- Produces: `WritePlan(path string, plan domain.ResumePlan) error` and `ConsumePlan(path string) (domain.ResumePlan, error)`.

- [ ] **Step 1: Write failing tests**

Test JSON round-trip, mode `0600`, removal after consume, malformed JSON removal,
and rejection of an empty executable/argv.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/platform/shell -run 'Test(Write|Consume)Plan' -count=1 -v`

Expected: compilation fails because the two functions do not exist.

- [ ] **Step 3: Implement minimal handoff**

`WritePlan` marshals JSON, writes with `0600`, and chmods an existing `mktemp`
file to `0600`. `ConsumePlan` reads and removes before decoding, refuses to
return a reusable plan if removal fails, decodes JSON, and validates executable
plus argv.

- [ ] **Step 4: Verify GREEN and commit**

Run the focused test again, then commit:

```bash
git add internal/platform/shell/handoff.go internal/platform/shell/handoff_test.go
git commit -m "feat: add one-shot resume plan handoff"
```

### Task 2: Defer resume until ZLE exits

**Files:**
- Modify: `cmd/claude-sessions/main.go`
- Modify: `cmd/claude-sessions/main_test.go`

**Interfaces:**
- Consumes: the shell handoff functions and `shell.ExecPlan`.
- Produces: `pick --handoff <path>`, `__exec-handoff <path>`, strict argument parsing, and deferred Zsh execution.

- [ ] **Step 1: Write failing lifecycle and routing tests**

Assert that generated Zsh init has no `/dev/tty` redirect and contains
`pick --handoff`, `zle push-input`, `__exec-handoff`, and `zle accept-line`.
Test strict handoff argument parsing. Test a pure
`executeOrHandoff(plan, path, execFn)` helper: handoff mode must not call
`execFn`; direct mode must call it exactly once.

- [ ] **Step 2: Verify RED**

Run: `go test ./cmd/claude-sessions -run 'Test(ZshPicker|PickHandoff|ExecuteOrHandoff)' -count=1 -v`

Expected: lifecycle and missing-helper failures.

- [ ] **Step 3: Implement minimal command routing**

Parse only `pick` and `pick --handoff <path>`. After selection, direct mode calls
`shell.ExecPlan`; handoff mode writes the plan and returns. Handle
`__exec-handoff <path>` before runtime setup by consuming and executing once.

Generate a widget that saves `BUFFER`/`CURSOR`, uses `mktemp`, invokes handoff
picker mode without redirection, restores the line on cancel, and on selection
uses `zle push-input`, sets `BUFFER` to `cs __exec-handoff
${(q)_cs_handoff}`, and calls `zle accept-line`.

- [ ] **Step 4: Verify GREEN, Zsh syntax, and commit**

Run:

```bash
go test ./cmd/claude-sessions -run 'Test(ZshPicker|PickHandoff|ExecuteOrHandoff|MainInitShellOutput)' -count=1 -v
go run ./cmd/claude-sessions init zsh > /tmp/claude-sessions-init.zsh
zsh -n /tmp/claude-sessions-init.zsh
```

Then commit `main.go` and `main_test.go` with message
`fix: defer zsh picker resume until zle exits`.

### Task 3: tmux/ZLE regression smoke

**Files:**
- Create: `internal/smoke/zsh_picker_tty_test.go`

**Interfaces:**
- Consumes: built command, generated Zsh init, tmux, temporary Codex session, and fake `codex`.
- Produces: proof that the resumed child sees fd 0 as a TTY.

- [ ] **Step 1: Write the failing end-to-end test**

Build the command into a temporary bin directory. Create a temporary
`HOME/.codex/sessions/YYYY/MM/DD` JSONL session and a fake `codex` earlier in
`PATH` that records arguments plus `[ -t 0 ]`. Start an isolated tmux server
running `zsh -f`, evaluate generated init, send `Ctrl+S`, select the fixture,
and wait for the probe file. Require `resume <fixture-id>` and `stdin=tty`.
Always kill the isolated tmux server in cleanup.

- [ ] **Step 2: Prove RED against `main`**

Build `main` into the test bin or temporarily restore its widget behavior. The
probe must be absent or report `stdin=notty`. Restore the implementation after
recording this expected failure.

- [ ] **Step 3: Verify GREEN and commit**

Run: `go test ./internal/smoke -run TestZshPickerResumeGetsTTYInTmux -count=1 -v`

Commit the new smoke test with message
`test: cover zsh picker resume tty handoff`.

### Task 4: Documentation and complete verification

**Files:**
- Modify: `docs/modules.md`
- Modify: `README.md` only if it describes this handoff.

- [ ] **Step 1: Update architecture documentation**

Document direct `syscall.Exec` versus the one-shot ZLE handoff. Do not document
or retain `/dev/tty` redirection.

- [ ] **Step 2: Run format and static checks**

```bash
gofmt -w cmd/claude-sessions/main.go cmd/claude-sessions/main_test.go internal/platform/shell/handoff.go internal/platform/shell/handoff_test.go internal/smoke/zsh_picker_tty_test.go
git diff --check
go vet ./...
make lint
```

- [ ] **Step 3: Run complete tests**

Run: `go test ./... -count=1`

If the known package-init `--version` five-second timeout recurs, rerun that
exact test once and report it separately without changing unrelated startup.

- [ ] **Step 4: Run a real Codex TUI smoke**

Use an isolated tmux server and a copied real session under temporary Codex
state. Launch through Zsh/ZLE handoff, capture the pane, verify neither `stdin
is not a terminal` nor `reader source not set`, then send `Ctrl+C` and clean up.

- [ ] **Step 5: Commit documentation**

Commit documentation with message `docs: explain zsh resume handoff`.

### Task 5: Review, integrate, install, and publish

**Files:**
- No new source files expected.

- [ ] **Step 1: Audit scope**

Run `git status --short`, `git diff main...HEAD --check`, `git diff --stat
main...HEAD`, and `git log --oneline main..HEAD`. Confirm the redirect is gone
and all changes serve the handoff.

- [ ] **Step 2: Integrate without touching the dirty checkout**

Advance `main` from a separate clean integration worktree. Preserve all user
modifications in the primary checkout.

- [ ] **Step 3: Install and verify**

Run `make install` from updated `main`, then verify the installed version and
pipe `claude-sessions init zsh` through `zsh -n`.

- [ ] **Step 4: Push and verify remote**

Push `main` to `origin` and compare local `main` with
`git ls-remote origin refs/heads/main`.

- [ ] **Step 5: Clean temporary worktrees**

Remove only worktrees and merged branches created for this fix, then prove the
original dirty checkout is unchanged.

### Follow-up: Hide the accepted handoff command

- [ ] Add a failing init-script test that rejects assigning `__exec-handoff` to
  `BUFFER` and requires an `add-zsh-hook precmd` registration.
- [ ] Add a failing tmux assertion that the pane never contains
  `__exec-handoff` after resume.
- [ ] Replace the visible accepted command with a global pending handoff, an
  empty accepted line, and an idempotently registered `precmd` consumer.
- [ ] Re-run focused tests, full tests, installed-binary tmux smoke, and real
  Codex smoke before updating `main` and `origin/main`.
