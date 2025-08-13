// Package testcase parses and executes individual test cases.
package testcase

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/ipaqsa/kube2e/internal/engine/step"
	interrors "github.com/ipaqsa/kube2e/internal/errors"
)

const (
	caseAnnotation = "testing.kube2e.io/case"
)

// Case holds the parsed contents of a case YAML file, including all steps to execute.
type Case struct {
	Path string `yaml:"path" json:"path"`

	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`

	// Objects maps a resource name to its template base-filename (without .yaml).
	// The key becomes the Kubernetes object name injected into every render.
	// Steps reference an entry by name; Ensure is the only action that uses Values.
	Objects map[string]string `yaml:"objects" json:"objects"`

	Steps []*step.Step `yaml:"steps" json:"steps"`

	Labels      map[string]string `yaml:"labels" json:"labels"`
	Annotations map[string]string `yaml:"annotations" json:"annotations"`
}

// parseCaseFile reads a case configuration from disk.
func parseCaseFile(path string) (*Case, error) {
	content, err := os.ReadFile(path) //nolint:gosec // path comes from trusted test configuration, not user input
	if err != nil {
		return nil, fmt.Errorf("read case file: %w", err)
	}

	testcase := new(Case)
	if err = yaml.Unmarshal(content, testcase); err != nil {
		return nil, fmt.Errorf("unmarshal case file: %w", err)
	}

	if testcase.Name == "" {
		return nil, interrors.ErrCaseNoName
	}

	if len(testcase.Annotations) == 0 {
		testcase.Annotations = make(map[string]string)
	}

	testcase.Annotations[caseAnnotation] = testcase.Name

	testcase.Path = path

	return testcase, nil
}
