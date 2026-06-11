package step

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/ipaqsa/kube2e/internal/engine/action"
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
	values      *safe.Store[string]

	logger *slog.Logger
}

// Config carries everything a step runner needs, including the case-level
// objects map so the step can resolve its template by resource name.
type Config struct {
	Kube     *svckube.Service
	Template *template.Manager

	// Objects is the case-level map of resource name → template base-filename.
	Objects map[string]string

	Step *Step

	Labels      map[string]string
	Annotations map[string]string
	Values      *safe.Store[string]

	Logger *slog.Logger
}

// Run executes every action in the step against the cluster.
func Run(ctx context.Context, conf *Config) error {
	if conf.Step == nil {
		return interrors.ErrNilStep
	}

	svc := new(service)

	svc.kube = conf.Kube
	svc.template = conf.Template

	svc.labels = maps.Clone(conf.Labels)
	svc.annotations = maps.Clone(conf.Annotations)
	svc.values = conf.Values

	if len(svc.labels) == 0 {
		svc.labels = make(map[string]string)
	}

	maps.Copy(svc.labels, conf.Step.Labels)

	if len(svc.annotations) == 0 {
		svc.annotations = make(map[string]string)
	}

	maps.Copy(svc.annotations, conf.Step.Annotations)

	svc.annotations[stepAnnotation] = conf.Step.Name

	svc.logger = conf.Logger.With("step", conf.Step.Name)

	return svc.run(ctx, conf.Step, conf.Objects)
}

// run renders the step's object and dispatches every action.
func (s *service) run(ctx context.Context, st *Step, objects map[string]string) error {
	if st == nil {
		return interrors.ErrNilStep
	}

	templateName, ok := objects[st.Object]
	if !ok {
		return fmt.Errorf("object %q: %w", st.Object, interrors.ErrObjectNotInMap)
	}

	err := st.forEach(func(act action.Action) error {
		s.logger.Info("run action", "name", act.String(st.Object))

		// Name is always injected from the objects map key.
		// Ensure actions carry their own Values for the full spec render;
		// all other actions render with name-only and use only GVK + name.
		// Stored case values (from Value actions) are exposed as .Values.<key>.
		renderValues := map[string]any{"name": st.Object}
		maps.Copy(renderValues, act.Values)

		renderValues["Values"] = maps.Collect(s.values.Iter())

		rendered, err := s.template.Render(templateName, renderValues)
		if err != nil {
			return fmt.Errorf("render object %q from template %q: %w", st.Object, templateName, err)
		}

		conf := &action.Config{
			Kube: s.kube,

			Action: act,
			Object: rendered,

			Labels:      s.labels,
			Annotations: s.annotations,
			Values:      s.values,

			Logger: s.logger,
		}

		return action.Run(ctx, conf)
	})
	if err != nil {
		if st.Optional {
			s.logger.Warn("optional step failed", "error", err)
			return nil
		}

		return fmt.Errorf("run step %q: %w", st.Name, err)
	}

	return nil
}
