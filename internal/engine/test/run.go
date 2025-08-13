package test

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/ipaqsa/kube2e/internal/engine/testcase"
	interrors "github.com/ipaqsa/kube2e/internal/errors"
	svckube "github.com/ipaqsa/kube2e/internal/kube"
	"github.com/ipaqsa/kube2e/internal/template"
)

type service struct {
	kube     *svckube.Service
	template *template.Manager

	labels      map[string]string
	annotations map[string]string

	logger *slog.Logger
}

// Config carries the inputs required to run a single test suite directory.
type Config struct {
	RestConf *rest.Config

	// TestDir is the filesystem path of the directory containing test.yaml.
	TestDir string

	Logger *slog.Logger
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

	conf.Logger.Info("run test", "name", test.Name)

	svc := new(service)

	svc.labels = test.Labels
	svc.annotations = test.Annotations

	svc.logger = conf.Logger.With("test", test.Name)

	opts := []svckube.Option{
		svckube.WithLabels(test.Labels),
		svckube.WithAnnotations(test.Annotations),
		svckube.WithNamespace(test.Namespace),
	}

	if svc.kube, err = svckube.New(conf.RestConf, svc.logger, opts...); err != nil {
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

	return svc.run(ctx, test)
}

// run executes the parsed test.
func (s *service) run(ctx context.Context, test *Test) error {
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

			Labels:      s.labels,
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
