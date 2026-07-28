# TUI Component Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the TUI from a monolithic 1645-line Model into independent Bubble Tea sub-model components communicating via messages.

**Architecture:** Extract ListModel, PreviewModel, SidePreviewModel, ContextMenuModel as independent sub-models. Add messages.go for cross-component messages, styles.go for all styles, helpers.go for utility functions. Parent Model becomes a thin ~100-line message router. All files stay in `package tui`.

**Tech Stack:** Go 1.26, Bubble Tea, Bubbles, Lip Gloss, go-runewidth

---

## Strategy

Since all files are in `package tui`, we can migrate incrementally:
1. First create the new foundation files (messages, styles, helpers) by extracting from existing code
2. Then create each component one at a time, migrating code from model.go and view.go
3. Finally slim down model.go to a thin router

Each task produces a **compilable, runnable** binary. No big-bang rewrite.

## File Structure

| File | Responsibility | Lines (est.) |
|------|---------------|-------------|
| `messages.go` | All cross-component message types | ~40 |
| `styles.go` | All lipgloss style definitions | ~60 |
| `helpers.go` | displayWidth, truncateToWidth, padToWidth, wrapText, highlightQuery, findMatchSnippet, relativeTime | ~120 |
| `context_menu.go` | ContextMenuModel: struct, Update, View, Open | ~100 |
| `side_preview.go` | SidePreviewModel: struct, Update, View, LoadSession, async | ~120 |
| `preview.go` | PreviewModel: struct, Update, View, SetContent (includes previewSearch) | ~250 |
| `list.go` | ListModel: struct, Update, View, CompactView, DetailView, search/sort/filter/mouse | ~350 |
| `model.go` | Parent Model: thin router, ~100 lines | ~100 |
| `preview_search.go` | Keep as-is (already well-encapsulated) | ~160 |

---

### Task 1: Extract styles.go and helpers.go

**Files:**
- Create: `internal/tui/styles.go`
- Create: `internal/tui/helpers.go`
- Modify: `internal/tui/view.go` — remove extracted code

These are pure extractions — move code to new files, delete from old files. No logic changes.

- [ ] **Step 1: Create styles.go**

Move ALL `var ( ... )` style blocks from `view.go` to a new `styles.go`. This includes: `titleStyle`, `selectedStyle`, `normalStyle`, `dimStyle`, `helpStyle`, `confirmStyle`, `userStyle`, `assistantStyle`, `userTextStyle`, `assistantTextStyle`, `toolStyle`, `highlightStyle`, `highlightSelectedStyle`, `loadingStyle`, `emptyStyle`, `detailLabelStyle`, `detailValueStyle`, `pathStyle`, `searchStatusStyle`, `menuStyle`, `menuItemStyle`, `menuSelectedStyle`.

The file should be `package tui` with only `import "github.com/charmbracelet/lipgloss"`.

- [ ] **Step 2: Create helpers.go**

Move these functions from `view.go` to `helpers.go`: `relativeTime`, `displayWidth`, `truncateToWidth`, `padToWidth`, `wrapText`, `highlightQuery`, `findMatchSnippet`, `formatPreviewWithColors`.

The file should be `package tui` with imports for `fmt`, `strings`, `time`, `github.com/charmbracelet/lipgloss`, `github.com/mattn/go-runewidth`, and the parser package.

- [ ] **Step 3: Remove extracted code from view.go**

Delete the style var blocks and helper functions from `view.go`. Keep only the render methods and the const blocks.

- [ ] **Step 4: Build and test**

```bash
cd ~/claude-sessions && go build ./cmd/claude-sessions/ && go test ./... && go vet ./...
```

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add -A && git commit -m "refactor: extract styles.go and helpers.go from view.go"
```

---

### Task 2: Create messages.go

**Files:**
- Create: `internal/tui/messages.go`
- Modify: `internal/tui/model.go` — remove `sessionsLoadedMsg` and `sidePreviewLoadedMsg` type definitions (they move to messages.go)

- [ ] **Step 1: Create messages.go with all message types**

```go
package tui

import session "github.com/jinguo998/claude-sessions/internal/app/model"

// === Child → Parent messages ===

// SessionSelectedMsg is sent when user presses Enter to resume a session.
type SessionSelectedMsg struct{ Session session.Session }

// SessionForkMsg is sent when user presses 'o' to fork a session.
type SessionForkMsg struct{ Session session.Session }

// SessionPreviewMsg is sent when user presses 'p' to preview a session.
type SessionPreviewMsg struct{ Session session.Session }

// SessionDeleteMsg is sent when user presses 'd' to request delete.
type SessionDeleteMsg struct{ Index int }

// SessionCdMsg is sent when user selects 'cd to project'.
type SessionCdMsg struct{ Session session.Session }

