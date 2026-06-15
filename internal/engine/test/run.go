package test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"k8s.io/client-go/rest"

	"github.com/ipaqsa/kube2e/internal/engine"
	"github.com/ipaqsa/kube2e/internal/engine/suite"
	"github.com/ipaqsa/kube2e/internal/template"
)

// Config carries the inputs required to run a single test suite directory.
type Config struct {
	RestConf *rest.Config

	// TestDir is the filesystem path of the test suite directory.
	// The suite name is derived from this directory's base name.
	TestDir string

	// Tags is the requested tag filter from the CLI. Empty means run all.
	Tags []string

	Logger *slog.Logger

	DryRun bool
}

// Run executes all test cases found under conf.TestDir.
// CRDs are expected to already be present in the cluster; kube2e does not
// manage CRD lifecycle.
func Run(ctx context.Context, conf *Config) (*Report, error) {
	test := newTest(conf.TestDir)
	report := newReport(test)

	logger := conf.Logger.With("test", test.Name)
	annotations := map[string]string{testAnnotation: test.Name}

	tmpl, err := template.NewManager(test.TemplatesDir(), logger)
	if err != nil {
		return finishReport(report, fmt.Errorf("create template manager: %w", err))
	}

	var runErr error

	execFunc := func(total, idx int, casePath string) error {
		progress := fmt.Sprintf("[%d/%d]", idx, total)
		logger.Info("run case", "progress", progress, "path", casePath)

		caseReport, caseErr := suite.Run(ctx, &suite.Config{
			RestConf:    conf.RestConf,
			DryRun:      conf.DryRun,
			Template:    tmpl,
			Path:        casePath,
			Tags:        conf.Tags,
			Annotations: annotations,
			Logger:      logger,
		})
		if caseReport != nil {
			report.Cases = append(report.Cases, *caseReport)
			report.Passed, report.Failed = countCases(report.Cases)
		}

		if caseErr != nil {
			if errors.Is(caseErr, context.Canceled) || errors.Is(caseErr, context.DeadlineExceeded) {
				return caseErr
			}

			logger.Error("case failed", "progress", progress, "path", casePath, "error", caseErr)

			runErr = errors.Join(runErr, fmt.Errorf("run case '%s': %w", filepath.Base(casePath), caseErr))
			report.Reason = fmt.Sprintf("case %q failed: %s", filepath.Base(casePath), caseErr.Error())
		}

		logger.Debug("case finished", "progress", progress, "path", casePath)

		return nil
	}

	if err = test.forEach(execFunc); err != nil {
		return finishReport(report, err)
	}

	return finishReport(report, runErr)
}

// countCases returns passed and failed case counts.
func countCases(reports []suite.Report) (int, int) {
	var (
		passed int
		failed int
	)

	for _, report := range reports {
		switch report.State {
		case engine.StatePassed, engine.StateSkipped:
			passed++
		case engine.StateFailed:
			failed++
		}
	}

	return passed, failed
}
