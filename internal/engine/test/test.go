// Package test orchestrates top-level test execution including namespace lifecycle.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/ipaqsa/kube2e/internal/errors"
)

const (
	testFile = "test.yaml"

	casesDir     = "cases"
	templatesDir = "templates"

	testAnnotation = "testing.kube2e.io/test"
)

// Test groups multiple cases into one scenario. CRDs must be provisioned by the
// caller before the test runs — kube2e does not manage CRD lifecycle.
type Test struct {
	// Path is the filesystem path to the test directory (set after parsing).
	Path string `yaml:"path" json:"path"`

	// Name is a required human-readable identifier for the test suite.
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`

	// Namespace is the default Kubernetes namespace for all objects in the test.
	Namespace string `yaml:"namespace" json:"namespace"`
	// Ignored lists case file basenames (without .yaml) that should be skipped.
	Ignored []string `yaml:"ignored" json:"ignored"`

	Labels      map[string]string `yaml:"labels" json:"labels"`
	Annotations map[string]string `yaml:"annotations" json:"annotations"`
}

// parseTestFile loads the test descriptor from the provided directory.
func parseTestFile(testDir string) (*Test, error) {
	content, err := os.ReadFile(filepath.Join(testDir, testFile)) //nolint:gosec // path comes from trusted test configuration, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read test file: %w", err)
	}

	test := new(Test)
	if err = yaml.Unmarshal(content, test); err != nil {
		return nil, fmt.Errorf("unmarshal test file: %w", err)
	}

	if test.Name == "" {
		return nil, errors.ErrTestNoName
	}

	if len(test.Annotations) == 0 {
		test.Annotations = make(map[string]string)
	}

	test.Annotations[testAnnotation] = test.Name

	test.Path = testDir

	return test, nil
}

// CasesDir returns the directory containing individual test case YAML files.
func (t *Test) CasesDir() string {
	return filepath.Join(t.Path, casesDir)
}

// TemplatesDir returns the directory containing Go templates used by steps.
func (t *Test) TemplatesDir() string {
	return filepath.Join(t.Path, templatesDir)
}

// forEach iterates over case YAML files in CasesDir and invokes f for each
// one that is not listed in t.Ignored. Ignored entries are matched against the
// base filename without the .yaml extension.
func (t *Test) forEach(f func(casePath string) error) error {
	cases, err := os.ReadDir(t.CasesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read the cases dir '%s': %w", t.CasesDir(), err)
	}

	ignoredSet := make(map[string]struct{}, len(t.Ignored))
	for _, name := range t.Ignored {
		ignoredSet[name] = struct{}{}
	}

	for _, testCase := range cases {
		if testCase.IsDir() || filepath.Ext(testCase.Name()) != ".yaml" {
			continue
		}

		caseName := strings.TrimSuffix(testCase.Name(), ".yaml")
		if _, skip := ignoredSet[caseName]; skip {
			continue
		}

		if err = f(filepath.Join(t.CasesDir(), testCase.Name())); err != nil {
			return err
		}
	}

	return nil
}
