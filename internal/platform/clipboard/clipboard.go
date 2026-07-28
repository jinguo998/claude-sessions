package clipboard

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/jinguo998/claude-sessions/internal/app/ports"
)

type System struct{}

var _ ports.Clipboard = System{}

func NewSystem() System {
	return System{}
}

func (System) Copy(text string) error {
	for _, cmd := range [][]string{
		{"pbcopy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	} {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Stdin = strings.NewReader(text)
		if err := c.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no clipboard command available")
}