// FilterChangedMsg is sent when list search/sort/filter changes (triggers side preview reload).
type FilterChangedMsg struct{}

// OpenContextMenuMsg is sent on right-click.
type OpenContextMenuMsg struct {
	Session session.Session
	X, Y    int
}

// PreviewCloseMsg is sent when preview wants to close.
type PreviewCloseMsg struct{}

// MenuActionMsg is sent when user selects a context menu item.
type MenuActionMsg struct {
	Action  string // "resume", "fork", "cd", "preview", "delete"
	Session session.Session
}

// MenuCloseMsg is sent when context menu is closed without action.
type MenuCloseMsg struct{}

// === Async messages ===

// SessionsLoadedMsg is sent when the initial session scan completes.
type SessionsLoadedMsg []session.Session

// SidePreviewLoadedMsg is sent when async side preview parsing completes.
type SidePreviewLoadedMsg struct {
	SessionID string
	Content   string
}
```

- [ ] **Step 2: Remove old message types from model.go**

Remove the `sessionsLoadedMsg` and `sidePreviewLoadedMsg` type definitions from model.go. Update all references to use the new capitalized names: `SessionsLoadedMsg` and `SidePreviewLoadedMsg`.

- [ ] **Step 3: Build and test**

```bash
cd ~/claude-sessions && go build ./cmd/claude-sessions/ && go test ./... && go vet ./...
```

- [ ] **Step 4: Commit**

```bash
cd ~/claude-sessions && git add -A && git commit -m "refactor: create messages.go with all cross-component message types"
```

---

### Task 3: Extract ContextMenuModel

**Files:**
- Create: `internal/tui/context_menu.go`
- Modify: `internal/tui/model.go` — remove menu state and updateContextMenu
- Modify: `internal/tui/view.go` — remove overlayContextMenu and menu rendering

- [ ] **Step 1: Create context_menu.go**

Define `ContextMenuModel` struct with: `items []MenuItem`, `cursor int`, `x int`, `y int`, `session session.Session`, `visible bool`.

Define `MenuItem` struct with `Label string` and `Action string`.

Implement:
- `NewContextMenuModel() ContextMenuModel` — initializes with default menu items
- `(c ContextMenuModel) Update(msg tea.Msg) (ContextMenuModel, tea.Cmd)` — handles KeyMsg (up/down/enter/esc) and MouseMsg (click on items, click outside)
- `(c ContextMenuModel) View() string` — renders the menu box
- `(c ContextMenuModel) OverlayOn(base string) string` — overlays the menu on base content (replaces entire lines)
- `(c ContextMenuModel) Open(sess session.Session, x, y int) ContextMenuModel`
- `(c ContextMenuModel) IsVisible() bool`

On Enter: return Cmd that sends `MenuActionMsg{Action, Session}`. On Esc or outside click: return Cmd that sends `MenuCloseMsg`.

Move menu items, menu constants (`menuWidth`, `menuHeight`), and the overlay logic from view.go.
Move `updateContextMenu` and `executeMenuAction` logic from model.go.

- [ ] **Step 2: Update model.go**

Remove `menuCursor`, `menuX`, `menuY` fields from Model. Remove `updateContextMenu` method and `executeMenuAction` method. Add `contextMenu ContextMenuModel` field. In the `Update` dispatcher, when `view == viewContextMenu`, forward msg to `m.contextMenu.Update(msg)` and handle returned Cmds.

- [ ] **Step 3: Update view.go**

Remove `overlayContextMenu`, `menuItems`, `menuWidth`, `menuHeight`, `renderContextMenu` (all menu-related code). In the `View()` method, for `viewContextMenu`, call `m.contextMenu.OverlayOn(baseView)`.

- [ ] **Step 4: Build and test**

```bash
cd ~/claude-sessions && go build ./cmd/claude-sessions/ && go test ./... && go vet ./...
```

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add -A && git commit -m "refactor: extract ContextMenuModel as independent sub-component"
```

---

### Task 4: Extract SidePreviewModel

**Files:**
- Create: `internal/tui/side_preview.go`
- Modify: `internal/tui/model.go` — remove side preview state and ensureSidePreview

- [ ] **Step 1: Create side_preview.go**

Define `SidePreviewModel` struct with: `sessionID string`, `content string`, `scroll int`, `width int`, `height int`, `loading bool`.

