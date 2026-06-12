// Package engine is the public entry point for executing kube2e test suites.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"k8s.io/client-go/rest"

	"github.com/ipaqsa/kube2e/internal/engine/test"
	"github.com/ipaqsa/kube2e/internal/image"
	"github.com/ipaqsa/kube2e/internal/tools/workerpool"
)

// Config holds the top-level inputs needed to discover and run a suite of tests.
type Config struct {
	// RestConfig is the Kubernetes REST client configuration used by all suites.
	RestConfig *rest.Config

	// WorkDir is the root directory that contains test subdirectories.
	WorkDir string

	// Tags is an optional tag filter. When non-empty only cases that carry at
	// least one matching tag are executed. See case.yaml for how to assign tags.
	Tags []string

	// Parallel controls how many test suite directories are executed concurrently.
	// Values less than 2 mean sequential execution.
	Parallel int

	// Remote identifies an optional container image source for test suites.
	Remote Remote

	// DryRun validates and renders tests without applying resources to the cluster.
	DryRun bool
}

// Remote holds credentials and address for a remote test image.
type Remote struct {
	Ref      string
	Username string
	Password string
}

// RunTests discovers test directories under cfg.WorkDir and runs them.
// When cfg.Tags is non-empty, only matching tests and cases are executed.
// When cfg.Parallel > 1, suites run concurrently (cases within a suite remain sequential).
func RunTests(ctx context.Context, cfg *Config, logger *slog.Logger) (*Report, error) {
	return runTests(ctx, cfg, logger)
}

// runTests executes either local or remote tests based on cfg.
func runTests(ctx context.Context, cfg *Config, logger *slog.Logger) (*Report, error) {
	if cfg.Remote.Ref != "" {
		return runRemote(ctx, cfg, logger)
	}

	return runLocal(ctx, cfg, logger)
}

// runRemote extracts remote tests and runs them through the local engine path.
func runRemote(ctx context.Context, cfg *Config, logger *slog.Logger) (*Report, error) {
	report := newReport(cfg)
	logger.Debug("pull tests image", "image", cfg.Remote.Ref)

	var runReport *Report

	err := image.Traverse(ctx, image.Remote{
		Ref:      cfg.Remote.Ref,
		Username: cfg.Remote.Username,
		Password: cfg.Remote.Password,
	}, func(dir string) error {
		next := *cfg
		next.WorkDir = filepath.Join(dir, cfg.WorkDir)
		next.Remote = Remote{}

		var err error

		runReport, err = runLocal(ctx, &next, logger)

		return err
	})

	if runReport != nil {
		report.Passed = runReport.Passed
		report.Failed = runReport.Failed
		report.Total = runReport.Total
		report.Tests = runReport.Tests
	}

	return finishReport(report, err)
}

// runLocal discovers local test directories under cfg.WorkDir and runs them.
func runLocal(ctx context.Context, cfg *Config, logger *slog.Logger) (*Report, error) {
	report := newReport(cfg)

	entries, err := os.ReadDir(cfg.WorkDir)
	if err != nil {
		return finishReport(report, fmt.Errorf("read work dir '%s': %w", cfg.WorkDir, err))
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

	report.Total = len(dirs)

	var mu sync.Mutex

	reports := make(map[string]test.Report, len(dirs))

	runErr := workerpool.Do(ctx, max(cfg.Parallel, 1), func(ctx context.Context, dir string) error {
		conf := &test.Config{
			RestConf: cfg.RestConfig,
			TestDir:  dir,
			Tags:     cfg.Tags,
			DryRun:   cfg.DryRun,
			Logger:   logger,
		}

		testReport, testErr := test.Run(ctx, conf)
		if testReport != nil {
			mu.Lock()
			reports[dir] = *testReport
			mu.Unlock()
		}

		if testErr != nil {
			return fmt.Errorf("test %q: %w", filepath.Base(dir), testErr)
		}

		return nil
	}, dirs)

	for _, dir := range dirs {
		testReport, ok := reports[dir]
		if !ok {
			continue
		}

		report.Tests = append(report.Tests, testReport)
	}

	report.Passed, report.Failed = countTests(report.Tests)

	report, err = finishReport(report, runErr)

	return report, err
}
