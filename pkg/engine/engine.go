// Package engine is the public entry point for executing kube2e test suites.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/ipaqsa/kube2e/internal/engine/test"
	"github.com/ipaqsa/kube2e/internal/tools/workerpool"
)

// Config holds the top-level inputs needed to discover and run a suite of tests.
type Config struct {
	// RestConfig is the Kubernetes REST client configuration used by all suites.
	RestConfig *rest.Config

	// WorkDir is the root directory that contains test subdirectories.
	WorkDir string

	// Tags is an optional tag filter. When non-empty only tests and cases that
	// carry at least one matching tag are executed. See test.yaml and case.yaml
	// for how to assign tags.
	Tags []string

	// Parallel controls how many test suite directories are executed concurrently.
	// Values less than 2 mean sequential execution.
	Parallel int

	DryRun bool
}

// RunTests discovers test directories under cfg.WorkDir and runs them.
// When cfg.Tags is non-empty, only matching tests and cases are executed.
// When cfg.Parallel > 1, suites run concurrently (cases within a suite remain sequential).
func RunTests(ctx context.Context, cfg *Config, logger *slog.Logger) error {
	entries, err := os.ReadDir(cfg.WorkDir)
	if err != nil {
		return fmt.Errorf("read work dir '%s': %w", cfg.WorkDir, err)
	}

	var dirs []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip hidden directories (.git, .idea, etc.).
		if strings.HasPrefix(entry.Name(), ".") {
			logger.Debug("skip hidden dir", "name", entry.Name())
			continue
		}

		dirs = append(dirs, filepath.Join(cfg.WorkDir, entry.Name()))
	}

	return workerpool.Do(ctx, max(cfg.Parallel, 1), func(ctx context.Context, dir string) error {
		conf := &test.Config{
			RestConf: cfg.RestConfig,
			TestDir:  dir,
			Tags:     cfg.Tags,
			DryRun:   cfg.DryRun,
			Logger:   logger,
		}

		if err := test.Run(ctx, conf); err != nil {
			return fmt.Errorf("test '%s': %w", filepath.Base(dir), err)
		}

		return nil
	}, dirs)
}
