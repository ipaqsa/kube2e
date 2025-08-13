package step

import (
	"context"
	"fmt"
	"log/slog"

	"kube2e/internal/engine/action"
	"kube2e/internal/manager/template"
	svckube "kube2e/internal/service/kube"
)

type service struct {
	manager *template.Manager
	kube    *svckube.Service
	logger  *slog.Logger
}

// run loads a step file and executes each action it defines.
func Run(ctx context.Context, kube *svckube.Service, manager *template.Manager, stepFile string, logger *slog.Logger) error {
	if kube == nil {
		return fmt.Errorf("kube is nil")
	}

	if manager == nil {
		return fmt.Errorf("manager is nil")
	}

	if len(stepFile) == 0 {
		return fmt.Errorf("step file is empty")
	}

	step, err := parseStepFile(stepFile)
	if err != nil {
		return fmt.Errorf("parse the '%s' step: %w", stepFile, err)
	}

	svc := new(service)

	svc.manager = manager
	svc.kube = kube
	svc.logger = logger.With("step", step.Name)

	return svc.run(ctx, step)
}

// run executes the step using the underlying services.
func (s *service) run(ctx context.Context, step *Step) error {
	if step == nil {
		return fmt.Errorf("step is nil")
	}

	err := step.forEach(func(act action.Action) error {
		s.logger.Info("run action", "name", act.String())
		return action.Run(ctx, s.kube, s.manager, act, s.logger)
	})
	if err != nil {
		return fmt.Errorf("run the '%s' case step: %w", step.Name, err)
	}

	return nil
}
