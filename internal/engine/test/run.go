package test

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/ipaqsa/kube2e/internal/engine/testcase"
	interrors "github.com/ipaqsa/kube2e/internal/errors"
	svckube "github.com/ipaqsa/kube2e/internal/kube"
	"github.com/ipaqsa/kube2e/internal/template"
)

type service struct {
	kube        *svckube.Service
	template    *template.Manager
	annotations map[string]string
	logger      *slog.Logger
}

// Config carries the inputs required to run a single test suite directory.
type Config struct {
	RestConf *rest.Config

	// TestDir is the filesystem path of the directory containing test.yaml.
	TestDir string

	// Tags is the requested tag filter from the CLI. Empty means run all.
	Tags []string

	Logger *slog.Logger

	DryRun bool
}

// Run sets up services and orchestrates test cases found in the directory.
// CRDs are expected to already be present in the cluster; kube2e does not
// manage CRD lifecycle.
func Run(ctx context.Context, conf *Config) error {
	test, err := parseTestFile(conf.TestDir)
	if err != nil {
		return fmt.Errorf("parse test file from the '%s' dir: %w", conf.TestDir, err)
	}

	if test == nil {
		conf.Logger.Warn("skip dir: no test.yaml found", "dir", conf.TestDir)
		return nil
	}

	// Test-level tag filter: if the test carries non-empty tags and none match,
	// skip the entire suite without error.
	if len(conf.Tags) > 0 && len(test.Tags) > 0 && !anyTagMatches(test.Tags, conf.Tags) {
		conf.Logger.Info("skip test (no matching tags)", "name", test.Name, "tags", test.Tags)
		return nil
	}

	conf.Logger.Info("run test", "name", test.Name)

	svc := new(service)

	// Inject the kube2e test-level tracing annotation.
	svc.annotations = map[string]string{testAnnotation: test.Name}

	svc.logger = conf.Logger.With("test", test.Name)

	kubeOpts := []svckube.Option{svckube.WithNamespace(test.Namespace)}
	if conf.DryRun {
		kubeOpts = append(kubeOpts, svckube.WithDryRun())
	}

	if svc.kube, err = svckube.New(conf.RestConf, svc.logger, kubeOpts...); err != nil {
		return fmt.Errorf("create kube service: %w", err)
	}

	version, err := svc.kube.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("get kubernetes version: %w", err)
	}

	svc.logger.Info("kubernetes version", "version", version)

	if svc.template, err = template.NewManager(test.TemplatesDir(), svc.logger); err != nil {
		return fmt.Errorf("create template manager from the dir '%s': %w", test.TemplatesDir(), err)
	}

	svc.logger.Debug("test service initialized")

	// When the test itself has a matching tag, propagate nil tags so all cases
	// run without case-level filtering.
	caseTags := conf.Tags
	if len(test.Tags) > 0 && anyTagMatches(test.Tags, conf.Tags) {
		caseTags = nil
	}

	return svc.run(ctx, test, caseTags)
}

// run executes the parsed test.
func (s *service) run(ctx context.Context, test *Test, caseTags []string) error {
	if test == nil {
		return interrors.ErrNilTest
	}

	var namespace *corev1.Namespace
	if len(test.Namespace) > 0 {
		namespace = &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: test.Namespace,
			},
		}
	}

	if namespace != nil {
		s.logger.Debug("ensure namespace", "name", namespace.Name)

		if err := s.kube.Ensure(ctx, namespace, svckube.WithEnsureToCache(false)); err != nil {
			return fmt.Errorf("ensure the namespace '%s': %w", test.Namespace, err)
		}
	}

	err := test.forEach(func(casePath string) error {
		conf := &testcase.Config{
			Kube:     s.kube,
			Template: s.template,

			Path: casePath,

			Tags:        caseTags,
			Annotations: s.annotations,

			Logger: s.logger,
		}

		s.logger.Info("run case", "path", casePath)

		return testcase.Run(ctx, conf)
	})
	if err != nil {
		return fmt.Errorf("run the test '%s': %w", test.Name, err)
	}

	if namespace != nil {
		s.logger.Debug("delete namespace", "name", namespace.Name)

		if err = s.kube.Delete(ctx, namespace); err != nil {
			return fmt.Errorf("delete the namespace '%s': %w", test.Name, err)
		}
	}

	return nil
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
