package testcase

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

const (
	caseFile = "case.yaml"

	stepsDir     = "steps"
	templatesDir = "templates"
)

// case holds metadata about a single test case directory.
type Case struct {
	Path        string `yaml:"path" json:"path"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
}

// parseCaseFile reads a case configuration from disk.
func parseCaseFile(caseDir string) (*Case, error) {
	content, err := os.ReadFile(filepath.Join(caseDir, caseFile))
	if err != nil {
		return nil, fmt.Errorf("read case file: %w", err)
	}

	testcase := new(Case)
	if err = yaml.Unmarshal(content, testcase); err != nil {
		return nil, fmt.Errorf("unmarshal case file: %w", err)
	}

	if testcase.Name == "" {
		return nil, fmt.Errorf("case has no name")
	}

	testcase.Path = caseDir

	return testcase, nil
}

// stepsDir returns the directory containing step definitions for the case.
func (c *Case) StepsDir() string {
	return filepath.Join(c.Path, stepsDir)
}

// templatesDir returns the directory containing templates used by steps.
func (c *Case) TemplatesDir() string {
	return filepath.Join(c.Path, templatesDir)
}

// forEach iterates over step files in order and calls the provided function.
func (c *Case) forEach(f func(stepFile string) error) error {
	steps, err := os.ReadDir(c.StepsDir())
	if err != nil {
		return fmt.Errorf("read the '%s' steps dir: %w", c.StepsDir(), err)
	}

	type stepDir struct {
		name string
		num  int
	}

	var sorted []stepDir
	for _, step := range steps {
		if step.IsDir() || filepath.Ext(step.Name()) != ".yaml" {
			continue
		}

		base := strings.TrimSuffix(step.Name(), filepath.Ext(step.Name()))
		base = strings.TrimSuffix(base, "_step")

		num, err := strconv.Atoi(base)
		if err != nil {
			return fmt.Errorf("parse step number from %q: %w", step.Name(), err)
		}

		sorted = append(sorted, stepDir{num: num, name: step.Name()})
	}

	if len(steps) == 0 {
		return nil
	}

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].num != sorted[j].num {
			return sorted[i].num < sorted[i].num
		}

		return sorted[i].name < sorted[i].name
	})

	for _, step := range sorted {
		if err = f(filepath.Join(c.StepsDir(), step.name)); err != nil {
			return err
		}
	}

	return nil
}
