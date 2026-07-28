package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jinguo998/claude-sessions/internal/domain"
)

func TestWriteAndConsumePlanRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.json")
	want := domain.ResumePlan{
		WorkingDir: "/tmp/project with spaces",
		Executable: "codex",
		Args:       []string{"codex", "resume", "session-id"},
		Message:    "resuming session",
	}

	if err := WritePlan(path, want); err != nil {
		t.Fatalf("WritePlan() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat handoff: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("handoff mode = %o, want 600", got)
	}

	got, err := ConsumePlan(path)
	if err != nil {
		t.Fatalf("ConsumePlan() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ConsumePlan() = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("handoff still exists after consume: %v", err)
	}
}

func TestWritePlanTightensExistingFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := domain.ResumePlan{Executable: "codex", Args: []string{"codex", "resume", "id"}}
	if err := WritePlan(path, plan); err != nil {
		t.Fatalf("WritePlan() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("handoff mode = %o, want 600", got)
	}
}

func TestConsumePlanRemovesMalformedHandoff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ConsumePlan(path); err == nil {
		t.Fatal("ConsumePlan() error = nil, want malformed JSON error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("malformed handoff still exists: %v", err)
	}
}

func TestConsumePlanRejectsIncompletePlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.json")
	if err := os.WriteFile(path, []byte(`{"executable":"codex"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ConsumePlan(path); err == nil {
		t.Fatal("ConsumePlan() error = nil, want incomplete plan error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("incomplete handoff still exists: %v", err)
	}
}
