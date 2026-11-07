package kube_test

import (
	"context"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/ipaqsa/kube2e/internal/kube"
)

const (
	// fastPoll keeps poll loops tight so passing waits finish in milliseconds.
	fastPoll = time.Millisecond
	// longTimeout bounds waits that are expected to pass; never reached.
	longTimeout = 5 * time.Second
	// shortTimeout is the deadline for waits that are expected to time out.
	shortTimeout = 50 * time.Millisecond
	// testFinalizer keeps a deleted object terminating until it is removed.
	testFinalizer = "example.com/hold"
)

// widgetGroupResource is the resource behind widgetGVK, for API errors.
var widgetGroupResource = schema.GroupResource{Group: "example.com", Resource: "widgets"}

// failGetFirst returns interceptor funcs whose Get fails with err for the first
// failures calls, then delegates; gets counts every Get.
func failGetFirst(gets *counter, failures int, err error) interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if gets.inc() <= failures {
				return err
			}

			return c.Get(ctx, key, obj, opts...)
		},
	}
}

// waitFast waits on obj with a tight interval and the given timeout.
func waitFast(timeout time.Duration, opts ...kube.WaitOptionFunc) []kube.WaitOptionFunc {
	return append([]kube.WaitOptionFunc{kube.WithWaitInterval(fastPoll), kube.WithWaitTimeout(timeout)}, opts...)
}

// TestWaitSeesStatusOnlyChange is the regression test for custom resources: a
// status write that leaves metadata.generation alone must still be observed.
func (s *ServiceSuite) TestWaitSeesStatusOnlyChange() {
	gets := new(counter)
	svc, _ := s.newService(readyOnGet(gets, 3), newWidget("w", "Pending"))

	err := svc.Wait(s.ctx, newWidget("w", ""), waitFast(longTimeout, kube.WithWaitConditions([]string{`.status.phase == "Ready"`}))...)

	s.Require().NoError(err)
	s.Equal(3, gets.count())
}

// TestWaitWithoutConditionsPassesOnceObjectExists succeeds on the first read
// of an existing object.
func (s *ServiceSuite) TestWaitWithoutConditionsPassesOnceObjectExists() {
	gets := new(counter)
	svc, _ := s.newService(failGetFirst(gets, 0, nil), newWidget("w", "Pending"))

	s.Require().NoError(svc.Wait(s.ctx, newWidget("w", ""), waitFast(longTimeout)...))
	s.Equal(1, gets.count())
}

// TestWaitKeepsPollingUntilObjectAppears treats NotFound as "not yet".
func (s *ServiceSuite) TestWaitKeepsPollingUntilObjectAppears() {
	gets := new(counter)
	notFound := apierrors.NewNotFound(widgetGroupResource, "w")
	svc, _ := s.newService(failGetFirst(gets, 2, notFound), newWidget("w", "Ready"))

	s.Require().NoError(svc.Wait(s.ctx, newWidget("w", ""), waitFast(longTimeout)...))
	s.Equal(3, gets.count())
}

// TestWaitKeepsPollingThroughTransientErrors rides out a temporarily
// unavailable API server.
func (s *ServiceSuite) TestWaitKeepsPollingThroughTransientErrors() {
	gets := new(counter)
	unavailable := apierrors.NewServiceUnavailable("apiserver restarting")
	svc, _ := s.newService(failGetFirst(gets, 2, unavailable), newWidget("w", "Ready"))

	s.Require().NoError(svc.Wait(s.ctx, newWidget("w", ""), waitFast(longTimeout)...))
	s.Equal(3, gets.count())
}

// TestWaitFailsFastOnForbidden ends the wait on the first non-transient error
// instead of polling until the timeout.
func (s *ServiceSuite) TestWaitFailsFastOnForbidden() {
	gets := new(counter)
	forbidden := apierrors.NewForbidden(widgetGroupResource, "w", errors.New("rbac"))
	svc, _ := s.newService(failGetFirst(gets, 100, forbidden), newWidget("w", "Ready"))

	err := svc.Wait(s.ctx, newWidget("w", ""), waitFast(longTimeout)...)

	s.Require().Error(err)
	s.True(apierrors.IsForbidden(err))
	s.Equal(1, gets.count())
}

// TestWaitFailsFastOnInvalidExpression does not poll a condition that can
// never evaluate.
func (s *ServiceSuite) TestWaitFailsFastOnInvalidExpression() {
	gets := new(counter)
	svc, _ := s.newService(failGetFirst(gets, 0, nil), newWidget("w", "Ready"))

	err := svc.Wait(s.ctx, newWidget("w", ""), waitFast(longTimeout, kube.WithWaitConditions([]string{`.status.phase ==`}))...)

	s.Require().ErrorContains(err, "evaluate condition")
	s.Equal(1, gets.count())
}

