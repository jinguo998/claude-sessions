# AGENTS.md

This file is only a routing map for coding agents. Do not duplicate the full project guide here.

`CLAUDE.md` is a compatibility symlink to this file for clients that look for that filename.

## Routes

- Architecture and module map: `docs/modules.md`
- User-facing usage: `README.md`
- Contribution workflow: `CONTRIBUTING.md`

## Code Areas

- Entrypoint and shell integration: `cmd/claude-sessions/`
- Source-agnostic session model: `internal/domain/`
- Source capability interfaces and shared source helpers: `internal/source/`
- Source adapters: `internal/source/claude/`, `internal/source/codex/`, `internal/source/opencode/`
- App services and view models: `internal/app/`
- Cache and trash stores: `internal/storage/`
- Full-screen TUI and inline picker: `internal/ui/tui/`, `internal/ui/picker/`
- Shell, clipboard, editor, and terminal platform implementations: `internal/platform/`
- Architecture and migration smoke tests: `internal/architecture/`, `internal/smoke/`

## Local Files

- Runtime traces and logs belong under `log/`, which is ignored.
- Do not commit generated local logs or runtime trace output.
