package smoke

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestZshPickerResumeGetsTTYInTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	claudeSessions := os.Getenv("CLAUDE_SESSIONS_SMOKE_BINARY")
	if claudeSessions == "" {
		claudeSessions = filepath.Join(binDir, "claude-sessions")
		cmd := exec.Command("go", "build", "-o", claudeSessions, "./cmd/claude-sessions")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build claude-sessions: %v\n%s", err, out)
		}
	}

	probePath := filepath.Join(tmp, "codex-probe.txt")
	bufferProbePath := filepath.Join(tmp, "buffer-probe.txt")
	fakeCodex := filepath.Join(binDir, "codex")
	fakeScript := `#!/bin/sh
if [ -t 0 ]; then
  stdin=tty
else
  stdin=notty
fi
{
  printf 'stdin=%s\n' "$stdin"
  printf 'args='
  printf '%s ' "$@"
  printf '\n'
} > "$PROBE_FILE"
`
	if err := os.WriteFile(fakeCodex, []byte(fakeScript), 0o755); err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	sessionsDir := filepath.Join(home, ".codex", "sessions", "2026", "07", "10")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	const sessionID = "11111111-2222-4333-8444-555555555555"
	sessionPath := filepath.Join(sessionsDir, "rollout-2026-07-10T00-00-00-"+sessionID+".jsonl")
	writeCodexFixture(t, sessionPath, sessionID, project)

	socket := fmt.Sprintf("claude-sessions-test-%d", time.Now().UnixNano())
	tmux := func(args ...string) *exec.Cmd {
		return exec.Command("tmux", append([]string{"-L", socket}, args...)...)
	}
	t.Cleanup(func() {
		_ = tmux("kill-server").Run()
	})

	pathEnv := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	start := tmux("new-session", "-d", "-s", "picker", "-x", "120", "-y", "30", "/bin/zsh", "-f")
	start.Env = append(os.Environ(), "HOME="+home, "PATH="+pathEnv, "PROBE_FILE="+probePath, "TERM=xterm-256color")
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start tmux: %v\n%s", err, out)
	}

	setupPath := filepath.Join(tmp, "setup.zsh")
	setupScript := fmt.Sprintf("export HOME=%q\nexport PATH=%q\nexport PROBE_FILE=%q\nstty -ixon\neval \"$(%q init zsh)\"\nprint -r -- INIT_READY\n", home, pathEnv, probePath, claudeSessions)
	if err := os.WriteFile(setupPath, []byte(setupScript), 0o600); err != nil {
		t.Fatal(err)
	}
	initCommand := fmt.Sprintf("source %q", setupPath)
	if out, err := tmux("send-keys", "-t", "picker", "-l", initCommand).CombinedOutput(); err != nil {
		t.Fatalf("send init command: %v\n%s", err, out)
	}
	if out, err := tmux("send-keys", "-t", "picker", "Enter").CombinedOutput(); err != nil {
		t.Fatalf("submit init command: %v\n%s", err, out)
	}
	waitForTmuxText(t, tmux, "picker", "INIT_READY", 10*time.Second)

	savedBufferCommand := fmt.Sprintf("print -r -- BUFFER_RESTORED > %q", bufferProbePath)
	if out, err := tmux("send-keys", "-t", "picker", "-l", savedBufferCommand).CombinedOutput(); err != nil {
		t.Fatalf("seed ZLE buffer: %v\n%s", err, out)
	}
	if out, err := tmux("send-keys", "-t", "picker", "C-s").CombinedOutput(); err != nil {
		t.Fatalf("open picker: %v\n%s", err, out)
	}
	waitForTmuxText(t, tmux, "picker", "tmux tty fixture", 10*time.Second)
	if out, err := tmux("send-keys", "-t", "picker", "Enter").CombinedOutput(); err != nil {
		t.Fatalf("select session: %v\n%s", err, out)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(probePath); err == nil {
			got := string(data)
			if !strings.Contains(got, "stdin=tty") {
				t.Fatalf("resumed Codex stdin is not a TTY:\n%s\npane:\n%s", got, captureTmuxPane(t, tmux, "picker"))
			}
			if !strings.Contains(got, "args=resume "+sessionID) {
				t.Fatalf("unexpected Codex arguments:\n%s", got)
			}
			pane := captureTmuxPane(t, tmux, "picker")
			for _, leaked := range []string{"__exec-handoff", "claude-sessions-resume."} {
				if strings.Contains(pane, leaked) {
					t.Fatalf("internal handoff %q leaked into the terminal:\n%s", leaked, pane)
				}
			}
			time.Sleep(100 * time.Millisecond)
			if out, err := tmux("send-keys", "-t", "picker", "Enter").CombinedOutput(); err != nil {
				t.Fatalf("submit restored ZLE buffer: %v\n%s", err, out)
			}
			waitForFileText(t, bufferProbePath, "BUFFER_RESTORED", 5*time.Second)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Codex probe was not written; pane:\n%s", captureTmuxPane(t, tmux, "picker"))
}

func waitForFileText(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("file %s did not contain %q", path, want)
}

func writeCodexFixture(t *testing.T, path, id, project string) {
	t.Helper()
	lines := []map[string]any{
		{
			"timestamp": "2026-07-10T00:00:00Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id": id, "cwd": project, "originator": "codex_cli_rs", "cli_version": "test", "source": "cli",
			},
		},
		{
			"timestamp": "2026-07-10T00:00:01Z",
			"type":      "event_msg",
			"payload":   map[string]any{"type": "user_message", "message": "tmux tty fixture"},
		},
	}
	var content strings.Builder
	for _, line := range lines {
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		content.Write(data)
		content.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForTmuxText(t *testing.T, tmux func(...string) *exec.Cmd, target, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(captureTmuxPane(t, tmux, target), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("tmux pane did not contain %q:\n%s", want, captureTmuxPane(t, tmux, target))
}

func captureTmuxPane(t *testing.T, tmux func(...string) *exec.Cmd, target string) string {
	t.Helper()
	out, err := tmux("capture-pane", "-p", "-t", target, "-S", "-100").CombinedOutput()
	if err != nil {
		t.Fatalf("capture tmux pane: %v\n%s", err, out)
	}
	return string(out)
}
