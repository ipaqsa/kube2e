package action

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"kube2e/internal/manager/template"
	svckube "kube2e/internal/service/kube"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

const (
	commandEnsure = "Ensure"
	commandDelete = "Delete"
	commandWait   = "Wait"
	commandPatch  = "Patch"

	defaultTimeout  = 30 * time.Second
	defaultInterval = 30 * time.Second
)

type service struct {
	manager *template.Manager
	kube    *svckube.Service
	logger  *slog.Logger
}

// run executes the provided action against the kubernetes cluster using the
// given services and template manager.
func Run(ctx context.Context, kube *svckube.Service, manager *template.Manager, action Action, logger *slog.Logger) error {
	svc := new(service)

	svc.kube = kube
	svc.manager = manager
	svc.logger = logger.With("action", action.String())

	return svc.run(ctx, action)
}

func (s *service) run(ctx context.Context, action Action) error {
	s.logger.Info("handle command", "name", action.Command)
	switch action.Command {
	case commandEnsure:
		return s.handleEnsure(ctx, action.Object)
	case commandDelete:
		return s.handleDelete(ctx, action.Object)
	case commandWait:
		return s.handleWait(ctx, action)
	case commandPatch:
		return s.handlePatch(ctx, action)
	}

	return nil
}

// handleEnsure renders the object template and ensures its presence on the cluster.
func (s *service) handleEnsure(ctx context.Context, obj Object) error {
	rendered, err := s.manager.Render(obj.Template, obj.Values)
	if err != nil {
		return fmt.Errorf("render the '%s' object: %w", obj.Template, err)
	}

	s.logger.Debug("ensure object", "name", rendered.GetName())

	return s.kube.Ensure(ctx, rendered)
}

// handleDelete renders the object template and deletes it from the cluster.
func (s *service) handleDelete(ctx context.Context, obj Object) error {
	rendered, err := s.manager.Render(obj.Template, obj.Values)
	if err != nil {
		return fmt.Errorf("render the '%s' object: %w", obj.Template, err)
	}

	s.logger.Debug("delete object", "name", rendered.GetName())

	return s.kube.Delete(ctx, rendered)
}

// handleWait renders the object template and waits for the specified condition.
func (s *service) handleWait(ctx context.Context, action Action) error {
	rendered, err := s.manager.Render(action.Object.Template, action.Object.Values)
	if err != nil {
		return fmt.Errorf("render the '%s' object: %w", action.Object.Template, err)
	}

	s.logger.Debug("wait object", "name", rendered.GetName())

	opts := []svckube.WaitOptionFunc{
		svckube.WithWaitCondition(action.Condition),
		svckube.WithWaitInterval(action.IntervalOrDefault(defaultInterval)),
		svckube.WithWaitTimeout(action.TimeoutOrDefault(defaultTimeout)),
		svckube.WithOnDeletion(action.Deletion),
	}

	return s.kube.Wait(ctx, rendered, opts...)
}

// handlePatch renders the object template and applies json patches before ensuring it.
func (s *service) handlePatch(ctx context.Context, action Action) error {
	rendered, err := s.manager.Render(action.Object.Template, action.Object.Values)
	if err != nil {
		return fmt.Errorf("render the '%s' object: %w", action.Object.Template, err)
	}

	s.logger.Debug("patch object", "name", rendered.GetName())

	if len(action.Patches) == 0 {
		return nil
	}

	opts := jsonpatch.NewApplyOptions()
	opts.EnsurePathExistsOnAdd = true

	obj, err := action.Patches.Apply(rendered, opts)
	if err != nil {
		return fmt.Errorf("apply patch to the '%s' object: %w", action.Object.Template, err)
	}

	return s.kube.Ensure(ctx, obj)
}
