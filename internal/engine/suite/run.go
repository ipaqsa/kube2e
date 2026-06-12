// Package suite manages test case execution and cleanup.
package suite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/ipaqsa/kube2e/internal/engine"
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
	RestConf *rest.Config
	DryRun   bool

	Template *template.Manager

	Path string

	// Tags is the requested tag filter. When non-empty the case is skipped unless
	// it has at least one matching tag.
	Tags []string

	// Annotations carries engine-injected tracing annotations (e.g. test name).
	Annotations map[string]string

	Logger *slog.Logger
}

// Run loads a case file, creates a scoped kube service, and executes its steps.
func Run(ctx context.Context, conf *Config) (*Report, error) {
	report := &Report{
		Path:      conf.Path,
		StartedAt: time.Now(),
	}

	testCase, err := parseCaseFile(conf.Path)
	if err != nil {
		return finishReport(report, fmt.Errorf("parse case file '%s': %w", conf.Path, err))
	}

	report = newReport(testCase)

	// Case-level tag filter: skip if tags requested but none of the case's tags match.
	if len(conf.Tags) > 0 && !anyTagMatches(testCase.Tags, conf.Tags) {
		conf.Logger.Debug("skip case (no matching tags)", "name", testCase.Name, "tags", testCase.Tags)

		report.FinishedAt = time.Now()
		report.State = engine.StateSkipped

		return report, nil
	}

	svc := new(service)

	kubeOpts := make([]svckube.Option, 0, 2)
	if testCase.Namespace != "" {
		kubeOpts = append(kubeOpts, svckube.WithNamespace(testCase.Namespace))
	}

	if conf.DryRun {
		kubeOpts = append(kubeOpts, svckube.WithDryRun())
	}

	if svc.kube, err = svckube.New(conf.RestConf, conf.Logger, kubeOpts...); err != nil {
		return finishReport(report, fmt.Errorf("create kube service: %w", err))
	}

	svc.template = conf.Template
	svc.values = safe.NewStore[string]()

	svc.annotations = maps.Clone(conf.Annotations)
	if svc.annotations == nil {
		svc.annotations = make(map[string]string)
	}

	svc.annotations[caseAnnotation] = testCase.Name

	svc.logger = conf.Logger.With("case", testCase.Name)
	svc.logger.Debug("case service initialized")

	var namespace *corev1.Namespace
	if testCase.Namespace != "" {
		namespace = &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: testCase.Namespace,
			},
		}

		svc.logger.Debug("ensure namespace", "name", namespace.Name)

		if err = svc.kube.Ensure(ctx, namespace, svckube.WithEnsureToCache(false)); err != nil {
			return finishReport(report, fmt.Errorf("ensure namespace '%s': %w", testCase.Namespace, err))
		}
	}

	defer func() {
		if namespace != nil {
			svc.logger.Debug("delete namespace", "name", namespace.Name)

			if deleteErr := svc.kube.Delete(ctx, namespace); deleteErr != nil {
				svc.logger.Warn("delete namespace", "name", namespace.Name, "error", deleteErr)
			}
		}
	}()

	runReport, err := svc.run(ctx, testCase)
	if runReport != nil {
		report = runReport
	}

	return finishReport(report, err)
}

// run iterates through the case steps using the provided services.
func (s *service) run(ctx context.Context, testCase *Case) (*Report, error) {
	report := newReport(testCase)
	if testCase == nil {
		return finishReport(report, interrors.ErrNilTestCase)
	}

	if len(testCase.Steps) == 0 {
		s.logger.Warn("no steps found")

		return report, nil
	}

	for idx, caseStep := range testCase.Steps {
		stepReport, hooksReport, err := s.runStepWithHooks(ctx, len(testCase.Steps), idx+1, testCase, caseStep)
		appendHooksReport(report, hooksReport)

		if stepReport != nil {
			report.Steps = append(report.Steps, *stepReport)
		}

		report.Passed, report.Failed = countSteps(report.Steps)

		if err != nil {
			return report, err
		}
	}

	s.logger.Debug("cleanup")

	return report, s.kube.Cleanup(ctx)
}

// runStepWithHooks executes caseStep with case-level before and after hooks.
func (s *service) runStepWithHooks(ctx context.Context, total, idx int, testCase *Case, caseStep *step.Step) (*step.Report, *HooksReport, error) {
	var (
		result     error
		stepReport *step.Report
	)

	hooks := new(HooksReport)

	beforeReports, err := s.runSteps(ctx, testCase, testCase.Hooks.BeforeEach, "before hook")
	hooks.BeforeEach = beforeReports

	if err != nil {
		result = errors.Join(result, err)
	} else {
		stepReport, err = s.runStep(ctx, total, idx, testCase, caseStep, "step")
		if err != nil {
			result = errors.Join(result, err)
		}
	}

	afterReports, err := s.runSteps(ctx, testCase, testCase.Hooks.AfterEach, "after hook")
	hooks.AfterEach = afterReports

	if err != nil {
		result = errors.Join(result, err)
	}

	return stepReport, hooks, result
}

// runSteps executes steps in order for the given phase.
func (s *service) runSteps(ctx context.Context, testCase *Case, steps []*step.Step, phase string) ([]step.Report, error) {
	reports := make([]step.Report, 0, len(steps))

	for idx, st := range steps {
		report, err := s.runStep(ctx, len(steps), idx+1, testCase, st, phase)
		if report != nil {
			reports = append(reports, *report)
		}

		if err != nil {
			return reports, err
		}
	}

	return reports, nil
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
func (s *service) runStep(ctx context.Context, total, idx int, testCase *Case, st *step.Step, phase string) (*step.Report, error) {
	name := ""
	actions := 0

	if st != nil {
		name = st.Name
		actions = st.CountActions()
	}

	log := s.logger.With("name", name, "actions", actions)

	progress := fmt.Sprintf("[%d/%d]", idx, total)
	log.Info("run step", "progress", progress, "phase", phase)

	start := time.Now()

	conf := &step.Config{
		Kube:     s.kube,
		Template: s.template,

		Objects: testCase.Objects,

		Step: st,

		Annotations: s.annotations,
		Values:      s.values,

		Logger: s.logger,
	}

	report, err := step.Run(ctx, conf)
	if err != nil {
		log.Debug("step failed", "progress", progress, "duration", time.Since(start))

		return report, fmt.Errorf("run the %s '%s': %w", phase, name, err)
	}

	log.Debug("step finished", "progress", progress, "duration", time.Since(start))

	return report, nil
}
