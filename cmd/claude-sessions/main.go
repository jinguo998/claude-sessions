package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	apparchive "github.com/jinguo998/claude-sessions/internal/app/archive"
	session "github.com/jinguo998/claude-sessions/internal/app/model"
	apppreview "github.com/jinguo998/claude-sessions/internal/app/preview"
	appresume "github.com/jinguo998/claude-sessions/internal/app/resume"
	appscan "github.com/jinguo998/claude-sessions/internal/app/scan"
	"github.com/jinguo998/claude-sessions/internal/domain"
	"github.com/jinguo998/claude-sessions/internal/platform/clipboard"
	"github.com/jinguo998/claude-sessions/internal/platform/editor"
	"github.com/jinguo998/claude-sessions/internal/platform/shell"
	"github.com/jinguo998/claude-sessions/internal/platform/terminal"
	"github.com/jinguo998/claude-sessions/internal/source"
	"github.com/jinguo998/claude-sessions/internal/source/claude"
	"github.com/jinguo998/claude-sessions/internal/source/codex"
	"github.com/jinguo998/claude-sessions/internal/source/opencode"
	"github.com/jinguo998/claude-sessions/internal/storage"
	"github.com/jinguo998/claude-sessions/internal/storage/cache"
	"github.com/jinguo998/claude-sessions/internal/storage/trash"
	"github.com/jinguo998/claude-sessions/internal/ui/picker"
	"github.com/jinguo998/claude-sessions/internal/ui/tui"
)

const zshInit = `# claude-sessions shell integration
cs() {
    command claude-sessions "$@"
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}

# Run pending picker handoffs after ZLE has released the terminal.
_claude_sessions_run_handoff() {
    local _cs_handoff="${_claude_sessions_pending_handoff:-}"
    if [[ -z "$_cs_handoff" ]]; then
        return 0
    fi
    unset _claude_sessions_pending_handoff
    cs __exec-handoff "$_cs_handoff"
}
autoload -Uz add-zsh-hook
add-zsh-hook precmd _claude_sessions_run_handoff

# Fuzzy session picker (ctrl+s)
_claude_sessions_pick() {
    local _cs_saved_buffer="$BUFFER"
    local _cs_saved_cursor="$CURSOR"
    local _cs_handoff
    local _cs_status
    _cs_handoff="$(mktemp "${TMPDIR:-/tmp}/claude-sessions-resume.XXXXXX")" || {
        zle reset-prompt
        return 1
    }
    command claude-sessions pick --handoff "$_cs_handoff"
    _cs_status=$?
    if (( _cs_status != 0 )) || [[ ! -s "$_cs_handoff" ]]; then
        rm -f "$_cs_handoff"
        BUFFER="$_cs_saved_buffer"
        CURSOR="$_cs_saved_cursor"
        zle reset-prompt
        return $_cs_status
    fi
    BUFFER="$_cs_saved_buffer"
    CURSOR="$_cs_saved_cursor"
    zle push-input
    typeset -g _claude_sessions_pending_handoff="$_cs_handoff"
    BUFFER=""
    CURSOR=0
    zle accept-line
}
zle -N _claude_sessions_pick
bindkey '^s' _claude_sessions_pick
`

