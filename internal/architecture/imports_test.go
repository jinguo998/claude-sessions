package architecture

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
)

type goPackage struct {
	ImportPath string
	Imports    []string
}

func TestInternalImportBoundaries(t *testing.T) {
	module := strings.TrimSpace(run(t, "go", "list", "-m"))
	out := run(t, "go", "list", "-json", "./...")

	dec := json.NewDecoder(strings.NewReader(out))
	for {
		var pkg goPackage
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		checkPackageImports(t, module, pkg)
	}
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func checkPackageImports(t *testing.T, module string, pkg goPackage) {
	t.Helper()

	internalPrefix := module + "/internal/"
	if !strings.HasPrefix(pkg.ImportPath, internalPrefix) {
		return
	}
	if strings.HasPrefix(pkg.ImportPath, internalPrefix+"architecture") {
		return
	}
	if strings.HasPrefix(pkg.ImportPath, internalPrefix+"smoke") {
		return
	}

	area := strings.TrimPrefix(pkg.ImportPath, internalPrefix)
	for _, imp := range pkg.Imports {
		internal := internalImport(module, imp)
		if internal == "" {
			if strings.HasPrefix(area, "domain") && !isStdlib(imp) {
				t.Errorf("%s imports non-stdlib dependency %s", pkg.ImportPath, imp)
			}
			continue
		}

		if !allowedInternalImport(area, internal) {
			t.Errorf("%s must not import internal/%s", pkg.ImportPath, internal)
		}
	}
}

func internalImport(module, imp string) string {
	prefix := module + "/internal/"
	if strings.HasPrefix(imp, prefix) {
		return strings.TrimPrefix(imp, prefix)
	}
	return ""
}

func isStdlib(imp string) bool {
	return !strings.Contains(imp, ".")
}

func allowedInternalImport(area, imp string) bool {
	switch {
	case strings.HasPrefix(area, "domain"):
		return false
	case area == "source" || strings.HasPrefix(area, "source/"):
		return imp == "domain" || strings.HasPrefix(imp, "domain/") || imp == "source"
	case area == "storage" || strings.HasPrefix(area, "storage/"):
		return imp == "domain" || strings.HasPrefix(imp, "domain/") || imp == "storage"
	case area == "app/ports":
		return imp == "domain" || strings.HasPrefix(imp, "domain/")
	case area == "app/model":
		return imp == "domain" || strings.HasPrefix(imp, "domain/")
	case strings.HasPrefix(area, "app/"):
		return imp == "domain" ||
			strings.HasPrefix(imp, "domain/") ||
			imp == "source" ||
			imp == "storage" ||
			imp == "app/model" ||
			imp == "app/ports"
	case strings.HasPrefix(area, "platform/"):
		return imp == "domain" ||
			strings.HasPrefix(imp, "domain/") ||
			imp == "app/ports"
	case strings.HasPrefix(area, "ui/"):
		return imp == "domain" ||
			strings.HasPrefix(imp, "domain/") ||
			imp == "app" ||
			strings.HasPrefix(imp, "app/")
	default:
		return false
	}
}

func TestNoLegacyInternalPackages(t *testing.T) {
	module := strings.TrimSpace(run(t, "go", "list", "-m"))
	out := run(t, "go", "list", "./...")

	legacy := map[string]bool{
		module + "/internal/session": true,
		module + "/internal/scanner": true,
		module + "/internal/parser":  true,
		module + "/internal/tui":     true,
		module + "/internal/picker":  true,
	}
	for _, pkg := range strings.Fields(out) {
		if legacy[pkg] {
			t.Fatalf("legacy package still exists: %s", pkg)
		}
	}
}

func TestBoundaryRuleExamples(t *testing.T) {
	cases := []struct {
		area string
		imp  string
		ok   bool
	}{
		{area: "ui/tui", imp: "app/scan", ok: true},
		{area: "ui/tui", imp: "source/claude", ok: false},
		{area: "app/scan", imp: "source", ok: true},
		{area: "app/scan", imp: "source/claude", ok: false},
		{area: "platform/shell", imp: "app/ports", ok: true},
		{area: "platform/shell", imp: "ui/tui", ok: false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s->%s", tc.area, tc.imp), func(t *testing.T) {
			if got := allowedInternalImport(tc.area, tc.imp); got != tc.ok {
				t.Fatalf("allowedInternalImport() = %v, want %v", got, tc.ok)
			}
		})
	}
}
