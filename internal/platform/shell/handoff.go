package shell

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

// WritePlan stores a resume plan for a shell to execute after its interactive
// line editor has relinquished the terminal.
func WritePlan(path string, plan domain.ResumePlan) error {
	if err := validateHandoffPlan(plan); err != nil {
		return err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("encode resume handoff: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write resume handoff: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("secure resume handoff: %w", err)
	}
	return nil
}

// ConsumePlan reads and removes a resume plan before returning it. Removing
// first makes the handoff one-shot even when decoding or validation fails.
func ConsumePlan(path string) (domain.ResumePlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		_ = os.Remove(path)
		return domain.ResumePlan{}, fmt.Errorf("read resume handoff: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return domain.ResumePlan{}, fmt.Errorf("remove resume handoff: %w", err)
	}

	var plan domain.ResumePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return domain.ResumePlan{}, fmt.Errorf("decode resume handoff: %w", err)
	}
	if err := validateHandoffPlan(plan); err != nil {
		return domain.ResumePlan{}, err
	}
	return plan, nil
}

func validateHandoffPlan(plan domain.ResumePlan) error {
	if plan.Executable == "" {
		return fmt.Errorf("invalid resume handoff: executable is empty")
	}
	if len(plan.Args) == 0 || plan.Args[0] == "" {
		return fmt.Errorf("invalid resume handoff: argv is empty")
	}
	return nil
}
