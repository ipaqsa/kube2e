package test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"sigs.k8s.io/yaml"
)

const (
	testFile = "test.yaml"

	crdsDir  = "crds"
	casesDir = "cases"
)

type Test struct {
	Path        string            `yaml:"path" json:"path"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Namespace   string            `yaml:"namespace" json:"namespace"`
	Ignored     []string          `yaml:"ignored" json:"ignored"`
	Labels      map[string]string `yaml:"labels" json:"labels"`
	Annotations map[string]string `yaml:"annotations" json:"annotations"`
}

func parseTestFile(testDir string) (*Test, error) {
	content, err := os.ReadFile(filepath.Join(testDir, testFile))
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
		return nil, fmt.Errorf("test has no name")
	}

	test.Path = testDir

	return test, nil
}

func (t *Test) CRDsDir() string {
	return filepath.Join(t.Path, crdsDir)
}

func (t *Test) CasesDir() string {
	return filepath.Join(t.Path, casesDir)
}

func (t *Test) forEach(f func(caseDir string) error) error {
	cases, err := os.ReadDir(t.CasesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read the '%s' cases dir: %w", t.CasesDir(), err)
	}

	for _, testCase := range cases {
		if !testCase.IsDir() || slices.Contains(t.Ignored, testCase.Name()) {
			continue
		}

		if err = f(filepath.Join(t.CasesDir(), testCase.Name())); err != nil {
			return err
		}
	}

	return nil
}
