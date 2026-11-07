package kube

import (
	"log/slog"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewForTest builds a Service around injected clients so black-box tests in
// package kube_test can run it against fakes. It exists only in test builds.
func NewForTest(cli client.Client, clientset kubernetes.Interface, logger *slog.Logger, opts ...Option) *Service {
	svc := newService(logger, opts...)
	svc.cli = cli
	svc.clientset = clientset
	svc.disc = clientset.Discovery()

	return svc
}
