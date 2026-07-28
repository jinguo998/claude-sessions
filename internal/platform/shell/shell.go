package shell

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

const CdHandoffFile = "/tmp/claude-sessions-cd"

func CdAndHandoff(dir string) error {
	fmt.Fprintf(os.Stderr, "\033[2m→ cd %s\033[0m\n", dir)
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("cd failed: %w", err)
	}
	if err := os.WriteFile(CdHandoffFile, []byte(dir), 0o644); err != nil {
		return err
	}
	return nil
}

func ExecPlan(plan domain.ResumePlan) error {
	if plan.WorkingDir != "" {
		if err := CdAndHandoff(plan.WorkingDir); err != nil {
			return err
		}
	}
	if plan.Message != "" {
		fmt.Fprintf(os.Stderr, "\033[2m→ %s\033[0m\n", plan.Message)
	}
	if plan.CdOnly {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
		shellPath, err := exec.LookPath(shell)
		if err != nil {
			return fmt.Errorf("shell not found: %w", err)
		}
		return syscall.Exec(shellPath, []string{shell}, os.Environ())
	}
	binPath, err := exec.LookPath(plan.Executable)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", plan.Executable, err)
	}
	return syscall.Exec(binPath, plan.Args, os.Environ())
}
