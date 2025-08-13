// Package action implements Kubernetes resource operations (Ensure, Delete, Wait, Patch).
package action

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svckube "github.com/ipaqsa/kube2e/internal/kube"
	"github.com/ipaqsa/kube2e/internal/tools/safe"
)

const (
	// create or update the object.
	commandEnsure = "Ensure"
	// delete the object.
	commandDelete = "Delete"
	// wait for specific condition.
	commandWait = "Wait"
	// patch the object.
	commandPatch = "Patch"
	// extract value from object and store in shared values.
	commandValue = "Value"

	// polling interval.
	defaultWaitInterval = 2 * time.Second
	// polling timeout.
	defaultWaitTimeout = 2 * time.Minute
)

type service struct {
	kube *svckube.Service

	labels      map[string]string
	annotations map[string]string
	values      *safe.Store[string]

	logger *slog.Logger
}

// Config holds parameters for a single action execution.
type Config struct {
	Kube *svckube.Service

	Object client.Object
	Action Action

	Labels      map[string]string
	Annotations map[string]string
	Values      *safe.Store[string]

	Logger *slog.Logger
}

// Run executes the provided action against the kubernetes cluster using the
// given services and template manager.
func Run(ctx context.Context, conf *Config) error {
	svc := new(service)

	svc.kube = conf.Kube

	svc.labels = conf.Labels
	svc.annotations = conf.Annotations
	svc.values = conf.Values

	svc.logger = conf.Logger.With("action", conf.Action.String(conf.Object.GetName()))

	return svc.run(ctx, conf.Action, conf.Object)
}

func (s *service) run(ctx context.Context, action Action, obj client.Object) error {
	s.logger.Info("handle command", "name", action.Command)

	start := time.Now()

	defer func() {
		s.logger.Info("command finished", "duration", time.Since(start))
	}()

	if action.Delay != nil {
		s.logger.Info("command delay", "duration", action.Delay.Duration)
		time.Sleep(action.Delay.Duration)
	}

	switch action.Command {
	case commandEnsure:
		return s.handleEnsure(ctx, obj)
	case commandDelete:
		return s.handleDelete(ctx, obj)
	case commandWait:
		return s.handleWait(ctx, action, obj)
	case commandPatch:
		return s.handlePatch(ctx, action, obj)
	case commandValue:
		return s.handleValue(ctx, action, obj)
	}

	return nil
}

// handleEnsure renders the object template and ensures its presence on the cluster.
func (s *service) handleEnsure(ctx context.Context, obj client.Object) error {
	s.logger.Debug("ensure object", "name", obj.GetName(), "namespace", obj.GetNamespace())

	return s.kube.Ensure(ctx, obj,
		svckube.WithEnsureLabels(s.labels),
		svckube.WithEnsureAnnotations(s.annotations))
}

// handleDelete renders the object template and deletes it from the cluster.
func (s *service) handleDelete(ctx context.Context, obj client.Object) error {
	s.logger.Debug("delete object", "name", obj.GetName())

	return s.kube.Delete(ctx, obj)
}

// handleWait renders the object template and waits for the specified condition.
func (s *service) handleWait(ctx context.Context, action Action, obj client.Object) error {
	s.logger.Debug("wait for object`s condition", "name", obj.GetName())

	opts := []svckube.WaitOptionFunc{
		svckube.WithWaitConditions(action.Conditions),
		svckube.WithWaitInterval(action.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithWaitTimeout(action.TimeoutOrDefault(defaultWaitTimeout)),
		svckube.WithOnDeletion(action.Deletion),
	}

	return s.kube.Wait(ctx, obj, opts...)
}

// handlePatch renders the object template and applies json patches before ensuring it.
func (s *service) handlePatch(ctx context.Context, action Action, obj client.Object) error {
	s.logger.Debug("patch object", "name", obj.GetName())

	if len(action.Patches) == 0 {
		return nil
	}

	opts := jsonpatch.NewApplyOptions()
	opts.EnsurePathExistsOnAdd = true

	patched, err := action.Patches.Apply(obj, opts)
	if err != nil {
		return fmt.Errorf("apply patch to the object '%s': %w", obj, err)
	}

	return s.kube.Ensure(ctx, patched)
}

// handleValue extracts a value from the object using a JQ path and stores it in shared values.
func (s *service) handleValue(ctx context.Context, action Action, obj client.Object) error {
	s.logger.Debug("set value from object", "name", obj.GetName(), "key", action.ValueKey, "path", action.ValuePath)

	if action.ValueKey == "" {
		return errors.New("valueKey is required")
	}

	if action.ValuePath == "" {
		return errors.New("valuePath is required")
	}

	value, err := s.kube.Filter(ctx, obj, action.ValuePath)
	if err != nil {
		return fmt.Errorf("extract value using path '%s': %w", action.ValuePath, err)
	}

	// Store in shared values
	s.values.Put(action.ValueKey, value)

	s.logger.Info("value stored", "key", action.ValueKey, "value", value)

	return nil
}
