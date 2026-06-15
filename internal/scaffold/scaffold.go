// Package scaffold creates starter kube2e test-suite directories.
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	casesDir     = "cases"
	templatesDir = "templates"

	dirPerm  = 0o750
	filePerm = 0o600
)

// Config holds the inputs for scaffolding a new test suite.
type Config struct {
	// Dir is the parent directory the suite directory is created in.
	Dir string
	// Name is the suite directory name; it also becomes the suite name.
	Name string
}

// Create scaffolds a new test suite under Dir/Name with cases/ and templates/
// subdirectories, a starter ConfigMap template, and a starter case whose
// optional fields are documented as comments. It returns the suite directory
// path and fails if that directory already exists.
func Create(cfg Config) (string, error) {
	if cfg.Name == "" {
		return "", errors.New("suite name is required")
	}

	suiteDir := filepath.Join(cfg.Dir, cfg.Name)

	switch _, err := os.Stat(suiteDir); {
	case err == nil:
		return "", fmt.Errorf("suite directory '%s' already exists", suiteDir)
	case !os.IsNotExist(err):
		return "", fmt.Errorf("stat suite dir '%s': %w", suiteDir, err)
	}

	for _, dir := range []string{filepath.Join(suiteDir, casesDir), filepath.Join(suiteDir, templatesDir)} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return "", fmt.Errorf("create dir '%s': %w", dir, err)
		}
	}

	files := map[string]string{
		filepath.Join(suiteDir, templatesDir, "configmap.yaml"): templateYAML,
		filepath.Join(suiteDir, casesDir, "example.yaml"):       caseYAML,
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), filePerm); err != nil {
			return "", fmt.Errorf("write '%s': %w", path, err)
		}
	}

	return suiteDir, nil
}