Implement:
- `NewSidePreviewModel() SidePreviewModel`
- `(s SidePreviewModel) Update(msg tea.Msg) (SidePreviewModel, tea.Cmd)` — handles `SidePreviewLoadedMsg`
- `(s SidePreviewModel) View() string` — renders the preview content with scroll offset
- `(s SidePreviewModel) LoadSession(sess session.Session, width int) (SidePreviewModel, tea.Cmd)` — sets loading=true, returns async Cmd that parses JSONL and sends `SidePreviewLoadedMsg`
- `(s SidePreviewModel) SetSize(w, h int) SidePreviewModel`
- `(s SidePreviewModel) ScrollUp(n int) SidePreviewModel`
- `(s SidePreviewModel) ScrollDown(n int) SidePreviewModel`
- `(s SidePreviewModel) NeedsReload(sessionID string) bool`

Move the `loadSidePreviewCmd` logic and side preview rendering from model.go and view.go.

- [ ] **Step 2: Update model.go**

Remove `sidePreviewID`, `sidePreviewStr`, `sidePreviewScroll` fields. Remove `ensureSidePreview` and `loadSidePreviewCmd` methods. Add `sidePreview SidePreviewModel` field. In Update, handle `SidePreviewLoadedMsg` by forwarding to `m.sidePreview.Update(msg)`. When cursor changes, call `m.sidePreview.LoadSession()`.

- [ ] **Step 3: Update view.go**

In `renderSplitView`, replace inline side preview rendering with `m.sidePreview.View()`. Remove side preview scroll rendering code.

- [ ] **Step 4: Build and test**

```bash
cd ~/claude-sessions && go build ./cmd/claude-sessions/ && go test ./... && go vet ./...
```

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add -A && git commit -m "refactor: extract SidePreviewModel with async loading"
```

---

### Task 5: Extract PreviewModel

**Files:**
- Create: `internal/tui/preview.go`
- Modify: `internal/tui/model.go` — remove preview state and updatePreview
- Modify: `internal/tui/view.go` — remove renderPreview

- [ ] **Step 1: Create preview.go**

Define `PreviewModel` struct with: `viewport viewport.Model`, `search previewSearch`, `lastKey string`, `title string`, `width int`, `height int`.

Implement:
- `NewPreviewModel() PreviewModel`
- `(p PreviewModel) Update(msg tea.Msg) (PreviewModel, tea.Cmd)` — all preview key handling: vim nav (gg/G/Ctrl+d/u/f/b), search (/ + n/N), scroll, Esc→PreviewCloseMsg. Also handles MouseMsg (scroll wheel).
- `(p PreviewModel) View() string` — title + viewport + search bar/help bar
- `(p PreviewModel) SetContent(title, content string) PreviewModel` — sets viewport content + search content, scrolls to bottom
- `(p PreviewModel) SetSize(w, h int) PreviewModel`

Move `updatePreview` logic and `renderPreview` from model.go and view.go. Embed `previewSearch` (preview_search.go stays as-is, it's already a clean component).

- [ ] **Step 2: Update model.go**

Remove `preview viewport.Model`, `previewSearch previewSearch`, `lastKey string` fields. Remove `updatePreview` method. Add `preview PreviewModel` field. In Update dispatcher, forward to `m.preview.Update(msg)` when `view == viewPreview`. Handle `PreviewCloseMsg` by setting `view = viewList`.

- [ ] **Step 3: Update view.go**

Remove `renderPreview` method. In `View()`, for `viewPreview`, return `m.preview.View()`.

- [ ] **Step 4: Build and test**

```bash
cd ~/claude-sessions && go build ./cmd/claude-sessions/ && go test ./... && go vet ./...
```

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add -A && git commit -m "refactor: extract PreviewModel with search and vim navigation"
```

---

### Task 6: Extract ListModel

**Files:**
- Create: `internal/tui/list.go`
- Modify: `internal/tui/model.go` — remove list state and updateList/updateSearching/handleMouse
- Modify: `internal/tui/view.go` — remove renderList/renderSplitView/renderSessionRows

This is the largest extraction.

- [ ] **Step 1: Create list.go**

Define `ListModel` struct with: `sessions []session.Session`, `filtered []session.Session`, `cursor int`, `listOffset int`, `searchInput textinput.Model`, `searching bool`, `searchQuery string`, `sortMode sortMode`, `filterProj string`, `width int`, `height int`, `lastClickTime time.Time`, `lastClickIdx int`.

Also define `sortMode` type and constants (`sortRecent`, `sortProject`, `sortMsgCount`, `sortModeCount`) and `sortMode.String()` — move these from model.go.

Implement:
- `NewListModel() ListModel`
- `(l ListModel) Update(msg tea.Msg) (ListModel, tea.Cmd)` — handles all list keys (j/k/enter/o/p/d/s/f/esc/q, search mode), all mouse events (click, double-click, right-click, scroll wheel on left panel). Emits SessionSelectedMsg, SessionForkMsg, SessionPreviewMsg, SessionDeleteMsg, SessionCdMsg, OpenContextMenuMsg, FilterChangedMsg.
- `(l ListModel) View() string` — full-width list view (narrow terminal)
- `(l ListModel) CompactView() string` — left panel for split mode
- `(l ListModel) DetailView() string` — metadata + path + last msg for selected item
- `(l ListModel) SetSessions(sessions []session.Session) ListModel` — sets sessions and applies filter/sort
- `(l ListModel) SetSize(w, h int) ListModel`
- `(l ListModel) SelectedSession() (session.Session, bool)`
- `(l ListModel) Cursor() int`

