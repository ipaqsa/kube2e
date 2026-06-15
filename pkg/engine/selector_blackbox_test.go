package engine_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipaqsa/kube2e/pkg/engine"
)

// writeSuite writes a single-case suite under a fresh work dir and returns the
// work dir to hand to RunTests.
func writeSuite(t *testing.T, caseYAML string) string {
	t.Helper()

	workDir := t.TempDir()
	casesDir := filepath.Join(workDir, "suite", "cases")

	if err := os.MkdirAll(casesDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(casesDir, "case.yaml"), []byte(caseYAML), 0o600); err != nil {
		t.Fatalf("write case: %v", err)
	}

	return workDir
}

func dryRun(t *testing.T, workDir string) error {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	_, err := engine.RunTests(context.Background(), &engine.Config{WorkDir: workDir, DryRun: true}, logger)

	return err
}

// TestSelectorTargetParses confirms a logs target using kind + labelSelector
// (no templated object) is accepted end-to-end.
func TestSelectorTargetParses(t *testing.T) {
	t.Parallel()

	workDir := writeSuite(t, `version: v1
name: sel
namespace: demo
steps:
  - name: inspect
    logs:
      target:
        kind: Pod
        labelSelector: app=demo
      contains: ready
`)

	if err := dryRun(t, workDir); err != nil {
		t.Fatalf("selector target should be valid, got: %v", err)
	}
}

// TestSelectorTargetRejectsObjectAndKind confirms object and kind are mutually
// exclusive.
func TestSelectorTargetRejectsObjectAndKind(t *testing.T) {
	t.Parallel()

	workDir := writeSuite(t, `version: v1
name: bad
namespace: demo
objects:
  app: cm
steps:
  - name: inspect
    exec:
      target:
        object: app
        kind: Pod
        labelSelector: app=demo
      command: ["true"]
`)

	err := dryRun(t, workDir)
	if err == nil {
		t.Fatal("expected validation error for object + kind, got nil")
	}

	if !strings.Contains(err.Error(), "must not be set together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestSelectorTargetRejectsKindWithoutLabelSelector confirms kind requires a
// label selector.
func TestSelectorTargetRejectsKindWithoutLabelSelector(t *testing.T) {
	t.Parallel()

	workDir := writeSuite(t, `version: v1
name: bad
namespace: demo
steps:
  - name: inspect
    exec:
      target:
        kind: Pod
      command: ["true"]
`)

	err := dryRun(t, workDir)
	if err == nil {
		t.Fatal("expected validation error for kind without labelSelector, got nil")
	}

	if !strings.Contains(err.Error(), "labelSelector") {
		t.Fatalf("unexpected error: %v", err)
	}
}
