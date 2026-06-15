// Package test orchestrates top-level test execution.
package test

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	casesDir     = "cases"
	templatesDir = "templates"

	testAnnotation = "testing.kube2e.io/test"
)

// Test represents a single test suite derived from a directory.
// The suite name equals the directory's base name; no descriptor file is required.
type Test struct {
	// Path is the filesystem path to the test directory.
	Path string
	// Name is the test suite name, derived from the directory base name.
	Name string
}

// newTest constructs a Test from the given directory path.
func newTest(dir string) *Test {
	return &Test{
		Path: dir,
		Name: filepath.Base(dir),
	}
}

// CasesDir returns the directory containing individual test case YAML files.
func (t *Test) CasesDir() string {
	return filepath.Join(t.Path, casesDir)
}

// TemplatesDir returns the directory containing Go templates used by steps.
// The directory is optional; its absence is not an error.
func (t *Test) TemplatesDir() string {
	return filepath.Join(t.Path, templatesDir)
}

// forEach iterates over case YAML files in CasesDir in alphabetical filename order.
func (t *Test) forEach(f func(total, idx int, casePath string) error) error {
	entries, err := os.ReadDir(t.CasesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read the cases dir '%s': %w", t.CasesDir(), err)
	}

	// Pre-filter to case files so total and idx reflect only executed cases,
	// not directories or non-yaml entries. os.ReadDir returns sorted names, so
	// alphabetical order is preserved.
	var cases []string

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		cases = append(cases, entry.Name())
	}

	for idx, name := range cases {
		path := filepath.Join(t.CasesDir(), name)

		if err = f(len(cases), idx+1, path); err != nil {
			return err
		}
	}

	return nil
}