Move `updateList`, `updateSearching`, list-related parts of `handleMouse`, `renderList`, `renderSplitView` (left panel part), `renderSessionRows`, `renderDeleteConfirm`, `applySort`, `applyFilter`, `updateListOffset`, `uniqueProjects` from model.go and view.go.

- [ ] **Step 2: Update model.go**

Remove ALL list-related fields (cursor, listOffset, searchInput, searching, searchQuery, sortMode, filterProj, lastClickTime, lastClickIdx) and methods (updateList, updateSearching, handleMouse, applySort, applyFilter, updateListOffset, uniqueProjects). Remove sortMode type.

Add `list ListModel` field. In Update:
- Forward KeyMsg/MouseMsg to `m.list.Update(msg)` when view is list
- Handle all SessionXxxMsg by setting result and quitting (or changing view)
- Handle FilterChangedMsg by triggering side preview reload
- Handle OpenContextMenuMsg by opening context menu

- [ ] **Step 3: Update view.go**

Remove `renderList`, `renderSplitView`, `renderSessionRows`, `renderDeleteConfirm`, and all related const blocks (column widths, padding). In `View()`:
- For viewList: if wideMode, compose `m.list.CompactView()` | border | `m.sidePreview.View()` + `m.list.DetailView()`; else `m.list.View()` + `m.list.DetailView()`
- For viewConfirmDelete: show confirm prompt (can be a simple function, no component needed)

At this point view.go should be very small — just the `View()` dispatcher and the split-view combiner.

- [ ] **Step 4: Build and test**

```bash
cd ~/claude-sessions && go build ./cmd/claude-sessions/ && go test ./... && go vet ./...
```

- [ ] **Step 5: Commit**

```bash
cd ~/claude-sessions && git add -A && git commit -m "refactor: extract ListModel — search, sort, filter, mouse, all list rendering"
```

---

### Task 7: Slim down model.go to thin router

**Files:**
- Modify: `internal/tui/model.go` — final cleanup

At this point model.go should already be slim. This task is the final cleanup pass.

- [ ] **Step 1: Verify model.go structure**

Model should now contain only:
- `viewState` type and constants
- `Result` type
- `Model` struct with: `list`, `preview`, `sidePreview`, `contextMenu`, `view`, `width`, `height`, `loaded`, `deleteIdx`, `result`
- `InitialModel()` — creates all sub-components
- `Init()` — returns scan Cmd
- `Update()` — thin router (~80 lines)
- `View()` — thin dispatcher + split view combiner
- `Result()` — returns result for main.go
- `wideMode()` — helper

Remove any orphaned code. Ensure no unused imports.

- [ ] **Step 2: Verify line counts**

```bash
cd ~/claude-sessions && wc -l internal/tui/*.go
```

Target: model.go < 150, each component < 250.

- [ ] **Step 3: Full build, test, vet**

```bash
cd ~/claude-sessions && go build ./cmd/claude-sessions/ && go test ./... && go vet ./...
```

- [ ] **Step 4: Commit**

```bash
cd ~/claude-sessions && git add -A && git commit -m "refactor: final cleanup — model.go is now a thin router"
```

---

### Task 8: Update CLAUDE.md and push

**Files:**
- Modify: `CLAUDE.md` — update architecture section
- Modify: `README.md` — update architecture section

- [ ] **Step 1: Update CLAUDE.md architecture section**

Update the package description for `internal/tui/` to reflect the new component structure:
```
- internal/tui/model.go — Parent Model, thin message router (~100 lines)
- internal/tui/list.go — ListModel: session list, search, sort, filter, mouse
- internal/tui/preview.go — PreviewModel: full-screen viewport + search + vim nav
- internal/tui/side_preview.go — SidePreviewModel: async-loaded right panel
- internal/tui/context_menu.go — ContextMenuModel: right-click menu
- internal/tui/preview_search.go — In-preview search component
- internal/tui/messages.go — Cross-component message types
- internal/tui/styles.go — All lipgloss styles
- internal/tui/helpers.go — Width calculation, text formatting utilities
```

- [ ] **Step 2: Update README.md architecture section**

Update the architecture block to match.

- [ ] **Step 3: Commit and push**

```bash
cd ~/claude-sessions && git add -A && git commit -m "docs: update architecture docs for component refactor"
git push origin main
```
