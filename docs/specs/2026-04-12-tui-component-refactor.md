# TUI Component Refactor — Design Spec

## Problem

`internal/tui/model.go` is 768 lines with all view states, handlers, and state fields in a single struct. Adding features requires understanding the entire file. State management is error-prone (e.g., `defer` on value receiver bug, preview search active state leaking).

## Goal

Refactor the TUI into independent Bubble Tea sub-model components that each own their state, handle their own input, and render themselves. Parent Model becomes a thin message router.

## Architecture

### Component Pattern

Every component follows the Bubble Tea sub-model pattern:

```go
type XxxModel struct { /* own state only */ }

func NewXxxModel() XxxModel
func (x XxxModel) Update(msg tea.Msg) (XxxModel, tea.Cmd)
func (x XxxModel) View() string
```

Components communicate via `tea.Msg`. A component never directly modifies another component's state. The parent Model routes messages between components.

### File Layout

```
internal/tui/
├── model.go           # Parent Model: ~100 lines, message routing + view switching
├── list.go            # ListModel: session list, search, sort, filter, mouse
├── preview.go         # PreviewModel: full-screen viewport + search + vim nav
├── side_preview.go    # SidePreviewModel: async-loaded right panel preview
├── context_menu.go    # ContextMenuModel: right-click menu
├── messages.go        # All cross-component message types
├── styles.go          # All lipgloss style definitions
└── helpers.go         # displayWidth, truncateToWidth, padToWidth, wrapText, etc.
```

### Message Types (messages.go)

All messages that cross component boundaries are defined here:

```go
// Child → Parent messages (actions that require view switching or cross-component coordination)

type SessionSelectedMsg struct{ Session session.Session }   // List: Enter (resume)
type SessionForkMsg struct{ Session session.Session }        // List: o (fork)
type SessionPreviewMsg struct{ Session session.Session }     // List: p (open preview)
type SessionDeleteMsg struct{ Index int }                    // List: d (request delete)
type SessionCdMsg struct{ Session session.Session }          // cd to project
type FilterChangedMsg struct{}                               // List: search/sort/filter changed
type OpenContextMenuMsg struct {                             // List: right-click
    Session session.Session
    X, Y    int
}

type PreviewCloseMsg struct{}                                // Preview: Esc/q/p

type MenuActionMsg struct {                                  // ContextMenu: user picked action
    Action  string                                           // "resume", "fork", "cd", "preview", "delete"
    Session session.Session
}
type MenuCloseMsg struct{}                                   // ContextMenu: closed

// Async messages
type SessionsLoadedMsg []session.Session                     // Scanner finished
type SidePreviewLoadedMsg struct{ ID, Content string }       // Side preview parsed
```

### Component Interfaces

#### ListModel (list.go)

**Owns:** cursor, listOffset, searchInput, searching, searchQuery, sortMode, filterProj, double-click state

**Public API:**
- `NewListModel() ListModel`
- `Update(msg tea.Msg) (ListModel, tea.Cmd)`
- `View() string` — full-width list (narrow terminal)
- `CompactView() string` — left panel in split mode
- `DetailView() string` — metadata panel below list
- `SetSessions([]session.Session) ListModel`
- `SetSize(w, h int) ListModel`
- `SelectedSession() (session.Session, bool)`
- `Cursor() int`

**Handles internally:** j/k, ↑/↓, `/` search toggle + input, `s` sort, `f` filter, mouse click, double-click, scroll wheel, search highlighting

**Emits:** SessionSelectedMsg, SessionForkMsg, SessionPreviewMsg, SessionDeleteMsg, SessionCdMsg, OpenContextMenuMsg, FilterChangedMsg

#### PreviewModel (preview.go)

**Owns:** viewport, previewSearch (embedded), lastKey, title, width, height

**Public API:**
- `NewPreviewModel() PreviewModel`
- `Update(msg tea.Msg) (PreviewModel, tea.Cmd)`
- `View() string`
- `SetContent(title, content string) PreviewModel`
- `SetSize(w, h int) PreviewModel`

**Handles internally:** j/k scroll, gg/G, Ctrl+d/u/f/b, `/` search + n/N, mouse scroll, Esc to clear search

**Emits:** PreviewCloseMsg (on Esc when no search active, or q/p)

#### SidePreviewModel (side_preview.go)

**Owns:** sessionID, content, scroll, width, height, loading

**Public API:**
- `NewSidePreviewModel() SidePreviewModel`
- `Update(msg tea.Msg) (SidePreviewModel, tea.Cmd)`
- `View() string`
- `LoadSession(sess session.Session, width int) (SidePreviewModel, tea.Cmd)` — starts async load
- `SetSize(w, h int) SidePreviewModel`
- `ScrollUp(n int) SidePreviewModel`
- `ScrollDown(n int) SidePreviewModel`

**Handles internally:** SidePreviewLoadedMsg (sets content when async load completes)

**Emits:** nothing (passive component, parent passes scroll events)