const bashInit = `# claude-sessions shell integration
cs() {
    command claude-sessions "$@"
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}

# Fuzzy session picker (ctrl+s)
_claude_sessions_pick() {
    claude-sessions pick
    local _cs_cd="/tmp/claude-sessions-cd"
    if [ -f "$_cs_cd" ]; then
        cd "$(cat "$_cs_cd")" && rm -f "$_cs_cd"
    fi
}
bind -x '"\C-s": _claude_sessions_pick'
`

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("claude-sessions %s\n", displayVersion())
		return
	}

	// Handle "init" subcommand
	// - "init"          → auto-detect shell, install eval line into config file (idempotent)
	// - "init zsh/bash" → print shell script to stdout (for eval usage)
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if len(os.Args) > 2 {
			// Print mode: output script for eval
			switch os.Args[2] {
			case "zsh":
				fmt.Print(zshInit)
			case "bash":
				fmt.Print(bashInit)
			default:
				fmt.Fprintf(os.Stderr, "unsupported shell: %s (supported: zsh, bash)\n", os.Args[2])
				os.Exit(1)
			}
		} else {
			// Install mode: auto-detect shell and write to config
			installShellInit()
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "__exec-handoff" {
		if len(os.Args) != 3 || os.Args[2] == "" {
			fmt.Fprintln(os.Stderr, "usage: claude-sessions __exec-handoff <path>")
			os.Exit(2)
		}
		plan, err := shell.ConsumePlan(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "resume handoff failed: %v\n", err)
			os.Exit(1)
		}
		if err := shell.ExecPlan(plan); err != nil {
			fmt.Fprintf(os.Stderr, "exec failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle "pick" subcommand — fzf-style fuzzy picker
	if len(os.Args) > 1 && os.Args[1] == "pick" {
		handoffPath, err := pickHandoffPath(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "pick: %v\n", err)
			os.Exit(2)
		}
		runtime := newRuntime()
		scanResult := runtime.scan.Scan(context.Background())
		sessions := scanResult.Sessions
		if len(sessions) == 0 {
			if scanResult.Err != nil {
				fmt.Fprintf(os.Stderr, "scan error: %v\n", scanResult.Err)
				os.Exit(1)
			}
			for _, warning := range scanResult.Warnings {
				fmt.Fprintf(os.Stderr, "scan warning [%s:%s]: %s\n", warning.Kind, warning.Source, warning.Message)
			}
			fmt.Fprintln(os.Stderr, "no sessions found")
			return
		}

		m := picker.NewModel(sessions, runtime.tui.Sources)
		p := tea.NewProgram(m, tea.WithMouseCellMotion())

		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		result := finalModel.(picker.Model).PickResult()
		if result.Cancelled || result.Session == nil {
			return
		}

		sess := *result.Session
		target := domain.ResumeTarget{
			Session:        sess.Domain(),
			Action:         domain.ResumeActionResume,
			PermissionMode: runtime.tui.Sources.DefaultPermissionMode(sess.Source),
		}
		execResumeTarget(runtime.resume, target, handoffPath)
		return
	}

	runtime := newRuntime()
	p := tea.NewProgram(tui.InitialModel(runtime.tui), tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	m, ok := finalModel.(tui.Model)
	if !ok {
		return
	}
	result := m.Result()
	if result.ID == "" {
		return
	}

	execResumeTarget(runtime.resume, targetFromTUIResult(result), "")
}

func pickHandoffPath(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) != 2 || args[0] != "--handoff" || args[1] == "" {
		return "", fmt.Errorf("usage: claude-sessions pick [--handoff <path>]")
	}
	return args[1], nil
}

func displayVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return version
}

type runtimeServices struct {
	scan   *appscan.Repository
	resume *appresume.Service
	tui    tui.Services
}

type runtimeSource struct {
	Info    session.SourceInfo
	Scanner source.Scanner
	Preview source.PreviewParser
	Resume  source.ResumePlanner
	Archive source.ArchiveSpecifier
}

func newRuntime() runtimeServices {
	tui.ConfigureTerminalTheme(terminal.MarkdownStyle())

	claudeAdapter := claude.NewAdapter()
	codexAdapter := codex.NewAdapter()
	openCodeAdapter := opencode.NewAdapter()
	sources := []runtimeSource{
		{
			Info: session.SourceInfo{
				Source:                   session.SourceClaude,
				Label:                    "Claude",
				Badge:                    "C",
				LightColor:               "27",
				DarkColor:                "75",
				DefaultPermissionMode:    session.PermissionModeFast,
				SupportsSafeResumeAction: true,
				SupportsFork:             true,
				SupportsArchive:          true,
			},
			Scanner: claudeAdapter,
			Preview: claudeAdapter,
			Resume:  claudeAdapter,
			Archive: claudeAdapter,
		},
		{
			Info: session.SourceInfo{
				Source:                   session.SourceCodex,
				Label:                    "Codex",
				Badge:                    "X",
				LightColor:               "22",
				DarkColor:                "40",
				DefaultPermissionMode:    session.PermissionModeSafe,
				SupportsSafeResumeAction: false,
				SupportsFork:             true,
				SupportsArchive:          true,
			},
			Scanner: codexAdapter,
			Preview: codexAdapter,
			Resume:  codexAdapter,
			Archive: codexAdapter,
		},
		{
			Info: session.SourceInfo{
				Source:                   session.SourceOpenCode,
				Label:                    "OpenCode",
				Badge:                    "O",
				LightColor:               "88",
				DarkColor:                "204",
				DefaultPermissionMode:    session.PermissionModeSafe,
				SupportsSafeResumeAction: false,
				SupportsFork:             true,
				SupportsArchive:          false,
			},
			Scanner: openCodeAdapter,
			Preview: openCodeAdapter,
			Resume:  openCodeAdapter,
		},
	}
	sourceInfos := make([]session.SourceInfo, 0, len(sources))
	scanners := make([]source.Scanner, 0, len(sources))
	previewParsers := make([]source.PreviewParser, 0, len(sources))
	resumePlanners := make([]source.ResumePlanner, 0, len(sources))
	archiveSpecifiers := make([]source.ArchiveSpecifier, 0, len(sources))
	for _, src := range sources {
		sourceInfos = append(sourceInfos, src.Info)
		if src.Scanner != nil {
			scanners = append(scanners, src.Scanner)
		}
		if src.Preview != nil {
			previewParsers = append(previewParsers, src.Preview)
		}
		if src.Resume != nil {
			resumePlanners = append(resumePlanners, src.Resume)
		}
		if src.Archive != nil {
			archiveSpecifiers = append(archiveSpecifiers, src.Archive)
		}
	}
	sourceRegistry := session.NewSourceRegistry(sourceInfos)

	metadataCache := cache.New()
	trashStore, err := trash.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "trash unavailable: %v\n", err)
	}
	var archiveStore storage.TrashStore
	if trashStore != nil {
		archiveStore = trashStore
	}
	scanRepo := appscan.NewRepository(scanners, metadataCache)
	previewSvc := apppreview.NewService(previewParsers)
	resumeSvc := appresume.NewService(resumePlanners)
	archiveSvc := apparchive.NewService(archiveSpecifiers, archiveStore)
	tuiServices := tui.Services{
		Scan:      scanRepo,
		Preview:   previewSvc,
		Archive:   archiveSvc,
		Clipboard: clipboard.NewSystem(),
		Editor:    editor.NewSystem(),
		Sources:   sourceRegistry,
	}
	return runtimeServices{scan: scanRepo, resume: resumeSvc, tui: tuiServices}
}

