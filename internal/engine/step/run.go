package step

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ipaqsa/kube2e/internal/engine"
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
// Actions execute in the order: Ensure → Patch → Wait → Assert → Logs → Exec → Delete.
func Run(ctx context.Context, conf *Config) (*Report, error) {
	report := &Report{StartedAt: time.Now()}
	if conf.Step == nil {
		return finishReport(report, interrors.ErrNilStep)
	}

	report.Name = conf.Step.Name
	report.Description = conf.Step.Description
	report.Optional = conf.Step.Optional
	report.Total = conf.Step.CountActions()

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

	actions, err := svc.run(ctx, conf.Step, conf.Objects)
	report.Actions = actions
	report.Passed, report.Failed = countActions(actions)
	report.Stats = objectStats(actions)

	if err != nil && conf.Step.Optional {
		svc.logger.Warn("optional step failed", "error", err)

		report.FinishedAt = time.Now()
		report.State = engine.StateSkipped
		report.Reason = err.Error()

		return report, nil
	}

	if err != nil {
		err = fmt.Errorf("run step %q: %w", conf.Step.Name, err)
	}

	return finishReport(report, err)
}

// run executes configured step actions in order until one fails.
func (s *service) run(ctx context.Context, st *Step, objects map[string]string) ([]action.Report, error) {
	if st == nil {
		return nil, interrors.ErrNilStep
	}

	var err error

	reports := make([]action.Report, 0, st.CountActions())

	if st.Ensure != nil {
		report, runErr := s.runEnsure(ctx, st, objects)
		err = appendActionReport(&reports, report, runErr)
	}

	if err == nil && st.Patch != nil {
		report, runErr := s.runPatch(ctx, st, objects)
		err = appendActionReport(&reports, report, runErr)
	}

	if err == nil && st.Wait != nil {
		report, runErr := s.runWait(ctx, st, objects)
		err = appendActionReport(&reports, report, runErr)
	}

	if err == nil && st.Assert != nil {
		report, runErr := s.runAssert(ctx, st, objects)
		err = appendActionReport(&reports, report, runErr)
	}

	if err == nil && st.Logs != nil {
		report, runErr := s.runLogs(ctx, st, objects)
		err = appendActionReport(&reports, report, runErr)
	}

	if err == nil && st.Exec != nil {
		report, runErr := s.runExec(ctx, st, objects)
		err = appendActionReport(&reports, report, runErr)
	}

	if err == nil && st.Delete != nil {
		report, runErr := s.runDelete(ctx, st, objects)
		err = appendActionReport(&reports, report, runErr)
	}

	return reports, err
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

// runEnsure renders the ensure object and executes the ensure action.
func (s *service) runEnsure(ctx context.Context, st *Step, objects map[string]string) (*action.Report, error) {
	s.logger.Info("run action", "action", "ensure")

	name := st.Ensure.Object

	obj, err := s.render(objects, name, st.Ensure.Values)
	if err != nil {
		return failedActionReport(action.NameEnsure, action.Target{Object: name}, fmt.Errorf("render object %q for ensure: %w", name, err))
	}

	return action.RunEnsure(ctx, s.actionConf(obj), st.Ensure)
}

// runPatch renders the patch object and executes the patch action.
func (s *service) runPatch(ctx context.Context, st *Step, objects map[string]string) (*action.Report, error) {
	s.logger.Info("run action", "action", "patch")

	name := st.Patch.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return failedActionReport(action.NamePatch, st.Patch.Target, fmt.Errorf("render object %q for patch: %w", name, err))
	}

	return action.RunPatch(ctx, s.actionConf(obj), st.Patch)
}

// runWait renders the wait object and executes the wait action.
func (s *service) runWait(ctx context.Context, st *Step, objects map[string]string) (*action.Report, error) {
	s.logger.Info("run action", "action", "wait")

	name := st.Wait.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return failedActionReport(action.NameWait, st.Wait.Target, fmt.Errorf("render object %q for wait: %w", name, err))
	}

	return action.RunWait(ctx, s.actionConf(obj), st.Wait)
}

// runAssert renders the assert object and executes the assert action.
func (s *service) runAssert(ctx context.Context, st *Step, objects map[string]string) (*action.Report, error) {
	s.logger.Info("run action", "action", "assert")

	name := st.Assert.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return failedActionReport(action.NameAssert, st.Assert.Target, fmt.Errorf("render object %q for assert: %w", name, err))
	}

	return action.RunAssert(ctx, s.actionConf(obj), st.Assert)
}

// runLogs renders the logs object (when the target names one) and executes the
// logs action. A kind + label-selector target needs no rendered object.
func (s *service) runLogs(ctx context.Context, st *Step, objects map[string]string) (*action.Report, error) {
	s.logger.Info("run action", "action", "logs")

	conf, err := s.targetConf(st.Logs.Target, objects)
	if err != nil {
		return failedActionReport(action.NameLogs, st.Logs.Target, err)
	}

	return action.RunLogs(ctx, conf, st.Logs)
}

// runExec renders the exec object (when the target names one) and executes the
// exec action. A kind + label-selector target needs no rendered object.
func (s *service) runExec(ctx context.Context, st *Step, objects map[string]string) (*action.Report, error) {
	s.logger.Info("run action", "action", "exec")

	conf, err := s.targetConf(st.Exec.Target, objects)
	if err != nil {
		return failedActionReport(action.NameExec, st.Exec.Target, err)
	}

	return action.RunExec(ctx, conf, st.Exec)
}

// targetConf builds an action config for an Exec/Logs target: it renders the
// named object, or returns a config with no object for a kind + selector target.
func (s *service) targetConf(t action.Target, objects map[string]string) (*action.Config, error) {
	if t.Object == "" {
		return s.actionConf(nil), nil
	}

	obj, err := s.render(objects, t.Object, nil)
	if err != nil {
		return nil, fmt.Errorf("render object %q: %w", t.Object, err)
	}

	return s.actionConf(obj), nil
}

// runDelete renders the delete object and executes the delete action.
func (s *service) runDelete(ctx context.Context, st *Step, objects map[string]string) (*action.Report, error) {
	s.logger.Info("run action", "action", "delete")

	name := st.Delete.Target.Object

	obj, err := s.render(objects, name, nil)
	if err != nil {
		return failedActionReport(action.NameDelete, st.Delete.Target, fmt.Errorf("render object %q for delete: %w", name, err))
	}

	return action.RunDelete(ctx, s.actionConf(obj), st.Delete)
}
