# Contributing to claude-sessions

## Prerequisites

- The Go version declared in `go.mod`
- Git

## Development

```bash
# Clone
git clone git@github.com:jinguo998/claude-sessions.git
cd claude-sessions

# Build
make build

# Run
./claude-sessions

# Test
make test

# Lint
make lint
```

## Making Changes

1. Create a branch from `main`
2. Make your changes
3. Run `make test && make lint`
4. Push and create a Pull Request
5. Describe the change and how it was tested

## Code Structure

```
cmd/claude-sessions/main.go    Entry point
internal/domain/               Source-agnostic session model
internal/source/               Source capability interfaces and adapters
internal/app/                  Scan, preview, query, stats, archive, resume services
internal/storage/              Cache and trash storage interfaces/implementations
internal/ui/                   Bubble Tea TUI and inline picker
internal/platform/             Shell, clipboard, editor, terminal implementations
testdata/                      Test fixtures
```

## Style

- Follow existing patterns
- `go vet` must pass
- Tests required for new scanner/parser logic

## Releasing

Push a semantic-version tag to build and publish Linux and macOS binaries:

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

Tags with a suffix such as `v0.2.0-rc.1` are published as prereleases.
