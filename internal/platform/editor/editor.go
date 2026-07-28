package editor

import (
	"os"
	"os/exec"

	"github.com/jinguo998/claude-sessions/internal/app/ports"
)

type System struct{}

var _ ports.Editor = System{}

func NewSystem() System {
	return System{}
}

func (System) Open(filePath string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	c := exec.Command(editor, filePath)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
