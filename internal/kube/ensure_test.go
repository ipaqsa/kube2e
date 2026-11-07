package kube_test

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	interrors "github.com/ipaqsa/kube2e/internal/errors"
	"github.com/ipaqsa/kube2e/internal/kube"
)

// errApply is a non-API failure injected into Apply.
var errApply = errors.New("apply failed")

// failApply returns interceptor funcs whose Apply fails with err for the first
// failures calls, then delegates; calls counts every Apply.
func failApply(calls *counter, failures int, err error) interceptor.Funcs {
	return interceptor.Funcs{
		Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
			if calls.inc() <= failures {
				return err
			}

			return c.Apply(ctx, obj, opts...)
		},
	}
}

// TestEnsureCreatesObjectInFallbackNamespace applies a namespace-less object
// into the service namespace.
func (s *ServiceSuite) TestEnsureCreatesObjectInFallbackNamespace() {
	svc, cli := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", map[string]any{"k": "v"})))

	got := s.liveConfigMap(cli)
	s.Equal(map[string]any{"k": "v"}, got.Object["data"])
}

// TestEnsureMergesAnnotations adds engine annotations without dropping the
// template's own.
func (s *ServiceSuite) TestEnsureMergesAnnotations() {
	svc, cli := s.newService(interceptor.Funcs{})

	obj := newConfigMap("cfg", nil)
	obj.SetAnnotations(map[string]string{"own": "1"})

	s.Require().NoError(svc.Ensure(s.ctx, obj, kube.WithEnsureAnnotations(map[string]string{"kube2e/case": "c"})))

	s.Equal(map[string]string{"own": "1", "kube2e/case": "c"}, s.liveConfigMap(cli).GetAnnotations())
}

// TestEnsureClusterScopedKeepsNoNamespace leaves cluster-scoped objects
// without a namespace.
func (s *ServiceSuite) TestEnsureClusterScopedKeepsNoNamespace() {
	svc, cli := s.newService(interceptor.Funcs{})

	ns := &unstructured.Unstructured{}
	ns.SetGroupVersionKind(namespaceGVK)
	ns.SetName("team")

	s.Require().NoError(svc.Ensure(s.ctx, ns))
	s.Empty(ns.GetNamespace())

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(namespaceGVK)
	s.Require().NoError(cli.Get(s.ctx, client.ObjectKey{Name: "team"}, got))
}

// TestEnsureTypedObject applies a typed object, the way the engine ensures the
// run namespace, by converting it to unstructured with its TypeMeta.
func (s *ServiceSuite) TestEnsureTypedObject() {
	svc, cli := s.newService(interceptor.Funcs{})

	ns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "team"},
	}

	s.Require().NoError(svc.Ensure(s.ctx, ns, kube.WithEnsureToCache(false)))

	got := &corev1.Namespace{}
	s.Require().NoError(cli.Get(s.ctx, client.ObjectKey{Name: "team"}, got))
}

// TestEnsureRetriesUntilSuccess retries any error up to the attempt budget.
func (s *ServiceSuite) TestEnsureRetriesUntilSuccess() {
	calls := new(counter)
	svc, cli := s.newService(failApply(calls, 2, errApply))

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", nil), kube.WithEnsureRetry(3, 0)))

	s.Equal(3, calls.count())
	s.liveConfigMap(cli)
}

// TestEnsureFailsWhenAttemptsAreSpent returns the last error once the budget
// is exhausted.
func (s *ServiceSuite) TestEnsureFailsWhenAttemptsAreSpent() {
	calls := new(counter)
	svc, _ := s.newService(failApply(calls, 5, errApply))

	err := svc.Ensure(s.ctx, newConfigMap("cfg", nil), kube.WithEnsureRetry(2, 0))

	s.Require().ErrorIs(err, errApply)
	s.Equal(2, calls.count())
}

// TestEnsureRetriesTransientServerErrorsWithoutUserRetry absorbs a server
// timeout through the built-in retry even with a single user attempt.
func (s *ServiceSuite) TestEnsureRetriesTransientServerErrorsWithoutUserRetry() {
	calls := new(counter)
	timeout := apierrors.NewServerTimeout(schema.GroupResource{Resource: "configmaps"}, "apply", 0)
	svc, _ := s.newService(failApply(calls, 1, timeout))

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", nil)))
	s.Equal(2, calls.count())
}

// TestEnsureCanceledContextMakesNoAttempt starts no attempt on a done context
// and keeps the cancellation in the error chain.
func (s *ServiceSuite) TestEnsureCanceledContextMakesNoAttempt() {
	calls := new(counter)
	svc, _ := s.newService(failApply(calls, 0, nil))

	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	err := svc.Ensure(ctx, newConfigMap("cfg", nil), kube.WithEnsureRetry(3, 0))

	s.Require().ErrorIs(err, context.Canceled)
	s.Zero(calls.count())
}

// TestEnsureNilObject rejects a nil object.
func (s *ServiceSuite) TestEnsureNilObject() {
	svc, _ := s.newService(interceptor.Funcs{})

	s.Require().ErrorIs(svc.Ensure(s.ctx, nil), interrors.ErrNilObject)
}

// TestEnsureUnknownKind fails on a kind the API server does not serve.
func (s *ServiceSuite) TestEnsureUnknownKind() {
	svc, _ := s.newService(interceptor.Funcs{})

	ghost := &unstructured.Unstructured{}
	ghost.SetGroupVersionKind(schema.GroupVersionKind{Group: "nope.io", Version: "v1", Kind: "Ghost"})
	ghost.SetName("g")

	err := svc.Ensure(s.ctx, ghost)

	s.Require().Error(err)
	s.True(meta.IsNoMatchError(err))
}
