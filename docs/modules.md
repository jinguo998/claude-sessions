# Module Map

This document maps the current layered session manager architecture.

## Dependency Rules

- `internal/domain`: source-agnostic concepts only; no internal imports.
- `internal/source`: source capability interfaces and shared source helpers; imports `domain`.
- `internal/source/claude`, `internal/source/codex`: adapt external JSONL/path formats into `domain`.
- `internal/storage`: storage interfaces; imports `domain`.
- `internal/storage/cache`, `internal/storage/trash`: concrete storage implementations.
- `internal/app`: services and app view models; depends on `domain`, source/storage interfaces, and `app/ports`.
- `internal/platform`: concrete shell, clipboard, editor, and terminal implementations.
- `internal/ui`: TUI and picker presentation; depends on app services/view models, never source/storage/platform implementations.
- `cmd/claude-sessions`: the only composition root; constructs adapters, stores, services, platform implementations, and UI models.

Run `make arch` to check these boundaries.

## `cmd/claude-sessions`

Entrypoint, command routing, shell integration, runtime wiring, and final process handoff.

- Creates Claude/Codex adapters, cache/trash stores, app services, platform implementations, and UI services.
- Handles `init`, `pick`, default TUI mode, version output, and resume/fork/cd execution.
- Direct TUI and command-line picker resumes use `syscall.Exec`; cd handoff uses
  `/tmp/claude-sessions-cd`.
- The Zsh `Ctrl+S` widget cannot exec an interactive CLI while ZLE owns stdin.
  It writes a one-shot `ResumePlan` handoff, exits the picker, and schedules the
  handoff consumer with `zle accept-line` so the resumed CLI starts from the
  normal shell terminal context.

## `internal/domain`

Pure source-agnostic concepts:

- `Session`, `Source`, `Project`, `ConversationTurn`, `TokenUsage`
- `ResumeTarget`, `ResumePlan`

Source-specific metadata must be normalized into fields such as `Title`, `Client`, `Origin`, `Labels`, or `Attributes`. Search corpus and trash sidecar details do not belong here.

## `internal/source`

Capability interfaces and shared source helpers:

- `Scanner`
- `PreviewParser`
- `ResumePlanner`
- `ArchiveSpecifier`

Unsupported capabilities return explicit capability errors instead of empty behavior.

## `internal/source/claude` and `internal/source/codex`

Concrete source adapters.

- Discover and scan session files.
- Parse full and tail previews.
- Map source metadata into domain fields.
- Build source-specific resume/fork/cd plans and archive side-dir specs.

## `internal/storage`

Storage interfaces:

- `MetadataCache`
- `TrashStore`
- `CachedSession`
- `ArchivedSessionMetadata`

Archive paths, metadata JSON, and restore sidecar state stay in storage/app archive code, not domain.

## `internal/storage/cache`

Metadata cache keyed by file size and mtime. Cache versioning lives here; derived fields can be rebuilt by app/query when needed.

## `internal/storage/trash`

Trash archive implementation. Preserves metadata needed to restore session files and side directories.

## `internal/app`

Application services and app-level models:

- `app/scan`: scans all sources through `source.Scanner`, applies cache, returns `ScanResult` with warnings.
- `app/preview`: dispatches preview parsing through `source.PreviewParser`.
- `app/query`: builds `IndexedSession` search corpus outside domain and filters sessions.
- `app/stats`: computes dashboard model from sessions.
- `app/archive`: coordinates source archive specs with trash storage.
- `app/resume`: dispatches resume plan generation through `source.ResumePlanner`.
- `app/model`: presentation-facing session model including derived `SearchText`.
- `app/ports`: platform-facing interfaces consumed by app/UI.

App subpackages should not form cycles. Shared app DTOs belong in `app/model`; otherwise each service owns its output model.

## `internal/ui/tui`

Full-screen Bubble Tea presentation.

- Consumes app services and app view models.
- Dispatches user actions into app services.
- Renders list, preview, stats, trash, project picker, context menu, and side preview.
- Does not parse JSONL, manage cache/trash internals, or instantiate source/platform implementations.

## `internal/ui/picker`

Inline fzf-style picker for `claude-sessions pick`.

- Consumes app session models.
- Performs presentation-level matching/highlighting.
- Returns a selected session to command-level handoff.

## `internal/platform`

Environment-specific implementations:

- `clipboard`: system clipboard commands.
- `editor`: `$EDITOR` launcher.
- `shell`: process exec, cd handoff, and one-shot resume-plan handoff.
- `terminal`: terminal theme detection.

Platform packages do not know about UI or source implementations.

## Test Guards

- `internal/architecture`: import boundary and legacy package deletion checks.
- `internal/smoke`: cross-layer migration smoke tests for preview, picker, resume, and archive restore.
- Source, storage, app, picker, and TUI packages keep focused unit coverage.
