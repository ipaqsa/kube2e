package test

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"kube2e/internal/engine/testcase"
	svccrd "kube2e/internal/service/crd"
	svckube "kube2e/internal/service/kube"
)

type service struct {
	kube        *svckube.Service
	crd         *svccrd.Service
	labels      map[string]string
	annotations map[string]string
	logger      *slog.Logger
}

func Run(ctx context.Context, cfg *rest.Config, testDir, specificCase string, logger *slog.Logger) error {
	if cfg == nil {
		return fmt.Errorf("rest config is nil")
	}

	if testDir == "" {
		return fmt.Errorf("test dir is empty")
	}

	test, err := parseTestFile(testDir)
	if err != nil {
		return fmt.Errorf("parse test file from the '%s' dir: %w", testDir, err)
	}

	if test == nil {
		logger.Debug("skip dir due to lack of test file", "name", testDir)
		return nil
	}

	svc := new(service)

	svc.logger = logger.With("test", test.Name)

	opts := []svckube.Option{
		svckube.WithLabels(test.Labels),
		svckube.WithAnnotations(test.Annotations),
		svckube.WithNamespace(test.Namespace),
	}

	if svc.kube, err = svckube.New(cfg, svc.logger, opts...); err != nil {
		return fmt.Errorf("create kube service: %v", err)
	}

	if svc.crd, err = svccrd.New(svc.kube, test.CRDsDir(), svc.logger); err != nil {
		return fmt.Errorf("create crd service: %w", err)
	}

	svc.logger.Debug("test service initialized")

	return svc.run(ctx, test, specificCase)
}

func (s *service) run(ctx context.Context, test *Test, specificCase string) error {
	if test == nil {
		return fmt.Errorf("nil test")
	}

	s.logger.Debug("ensure crds")
	if err := s.crd.Ensure(ctx); err != nil {
		return fmt.Errorf("ensure crds: %w", err)
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
			return fmt.Errorf("ensure the '%s' namespace: %w", test.Namespace, err)
		}
	}

	err := test.forEach(func(caseDir string) error {
		if len(specificCase) != 0 && specificCase == caseDir {
			return nil
		}

		s.logger.Info("run case from dir", "name", caseDir)
		return testcase.Run(ctx, s.kube, caseDir, s.logger)
	})
	if err != nil {
		return fmt.Errorf("run the '%s' test: %w", test.Name, err)
	}

	if namespace != nil {
		s.logger.Debug("delete namespace", "name", namespace.Name)
		if err = s.kube.Delete(ctx, namespace); err != nil {
			return fmt.Errorf("delete the '%s' namespace: %w", test.Name, err)
		}
	}

	s.logger.Debug("delete crds")
	if err = s.crd.Delete(ctx); err != nil {
		return fmt.Errorf("delete crds: %w", err)
	}

	return nil
}
