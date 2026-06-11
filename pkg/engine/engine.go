// Package engine is the public entry point for executing kube2e test suites.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/ipaqsa/kube2e/internal/engine/test"
)

// Config holds the top-level inputs needed to discover and run a suite of tests.
type Config struct {
	// RestConfig is the Kubernetes REST client configuration used by all suites.
	RestConfig *rest.Config

	// WorkDir is the root directory that contains test subdirectories.
	WorkDir string
	// Tests is an optional allowlist of test directory names. When empty all
	// test directories under WorkDir are executed.
	Tests []string
}

// RunTests discovers test directories under cfg.WorkDir and runs each one in
// order. When cfg.Tests is non-empty only the listed directories are executed.
// Hidden directories (names starting with ".") are always skipped.
func RunTests(ctx context.Context, cfg *Config, logger *slog.Logger) error {
	return runTest(ctx, cfg, cfg.WorkDir, logger)
}

// runTest discovers and runs immediate child suites in testsDir.
func runTest(ctx context.Context, cfg *Config, testsDir string, logger *slog.Logger) error {
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		return fmt.Errorf("read work dir '%s': %w", testsDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip hidden directories (.git, .idea, etc.).
		if strings.HasPrefix(name, ".") {
			logger.Debug("skip hidden dir", "name", name)
			continue
		}

		if len(cfg.Tests) > 0 && !slices.Contains(cfg.Tests, name) {
			logger.Warn("skip test dir", "name", name)
			continue
		}

		logger.Info("found test dir", "name", name)

		conf := &test.Config{
			RestConf: cfg.RestConfig,
			TestDir:  filepath.Join(testsDir, name),
			Logger:   logger,
		}

		if err = test.Run(ctx, conf); err != nil {
			return fmt.Errorf("test '%s': %w", name, err)
		}
	}

	return nil
}
