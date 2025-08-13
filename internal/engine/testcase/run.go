package testcase

import (
	"context"
	"fmt"
	"log/slog"

	"kube2e/internal/engine/step"
	"kube2e/internal/manager/template"
	svckube "kube2e/internal/service/kube"
)

type service struct {
	manager *template.Manager
	kube    *svckube.Service
	logger  *slog.Logger
}

func Run(ctx context.Context, kube *svckube.Service, caseDir string, logger *slog.Logger) error {
	if kube == nil {
		return fmt.Errorf("kube service is nil")
	}

	if len(caseDir) == 0 {
		return fmt.Errorf("case dir is empty")
	}

	testCase, err := parseCaseFile(caseDir)
	if err != nil {
		return fmt.Errorf("parse case file from the '%s' dir: %w", caseDir, err)
	}

	svc := new(service)

	svc.kube = kube
	svc.logger = logger.With("case", testCase.Name)

	if svc.manager, err = template.NewManager(testCase.TemplatesDir(), svc.logger); err != nil {
		return fmt.Errorf("create template store from the '%s' dir: %w", testCase.TemplatesDir(), err)
	}

	svc.logger.Debug("case service initialized")

	return svc.run(ctx, testCase)
}

func (s *service) run(ctx context.Context, testCase *Case) error {
	if testCase == nil {
		return fmt.Errorf("nil test case")
	}

	err := testCase.forEach(func(stepDir string) error {
		s.logger.Info("run step from dir", "name", stepDir)
		return step.Run(ctx, s.kube, s.manager, stepDir, s.logger)
	})
	if err != nil {
		return fmt.Errorf("run the '%s' test case: %w", testCase.Name, err)
	}

	s.logger.Info("clear applied resources")

	return s.kube.ClearApplied(ctx)
}
