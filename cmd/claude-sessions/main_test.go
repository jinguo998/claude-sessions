package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jinguo998/claude-sessions/internal/domain"
	platformshell "github.com/jinguo998/claude-sessions/internal/platform/shell"
)

func captureMainOutput(t *testing.T, args ...string) string {
	t.Helper()

	oldArgs := os.Args
	oldStdout := os.Stdout
	defer func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
	}()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}

	os.Args = append([]string{"claude-sessions"}, args...)
	os.Stdout = w

	main()

	if err := w.Close(); err != nil {
		t.Fatalf("stdout close error = %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}
	return buf.String()
}

func TestMainInitShellOutput(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{shell: "zsh", want: zshInit},
		{shell: "bash", want: bashInit},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			out := captureMainOutput(t, "init", tt.shell)
			if out != tt.want {
				t.Fatalf("main init output = %q, want %q", out, tt.want)
			}
			if !strings.Contains(out, "cs() {") || !strings.Contains(out, `command claude-sessions "$@"`) {
				t.Fatalf("init output missing shell wrapper: %q", out)
			}

			tmp := filepath.Join(t.TempDir(), "init."+tt.shell)
			if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			if err := exec.Command(tt.shell, "-n", tmp).Run(); err != nil {
				t.Fatalf("%s -n rejected init output: %v", tt.shell, err)
			}
		})
	}
}

func TestZshPickerDefersResumeUntilAfterZLE(t *testing.T) {
	out := captureMainOutput(t, "init", "zsh")
	for _, forbidden := range []string{"</dev/tty", "<>/dev/tty"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("zsh picker must not redirect through %q:\n%s", forbidden, out)
		}
	}
	if strings.Contains(out, `BUFFER="cs __exec-handoff`) {
		t.Fatalf("zsh picker must not expose the handoff command through BUFFER:\n%s", out)
	}
	for _, want := range []string{
		"pick --handoff",
		"zle push-input",
		"add-zsh-hook precmd",
		"typeset -g _claude_sessions_pending_handoff",
		`BUFFER=""`,
		"zle accept-line",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("zsh picker missing %q:\n%s", want, out)
		}
	}
}

func TestPickHandoffPath(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "direct picker"},
		{name: "handoff", args: []string{"--handoff", "/tmp/resume.json"}, want: "/tmp/resume.json"},
		{name: "missing path", args: []string{"--handoff"}, wantErr: true},
		{name: "empty path", args: []string{"--handoff", ""}, wantErr: true},
		{name: "unknown flag", args: []string{"--other", "value"}, wantErr: true},
		{name: "extra argument", args: []string{"--handoff", "/tmp/resume.json", "extra"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickHandoffPath(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("pickHandoffPath(%q) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("pickHandoffPath(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestExecuteOrHandoff(t *testing.T) {
	plan := domain.ResumePlan{Executable: "codex", Args: []string{"codex", "resume", "session-id"}}

	t.Run("handoff does not exec", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resume.json")
		execCalls := 0
		err := executeOrHandoff(plan, path, func(domain.ResumePlan) error {
			execCalls++
			return nil
		})
		if err != nil {
			t.Fatalf("executeOrHandoff() error = %v", err)
		}
		if execCalls != 0 {
			t.Fatalf("exec calls = %d, want 0", execCalls)
		}
		got, err := platformshell.ConsumePlan(path)
		if err != nil {
			t.Fatalf("ConsumePlan() error = %v", err)
		}
		if !reflect.DeepEqual(got, plan) {
			t.Fatalf("handoff plan = %#v, want %#v", got, plan)
		}
	})

	t.Run("direct picker execs", func(t *testing.T) {
		execCalls := 0
		err := executeOrHandoff(plan, "", func(got domain.ResumePlan) error {
			execCalls++
			if !reflect.DeepEqual(got, plan) {
				t.Fatalf("exec plan = %#v, want %#v", got, plan)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("executeOrHandoff() error = %v", err)
		}
		if execCalls != 1 {
			t.Fatalf("exec calls = %d, want 1", execCalls)
		}
	})
}

func TestMainVersionOutput(t *testing.T) {
	oldVersion := version
	defer func() { version = oldVersion }()
	version = "v1.2.3-test"

	for _, arg := range []string{"version", "--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			out := captureMainOutput(t, arg)
			if out != "claude-sessions v1.2.3-test\n" {
				t.Fatalf("version output = %q", out)
			}
		})
	}
}

func TestNonInteractiveCommandsDoNotProbeTerminalAtPackageInit(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "claude-sessions")
	if err := exec.Command("go", "build", "-o", exe, ".").Run(); err != nil {
		t.Fatalf("go build test binary failed: %v", err)
	}

	tests := [][]string{
		{"--version"},
		{"init", "zsh"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, args...)
			out, err := cmd.CombinedOutput()
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("%s timed out; output=%q", strings.Join(args, " "), out)
			}
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, out)
			}
			if len(out) == 0 {
				t.Fatalf("%s produced no output", strings.Join(args, " "))
			}
		})
	}
}

