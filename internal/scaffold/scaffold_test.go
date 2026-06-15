package scaffold_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/ipaqsa/kube2e/internal/engine/suite"
	"github.com/ipaqsa/kube2e/internal/scaffold"
	"github.com/ipaqsa/kube2e/internal/template"
)

func TestCreate(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()

	suiteDir, err := scaffold.Create(scaffold.Config{Dir: parent, Name: "demo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if suiteDir != filepath.Join(parent, "demo") {
		t.Fatalf("suiteDir = %q, want %q", suiteDir, filepath.Join(parent, "demo"))
	}

	for _, rel := range []string{"cases/example.yaml", "templates/configmap.yaml"} {
		if _, err := os.Stat(filepath.Join(suiteDir, rel)); err != nil {
			t.Fatalf("expected %q to exist: %v", rel, err)
		}
	}
}

func TestCreateExistingDirFails(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()

	if _, err := scaffold.Create(scaffold.Config{Dir: parent, Name: "demo"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	if _, err := scaffold.Create(scaffold.Config{Dir: parent, Name: "demo"}); err == nil {
		t.Fatal("expected error when suite directory already exists, got nil")
	}
}

func TestCreateEmptyNameFails(t *testing.T) {
	t.Parallel()

	if _, err := scaffold.Create(scaffold.Config{Dir: t.TempDir(), Name: ""}); err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// TestScaffoldedOutputIsValid guards the generated content: the case must parse
// strictly into the public Case type and the template must compile.
func TestScaffoldedOutputIsValid(t *testing.T) {
	t.Parallel()

	suiteDir, err := scaffold.Create(scaffold.Config{Dir: t.TempDir(), Name: "demo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(suiteDir, "cases", "example.yaml")) //nolint:gosec // test reads a file it just created under t.TempDir()
	if err != nil {
		t.Fatalf("read case: %v", err)
	}

	var c suite.Case
	if err := yaml.UnmarshalStrict(content, &c); err != nil {
		t.Fatalf("scaffolded case does not parse strictly: %v", err)
	}

	if c.Version != "v1" {
		t.Fatalf("case version = %q, want v1", c.Version)
	}

	logger := slog.New(slog.DiscardHandler)
	if _, err := template.NewManager(filepath.Join(suiteDir, "templates"), logger); err != nil {
		t.Fatalf("scaffolded template does not compile: %v", err)
	}
}
