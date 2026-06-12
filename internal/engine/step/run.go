package step

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ipaqsa/kube2e/internal/engine/action"
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

// Config carries everything a step runner needs, including the case-level
// objects map so the step can resolve its template by resource name.
type Config struct {
	Kube     *svckube.Service
	Template *template.Manager

	// Objects is the case-level map of resource name → template base-filename.
	Objects map[string]string

	Step *Step

	Annotations map[string]string
	Values      *safe.Store[string]

	Logger *slog.Logger
}

// Run executes every configured action in the step against the cluster.
// Actions execute in the order: Ensure → Patch → Wait → Assert → Delete.
func Run(ctx context.Context, conf *Config) error {
	if conf.Step == nil {
		return interrors.ErrNilStep
	}

	svc := new(service)

	svc.kube = conf.Kube
	svc.template = conf.Template
	svc.values = conf.Values

	svc.annotations = maps.Clone(conf.Annotations)
	if len(svc.annotations) == 0 {
		svc.annotations = make(map[string]string)
	}

	svc.annotations[stepAnnotation] = conf.Step.Name

	svc.logger = conf.Logger.With("step", conf.Step.Name)

	return svc.run(ctx, conf.Step, conf.Objects)
}

func (s *service) run(ctx context.Context, st *Step, objects map[string]string) error {
	if st == nil {
		return interrors.ErrNilStep
	}

	var err error

	if st.Ensure != nil {
		err = s.runEnsure(ctx, st, objects)
	}

	if err == nil && st.Patch != nil {
		err = s.runPatch(ctx, st, objects)
	}

	if err == nil && st.Wait != nil {
		err = s.runWait(ctx, st, objects)
	}

	if err == nil && st.Assert != nil {
		err = s.runAssert(ctx, st, objects)
	}

	if err == nil && st.Delete != nil {
		err = s.runDelete(ctx, st, objects)
	}

	if err != nil {
		if st.Optional {
			s.logger.Warn("optional step failed", "error", err)

			return nil
		}

		return fmt.Errorf("run step %q: %w", st.Name, err)
	}

	return nil
}

// actionConf builds the base action.Config for a rendered object.
func (s *service) actionConf(obj client.Object) *action.Config {
	return &action.Config{
		Kube:        s.kube,
		Object:      obj,
		Annotations: s.annotations,
		Logger:      s.logger,
	}
}

// render looks up the template for name and executes it with the merged values.
func (s *service) render(objects map[string]string, name string, extra map[string]any) (client.Object, error) {
	tmplName, ok := objects[name]
	if !ok {
		return nil, fmt.Errorf("object %q: %w", name, interrors.ErrObjectNotInMap)
	}

	values := map[string]any{"name": name}

	maps.Copy(values, extra)

	values["Values"] = maps.Collect(s.values.Iter())

	return s.template.Render(tmplName, values)
}

func (s *service) runEnsure(ctx context.Context, st *Step, objects map[string]string) error {
	s.logger.Info("run action", "action", "ensure")

	name := st.Ensure.Object

	obj, err := s.render(objects, name, st.Ensure.Values)
	if err != nil {
		return fmt.Errorf("render object %q for ensure: %w", name, err)
	}

	return action.RunEnsure(ctx, s.actionConf(obj), st.Ensure)
}

func (s *service) runPatch(ctx context.Context, st *Step, objects map[string]string) error {
	s.logger.Info("run action", "action", "patch")

	name := st.Patch.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return fmt.Errorf("render object %q for patch: %w", name, err)
	}

	return action.RunPatch(ctx, s.actionConf(obj), st.Patch)
}

func (s *service) runWait(ctx context.Context, st *Step, objects map[string]string) error {
	s.logger.Info("run action", "action", "wait")

	name := st.Wait.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return fmt.Errorf("render object %q for wait: %w", name, err)
	}

	return action.RunWait(ctx, s.actionConf(obj), st.Wait)
}

func (s *service) runAssert(ctx context.Context, st *Step, objects map[string]string) error {
	s.logger.Info("run action", "action", "assert")

	name := st.Assert.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return fmt.Errorf("render object %q for assert: %w", name, err)
	}

	return action.RunAssert(ctx, s.actionConf(obj), st.Assert)
}

func (s *service) runDelete(ctx context.Context, st *Step, objects map[string]string) error {
	s.logger.Info("run action", "action", "delete")

	name := st.Delete.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return fmt.Errorf("render object %q for delete: %w", name, err)
	}

	return action.RunDelete(ctx, s.actionConf(obj), st.Delete)
}
