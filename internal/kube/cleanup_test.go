package kube_test

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/ipaqsa/kube2e/internal/kube"
)

// errDelete is a failure injected into Delete.
var errDelete = errors.New("delete failed")

// countDeletes returns interceptor funcs that count Delete calls and fail the
// ones for failName with errDelete.
func countDeletes(deletes *counter, failName string) interceptor.Funcs {
	return interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			deletes.inc()

			if obj.GetName() == failName {
				return errDelete
			}

			return c.Delete(ctx, obj, opts...)
		},
	}
}

// exists reports whether name of gvk is present in testNamespace.
func (s *ServiceSuite) exists(cli client.Client, gvk schema.GroupVersionKind, name string) bool {
	s.T().Helper()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	err := cli.Get(s.ctx, client.ObjectKey{Namespace: testNamespace, Name: name}, obj)
	if apierrors.IsNotFound(err) {
		return false
	}

	s.Require().NoError(err)

	return true
}

// TestCleanupDeletesCreatedObjects removes what Ensure created.
func (s *ServiceSuite) TestCleanupDeletesCreatedObjects() {
	svc, cli := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", nil)))
	s.Require().NoError(svc.Cleanup(s.ctx))

	s.False(s.exists(cli, configMapGVK, "cfg"))
}

// TestCleanupKeepsPreexistingObjects never deletes an object Ensure only
// updated.
func (s *ServiceSuite) TestCleanupKeepsPreexistingObjects() {
	live := newConfigMap("cfg", nil)
	live.SetNamespace(testNamespace)

	svc, cli := s.newService(interceptor.Funcs{}, live)

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", map[string]any{"k": "v"})))
	s.Require().NoError(svc.Cleanup(s.ctx))

	s.True(s.exists(cli, configMapGVK, "cfg"))
}

// TestCleanupSkipsUncachedObjects leaves objects ensured with caching off.
func (s *ServiceSuite) TestCleanupSkipsUncachedObjects() {
	svc, cli := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", nil), kube.WithEnsureToCache(false)))
	s.Require().NoError(svc.Cleanup(s.ctx))

	s.True(s.exists(cli, configMapGVK, "cfg"))
}

// TestCleanupTracksSameNameAcrossKinds deletes both objects when two kinds
// share a name, so neither overwrites the other in the cache.
func (s *ServiceSuite) TestCleanupTracksSameNameAcrossKinds() {
	svc, cli := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("app", nil)))
	s.Require().NoError(svc.Ensure(s.ctx, newWidget("app", "Ready")))
	s.Require().NoError(svc.Cleanup(s.ctx))

	s.False(s.exists(cli, configMapGVK, "app"))
	s.False(s.exists(cli, widgetGVK, "app"))
}

// TestCleanupSkipsObjectsDeletedMidCase does not delete again what a delete
// action already removed.
func (s *ServiceSuite) TestCleanupSkipsObjectsDeletedMidCase() {
	deletes := new(counter)
	svc, _ := s.newService(countDeletes(deletes, ""))

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", nil)))
	s.Require().NoError(svc.Delete(s.ctx, newConfigMap("cfg", nil)))
	s.Require().NoError(svc.Cleanup(s.ctx))

	s.Equal(1, deletes.count())
}

// TestCleanupContinuesPastFailures attempts every object and reports the
// failures together.
func (s *ServiceSuite) TestCleanupContinuesPastFailures() {
	deletes := new(counter)
	svc, cli := s.newService(countDeletes(deletes, "stuck"))

	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("stuck", nil)))
	s.Require().NoError(svc.Ensure(s.ctx, newConfigMap("cfg", nil)))

	err := svc.Cleanup(s.ctx)

	s.Require().ErrorIs(err, errDelete)
	s.Require().ErrorContains(err, "stuck")
	s.False(s.exists(cli, configMapGVK, "cfg"))
}
