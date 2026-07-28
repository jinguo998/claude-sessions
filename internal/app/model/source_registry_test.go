package model

import "testing"

func TestDefaultSourceRegistry(t *testing.T) {
	registry := DefaultSourceRegistry()

	claude := registry.Info(SourceClaude)
	if claude.Label != "Claude" || claude.Badge != "C" {
		t.Fatalf("Claude info = %#v", claude)
	}
	if claude.DefaultPermissionMode != PermissionModeFast {
		t.Fatalf("Claude default permission = %q, want %q", claude.DefaultPermissionMode, PermissionModeFast)
	}
	if !claude.SupportsSafeResumeAction {
		t.Fatal("Claude should expose safe resume action")
	}

	codex := registry.Info(SourceCodex)
	if codex.Label != "Codex" || codex.Badge != "X" {
		t.Fatalf("Codex info = %#v", codex)
	}
	if codex.DefaultPermissionMode != PermissionModeSafe {
		t.Fatalf("Codex default permission = %q, want %q", codex.DefaultPermissionMode, PermissionModeSafe)
	}
	if codex.SupportsSafeResumeAction {
		t.Fatal("Codex should not expose a separate safe resume action")
	}

	opencode := registry.Info(SourceOpenCode)
	if opencode.Label != "OpenCode" || opencode.Badge != "O" {
		t.Fatalf("OpenCode info = %#v", opencode)
	}
	if opencode.DefaultPermissionMode != PermissionModeSafe {
		t.Fatalf("OpenCode default permission = %q, want %q", opencode.DefaultPermissionMode, PermissionModeSafe)
	}
	if opencode.SupportsSafeResumeAction {
		t.Fatal("OpenCode should not expose a separate safe resume action")
	}
	if !opencode.SupportsFork {
		t.Fatal("OpenCode should support fork")
	}
	if opencode.SupportsArchive {
		t.Fatal("OpenCode should not support archive in the file-moving trash store")
	}
}

func TestSourceRegistryUnknownFallback(t *testing.T) {
	registry := NewSourceRegistry(nil)
	info := registry.Info(Source("future"))

	if info.Label != "future" || info.Badge != "?" {
		t.Fatalf("unknown info = %#v", info)
	}
	if info.DefaultPermissionMode != PermissionModeSafe {
		t.Fatalf("unknown default permission = %q, want %q", info.DefaultPermissionMode, PermissionModeSafe)
	}
}

func TestSourceRegistryAllReturnsCopy(t *testing.T) {
	registry := DefaultSourceRegistry()
	all := registry.All()
	all[0].Label = "changed"

	if got := registry.Info(SourceClaude).Label; got != "Claude" {
		t.Fatalf("registry mutated through All result, label = %q", got)
	}
}
