// Package testcase manages test case execution and cleanup.
package testcase

import (
	"context"
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
		s.logger.Info("run step", "name", caseStep.Name)

		conf := &step.Config{
			Kube:     s.kube,
			Template: s.template,

			Objects: testCase.Objects,

			Step: caseStep,

			Labels:      s.labels,
			Annotations: s.annotations,
			Values:      s.values,

			Logger: s.logger,
		}

		if err := step.Run(ctx, conf); err != nil {
			return fmt.Errorf("run the step '%s': %w", caseStep.Name, err)
		}
	}

	s.logger.Info("clear applied resources")

	return s.kube.ClearApplied(ctx)
}
