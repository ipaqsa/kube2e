// Package testcase manages test case execution and cleanup.
package testcase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"

	"github.com/ipaqsa/kube2e/internal/engine/step"
	interrors "github.com/ipaqsa/kube2e/internal/errors"
	svckube "github.com/ipaqsa/kube2e/internal/kube"
	"github.com/ipaqsa/kube2e/internal/template"
	"github.com/ipaqsa/kube2e/internal/tools/safe"
)

type service struct {
	kube     *svckube.Service
	template *template.Manager

	labels      map[string]string
	annotations map[string]string

	values *safe.Store[string]

	logger *slog.Logger
}

// Config holds parameters for a test case execution.
type Config struct {
	Kube     *svckube.Service
	Template *template.Manager

	Path string

	Labels      map[string]string
	Annotations map[string]string

	Logger *slog.Logger
}

// Run loads a case directory and executes its steps.
func Run(ctx context.Context, conf *Config) error {
	testCase, err := parseCaseFile(conf.Path)
	if err != nil {
		return fmt.Errorf("parse case file '%s': %w", conf.Path, err)
	}

	svc := new(service)

	svc.kube = conf.Kube
	svc.template = conf.Template

	svc.labels = conf.Labels
	svc.annotations = conf.Annotations
	svc.values = safe.NewStore[string]()

	if len(svc.labels) == 0 {
		svc.labels = make(map[string]string)
	}

	maps.Copy(svc.labels, testCase.Labels)

	if len(svc.annotations) == 0 {
		svc.annotations = make(map[string]string)
	}

	maps.Copy(svc.annotations, testCase.Annotations)

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

	if err := s.runSteps(ctx, testCase, testCase.Hooks.Before, "before hook"); err != nil {
		result = errors.Join(result, err)
	} else if err = s.runStep(ctx, testCase, caseStep, "step"); err != nil {
		result = errors.Join(result, err)
	}

	if err := s.runSteps(ctx, testCase, testCase.Hooks.After, "after hook"); err != nil {
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

		Labels:      s.labels,
		Annotations: s.annotations,
		Values:      s.values,

		Logger: s.logger,
	}

	if err := step.Run(ctx, conf); err != nil {
		return fmt.Errorf("run the %s '%s': %w", phase, name, err)
	}

	return nil
}