func TestDisplayVersionFallsBackToDev(t *testing.T) {
	oldVersion := version
	defer func() { version = oldVersion }()
	version = "dev"

	if got := displayVersion(); got == "" {
		t.Fatal("displayVersion() should not be empty")
	}
}

func TestResolveShellConfig(t *testing.T) {
	home := t.TempDir()

	t.Run("zsh", func(t *testing.T) {
		cf, el, err := resolveShellConfig("zsh", home)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(cf, ".zshrc") {
			t.Errorf("expected .zshrc, got %s", cf)
		}
		if !strings.Contains(el, "init zsh") {
			t.Errorf("expected eval line for zsh, got %s", el)
		}
	})

	t.Run("bash with bashrc", func(t *testing.T) {
		// Create .bashrc so it doesn't fall through
		bashrc := filepath.Join(home, ".bashrc")
		os.WriteFile(bashrc, []byte("# existing"), 0644)

		cf, el, err := resolveShellConfig("bash", home)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(cf, ".bashrc") {
			t.Errorf("expected .bashrc, got %s", cf)
		}
		if !strings.Contains(el, "init bash") {
			t.Errorf("expected eval line for bash, got %s", el)
		}
	})

	t.Run("bash fallback to bash_profile", func(t *testing.T) {
		emptyHome := t.TempDir() // no .bashrc here
		cf, _, err := resolveShellConfig("bash", emptyHome)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(cf, ".bash_profile") {
			t.Errorf("expected .bash_profile fallback, got %s", cf)
		}
	})

	t.Run("unsupported shell", func(t *testing.T) {
		_, _, err := resolveShellConfig("fish", home)
		if err == nil {
			t.Fatal("expected error for unsupported shell")
		}
		if !strings.Contains(err.Error(), "unsupported") {
			t.Errorf("expected unsupported error, got %v", err)
		}
	})
}

func TestInstallShellInitTo(t *testing.T) {
	t.Run("fresh install", func(t *testing.T) {
		configFile := filepath.Join(t.TempDir(), ".zshrc")
		evalLine := `eval "$(claude-sessions init zsh)"`

		res, err := installShellInitTo(configFile, evalLine)
		if err != nil {
			t.Fatal(err)
		}
		if res.AlreadyInstalled {
			t.Error("expected fresh install, got already installed")
		}

		content, _ := os.ReadFile(configFile)
		if !strings.Contains(string(content), evalLine) {
			t.Errorf("config file should contain eval line, got: %s", content)
		}
		if !strings.Contains(string(content), "# claude-sessions shell integration") {
			t.Error("config file should contain comment header")
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		configFile := filepath.Join(t.TempDir(), ".zshrc")
		evalLine := `eval "$(claude-sessions init zsh)"`

		// First install
		_, err := installShellInitTo(configFile, evalLine)
		if err != nil {
			t.Fatal(err)
		}

		// Second install — should be idempotent
		res, err := installShellInitTo(configFile, evalLine)
		if err != nil {
			t.Fatal(err)
		}
		if !res.AlreadyInstalled {
			t.Error("expected already installed on second call")
		}

		// Verify only one copy
		content, _ := os.ReadFile(configFile)
		if strings.Count(string(content), evalLine) != 1 {
			t.Errorf("expected exactly 1 eval line, got: %s", content)
		}
	})

	t.Run("preserves existing content", func(t *testing.T) {
		configFile := filepath.Join(t.TempDir(), ".zshrc")
		existing := "# my existing config\nexport FOO=bar\n"
		os.WriteFile(configFile, []byte(existing), 0644)

		evalLine := `eval "$(claude-sessions init zsh)"`
		_, err := installShellInitTo(configFile, evalLine)
		if err != nil {
			t.Fatal(err)
		}

		content, _ := os.ReadFile(configFile)
		if !strings.HasPrefix(string(content), existing) {
			t.Error("existing content should be preserved")
		}
		if !strings.Contains(string(content), evalLine) {
			t.Error("eval line should be appended")
		}
	})
}