func targetFromTUIResult(result tui.Result) domain.ResumeTarget {
	action := domain.ResumeActionResume
	if result.Fork {
		action = domain.ResumeActionFork
	}
	if result.CdOnly {
		action = domain.ResumeActionCd
	}
	mode := result.PermissionMode
	if mode == "" {
		mode = domain.PermissionModeSafe
	}
	return domain.ResumeTarget{
		Session: domain.Session{
			ID:          result.ID,
			Source:      domain.Source(result.Source),
			ProjectPath: result.Dir,
		},
		Action:         action,
		PermissionMode: mode,
	}
}

func execResumeTarget(service *appresume.Service, target domain.ResumeTarget, handoffPath string) {
	plan, err := service.Plan(context.Background(), target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resume plan failed: %v\n", err)
		os.Exit(1)
	}
	if err := executeOrHandoff(plan, handoffPath, shell.ExecPlan); err != nil {
		fmt.Fprintf(os.Stderr, "exec failed: %v\n", err)
		os.Exit(1)
	}
}

func executeOrHandoff(plan domain.ResumePlan, handoffPath string, execPlan func(domain.ResumePlan) error) error {
	if handoffPath != "" {
		return shell.WritePlan(handoffPath, plan)
	}
	return execPlan(plan)
}

const evalMarker = `claude-sessions init`

// shellInitResult describes what installShellInitTo did.
type shellInitResult struct {
	AlreadyInstalled bool
	ConfigFile       string
	EvalLine         string
}

// resolveShellConfig returns the config file path and eval line for a given shell and home dir.
// Returns an error for unsupported shells.
func resolveShellConfig(shell, home string) (configFile, evalLine string, err error) {
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), `eval "$(claude-sessions init zsh)"`, nil
	case "bash":
		rc := filepath.Join(home, ".bashrc")
		if _, err := os.Stat(rc); os.IsNotExist(err) {
			rc = filepath.Join(home, ".bash_profile")
		}
		return rc, `eval "$(claude-sessions init bash)"`, nil
	default:
		return "", "", fmt.Errorf("unsupported shell: %s (supported: zsh, bash)", shell)
	}
}

// installShellInitTo appends the eval line to configFile if not already present.
// Returns the result describing what happened. Idempotent.
func installShellInitTo(configFile, evalLine string) (shellInitResult, error) {
	res := shellInitResult{ConfigFile: configFile, EvalLine: evalLine}

	existing, err := os.ReadFile(configFile)
	if err == nil && strings.Contains(string(existing), evalMarker) {
		res.AlreadyInstalled = true
		return res, nil
	}

	f, err := os.OpenFile(configFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return res, fmt.Errorf("failed to open %s: %w", configFile, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "\n# claude-sessions shell integration\n%s\n", evalLine); err != nil {
		return res, fmt.Errorf("failed to write: %w", err)
	}

	return res, nil
}

// installShellInit detects the user's shell and appends an eval line to
// the appropriate config file. Idempotent: skips if the line already exists.
func installShellInit() {
	log := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}

	shell := filepath.Base(os.Getenv("SHELL"))
	log("Detected shell: %s", shell)

	configFile, evalLine, err := resolveShellConfig(shell, os.Getenv("HOME"))
	if err != nil {
		log("%v", err)
		os.Exit(1)
	}
	log("Config file: %s", configFile)

	res, err := installShellInitTo(configFile, evalLine)
	if err != nil {
		log("%v", err)
		os.Exit(1)
	}

	if res.AlreadyInstalled {
		log("Already installed — skipping")
	} else {
		log("Done!")
		log("")
		log("Added to %s:", configFile)
		log("  %s", evalLine)
	}

	log("")
	log("Features:")
	log("  cs               open session browser (alias for claude-sessions)")
	log("  ctrl+s           fuzzy session picker")

	if !res.AlreadyInstalled {
		log("")
		log("Run to activate: source %s", configFile)
	}
}