// TestWaitTimeoutReportsLastState keeps DeadlineExceeded in the chain and says
// which condition was still unmet.
func (s *ServiceSuite) TestWaitTimeoutReportsLastState() {
	svc, _ := s.newService(interceptor.Funcs{}, newWidget("w", "Pending"))

	err := svc.Wait(s.ctx, newWidget("w", ""), waitFast(shortTimeout, kube.WithWaitConditions([]string{`.status.phase == "Ready"`}))...)

	s.Require().ErrorIs(err, context.DeadlineExceeded)
	s.Require().ErrorContains(err, `condition not satisfied: .status.phase == "Ready"`)
}

// TestWaitStopsOnParentCancellation returns the caller's cancellation.
func (s *ServiceSuite) TestWaitStopsOnParentCancellation() {
	svc, _ := s.newService(interceptor.Funcs{}, newWidget("w", "Pending"))

	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	err := svc.Wait(ctx, newWidget("w", ""), waitFast(longTimeout, kube.WithWaitConditions([]string{`.status.phase == "Ready"`}))...)

	s.Require().ErrorIs(err, context.Canceled)
}

// TestWaitUnknownKind fails at once on a kind the API server does not serve.
func (s *ServiceSuite) TestWaitUnknownKind() {
	svc, _ := s.newService(interceptor.Funcs{})

	ghost := &unstructured.Unstructured{}
	ghost.SetGroupVersionKind(schema.GroupVersionKind{Group: "nope.io", Version: "v1", Kind: "Ghost"})
	ghost.SetName("g")

	err := svc.Wait(s.ctx, ghost, waitFast(longTimeout)...)

	s.Require().Error(err)
	s.True(meta.IsNoMatchError(err))
}

// TestDeleteMissingObjectIsNotAnError treats NotFound as already deleted.
func (s *ServiceSuite) TestDeleteMissingObjectIsNotAnError() {
	svc, _ := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.Delete(s.ctx, newConfigMap("cfg", nil)))
}

// TestDeleteRetries retries a failed delete within the attempt budget.
func (s *ServiceSuite) TestDeleteRetries() {
	live := newConfigMap("cfg", nil)
	live.SetNamespace(testNamespace)

	deletes := new(counter)
	svc, cli := s.newService(interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if deletes.inc() == 1 {
				return errDelete
			}

			return c.Delete(ctx, obj, opts...)
		},
	}, live)

	s.Require().NoError(svc.Delete(s.ctx, newConfigMap("cfg", nil), kube.WithDeleteRetry(2, 0)))

	s.Equal(2, deletes.count())
	s.False(s.exists(cli, configMapGVK, "cfg"))
}

// TestDeleteWaitBlocksUntilFinalizerIsRemoved waits through the terminating
// phase until the object is really gone.
func (s *ServiceSuite) TestDeleteWaitBlocksUntilFinalizerIsRemoved() {
	held := newWidget("w", "Ready")
	held.SetFinalizers([]string{testFinalizer})

	gets := new(counter)
	svc, cli := s.newService(releaseFinalizerOnGet(gets, 3), held)

	err := svc.Delete(s.ctx, newWidget("w", ""), kube.WithDeleteWait(true, longTimeout), kube.WithDeleteInterval(fastPoll))

	s.Require().NoError(err)
	s.GreaterOrEqual(gets.count(), 3)
	s.False(s.exists(cli, widgetGVK, "w"))
}

// TestDeleteWaitTimesOutOnStuckFinalizer reports a terminating object that
// never goes away.
func (s *ServiceSuite) TestDeleteWaitTimesOutOnStuckFinalizer() {
	held := newWidget("w", "Ready")
	held.SetFinalizers([]string{testFinalizer})

	svc, _ := s.newService(interceptor.Funcs{}, held)

	err := svc.Delete(s.ctx, newWidget("w", ""), kube.WithDeleteWait(true, shortTimeout), kube.WithDeleteInterval(fastPoll))

	s.Require().ErrorIs(err, context.DeadlineExceeded)
	s.Require().ErrorContains(err, "object is terminating")
}

// releaseFinalizerOnGet drops the finalizers of a terminating object from the
// releaseAt-th Get on, the way its controller would, so the fake deletes it.
func releaseFinalizerOnGet(gets *counter, releaseAt int) interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}

			if gets.inc() < releaseAt || obj.GetDeletionTimestamp() == nil {
				return nil
			}

			obj.SetFinalizers(nil)

			if err := c.Update(ctx, obj); err != nil {
				return err
			}

			return c.Get(ctx, key, obj, opts...)
		},
	}
}