#### ContextMenuModel (context_menu.go)

**Owns:** items []MenuItem, cursor, x, y, session, visible

**Public API:**
- `NewContextMenuModel() ContextMenuModel`
- `Update(msg tea.Msg) (ContextMenuModel, tea.Cmd)`
- `View() string` — rendered menu box
- `Open(sess session.Session, x, y int) ContextMenuModel`
- `IsVisible() bool`

**Handles internally:** ↑/↓ selection, Enter execute, Esc close, mouse click on items

**Emits:** MenuActionMsg (with action string + session), MenuCloseMsg

**MenuItem:**
```go
type MenuItem struct {
    Label  string  // "Resume (safe)", "Fork", "cd to project", "Preview", "Delete"
    Action string  // "resume", "fork", "cd", "preview", "delete"
}
```

### Parent Model (model.go)

```go
type Model struct {
    // Sub-components
    list        ListModel
    preview     PreviewModel
    sidePreview SidePreviewModel
    contextMenu ContextMenuModel

    // Shared state (not owned by any component)
    view     viewState        // list | preview | contextMenu | confirmDelete
    width    int
    height   int
    loaded   bool
    deleteIdx int             // for confirm delete

    // Output
    result *Result
}
```

**Update flow (~80 lines):**

```
msg received
  ├─ tea.WindowSizeMsg → update all component sizes
  ├─ SessionsLoadedMsg → list.SetSessions(), trigger side preview
  ├─ SidePreviewLoadedMsg → sidePreview.Update()
  ├─ tea.KeyMsg / tea.MouseMsg →
  │     switch view:
  │       list → list.Update(msg)
  │       preview → preview.Update(msg)
  │       contextMenu → contextMenu.Update(msg)
  │       confirmDelete → handle y/n locally
  ├─ SessionSelectedMsg → set result (resume + skipPerms), quit
  ├─ SessionForkMsg → set result (fork), quit
  ├─ SessionCdMsg → set result (cdOnly), quit
  ├─ SessionPreviewMsg → load content, preview.SetContent(), view=preview
  ├─ SessionDeleteMsg → deleteIdx=idx, view=confirmDelete
  ├─ OpenContextMenuMsg → contextMenu.Open(), view=contextMenu
  ├─ MenuActionMsg → route to appropriate result/view
  ├─ MenuCloseMsg → view=list
  ├─ PreviewCloseMsg → view=list
  └─ FilterChangedMsg → reload side preview
```

**View flow:**

```
switch view:
  list → wideMode?
           yes → list.CompactView() | border | sidePreview.View()
                 + list.DetailView()
           no  → list.View() + list.DetailView()
  preview → preview.View()
  contextMenu → base view + contextMenu overlay
  confirmDelete → base view + confirm prompt
```

### Data Flow for Key Scenarios

**User presses Enter on a session:**
1. `list.Update(KeyMsg "enter")` → list returns `SessionSelectedMsg{session}` via Cmd
2. Parent receives `SessionSelectedMsg` → sets `result = &Result{Dir, ID, SkipPerms: true}`, returns `tea.Quit`
3. `main.go` reads `Result`, does `os.Chdir` + `syscall.Exec`

**User opens preview:**
1. `list.Update(KeyMsg "p")` → list returns `SessionPreviewMsg{session}` via Cmd
2. Parent receives `SessionPreviewMsg` → calls `parser.ParseSessionMessages`, sets `preview.SetContent(title, content)`, `view = viewPreview`
3. All subsequent KeyMsg/MouseMsg routed to `preview.Update()`
4. Preview sends `PreviewCloseMsg` → parent sets `view = viewList`

**User right-clicks:**
1. `list.Update(MouseMsg rightClick)` → list returns `OpenContextMenuMsg{session, x, y}` via Cmd
2. Parent receives `OpenContextMenuMsg` → `contextMenu.Open(session, x, y)`, `view = viewContextMenu`
3. User picks "Fork" → `contextMenu.Update(KeyMsg "enter")` → returns `MenuActionMsg{Action: "fork", Session: s}` via Cmd
4. Parent receives `MenuActionMsg` → sets result, quits

**Cursor moves in list (wide mode):**
1. `list.Update(KeyMsg "j")` → list moves cursor, returns `FilterChangedMsg` via Cmd
2. Parent receives `FilterChangedMsg` → calls `sidePreview.LoadSession(list.SelectedSession(), width)` which returns async Cmd
3. Later: `SidePreviewLoadedMsg` → `sidePreview.Update(msg)` sets content

## What Does NOT Change

- `internal/session/`, `internal/scanner/`, `internal/parser/` — untouched
- `cmd/claude-sessions/main.go` — untouched (same `tui.Model`, `tui.Result` interface)
- `testdata/` — untouched
- Behavior and UX — all features preserved exactly

## Success Criteria

- `model.go` under 150 lines
- Each component file under 250 lines
- `go build ./cmd/claude-sessions/` passes
- `go test ./...` passes
- All existing features work identically
