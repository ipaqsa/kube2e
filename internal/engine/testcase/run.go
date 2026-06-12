// Package testcase manages test case execution and cleanup.
package testcase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/ipaqsa/kube2e/internal/engine/step"
	interrors "github.com/ipaqsa/kube2e/internal/errors"
	svckube "github.com/ipaqsa/kube2e/internal/kube"
	"github.com/ipaqsa/kube2e/internal/template"
	"github.com/ipaqsa/kube2e/internal/tools/safe"
)

type service struct {
	kube        *svckube.Service
	template    *template.Manager
	annotations map[string]string
	values      *safe.Store[string]
	logger      *slog.Logger
}

// Config holds parameters for a test case execution.
type Config struct {
	Kube     *svckube.Service
	Template *template.Manager

	Path string

	// Tags is the requested tag filter. When non-empty the case is skipped unless
	// it has at least one matching tag.
	Tags []string

	// Annotations carries engine-injected tracing annotations (e.g. test name).
	Annotations map[string]string

	Logger *slog.Logger
}

// Run loads a case directory and executes its steps.
func Run(ctx context.Context, conf *Config) error {
	testCase, err := parseCaseFile(conf.Path)
	if err != nil {
		return fmt.Errorf("parse case file '%s': %w", conf.Path, err)
	}

	// Case-level tag filter: skip if tags requested but none of the case's tags match.
	if len(conf.Tags) > 0 && !anyTagMatches(testCase.Tags, conf.Tags) {
		conf.Logger.Info("skip case (no matching tags)", "name", testCase.Name, "tags", testCase.Tags)
		return nil
	}

	svc := new(service)

	svc.kube = conf.Kube
	svc.template = conf.Template
	svc.values = safe.NewStore[string]()

	svc.annotations = maps.Clone(conf.Annotations)
	if len(svc.annotations) == 0 {
		svc.annotations = make(map[string]string)
	}

	svc.annotations[caseAnnotation] = testCase.Name

	svc.logger = conf.Logger.With("case", testCase.Name)

	svc.logger.Debug("case service initialized")

	return svc.run(ctx, testCase)
}

// run iterates through the case steps using the provided services.
func (s *service) run(ctx context.Context, testCase *Case) error {
	if testCase == nil {
		return interrors.ErrNilTestCase
	}

	if len(testCase.Steps) == 0 {
		s.logger.Warn("no steps found")
		return nil
	}

	for _, caseStep := range testCase.Steps {
		if err := s.runStepWithHooks(ctx, testCase, caseStep); err != nil {
			return err
		}
	}

	s.logger.Info("clear applied resources")

	return s.kube.ClearApplied(ctx)
}

// runStepWithHooks executes caseStep with case-level before and after hooks.
func (s *service) runStepWithHooks(ctx context.Context, testCase *Case, caseStep *step.Step) error {
	var result error

	if err := s.runSteps(ctx, testCase, testCase.Hooks.BeforeEach, "before hook"); err != nil {
		result = errors.Join(result, err)
	} else if err = s.runStep(ctx, testCase, caseStep, "step"); err != nil {
		result = errors.Join(result, err)
	}

	if err := s.runSteps(ctx, testCase, testCase.Hooks.AfterEach, "after hook"); err != nil {
		result = errors.Join(result, err)
	}

	return result
}

// runSteps executes steps in order for the given phase.
func (s *service) runSteps(ctx context.Context, testCase *Case, steps []*step.Step, phase string) error {
	for _, st := range steps {
		if err := s.runStep(ctx, testCase, st, phase); err != nil {
			return err
		}
	}

	return nil
}

// anyTagMatches reports whether items contains at least one element from requested.
func anyTagMatches(items, requested []string) bool {
	for _, item := range items {
		if slices.Contains(requested, item) {
			return true
		}
	}

	return false
}

// runStep executes one step using the test case services.
func (s *service) runStep(ctx context.Context, testCase *Case, st *step.Step, phase string) error {
	name := ""
	if st != nil {
		name = st.Name
	}

	s.logger.Info("run step", "phase", phase, "name", name)

	conf := &step.Config{
		Kube:     s.kube,
		Template: s.template,

		Objects: testCase.Objects,

		Step: st,

		Annotations: s.annotations,
		Values:      s.values,

		Logger: s.logger,
	}

	if err := step.Run(ctx, conf); err != nil {
		return fmt.Errorf("run the %s '%s': %w", phase, name, err)
	}

	return nil
}
