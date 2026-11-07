package kube_test

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	interrors "github.com/ipaqsa/kube2e/internal/errors"
	"github.com/ipaqsa/kube2e/internal/kube"
	"github.com/ipaqsa/kube2e/internal/tools/patch"
)

// fakeLogLine is the body client-go's fake clientset serves for every pod log.
const fakeLogLine = "fake logs"

// appLabels selects the pods seeded by logs tests.
var appLabels = map[string]string{"app": "web"}

// webPods is a label-selector target matching appLabels.
var webPods = kube.PodTarget{Kind: "Pod", LabelSelector: "app=web"}

// logsFast polls logs on a tight interval with the given policy and window.
func logsFast(match kube.LogsMatch, timeout time.Duration) []kube.LogsOptionFunc {
	return []kube.LogsOptionFunc{kube.WithLogsMatch(match), kube.WithLogsInterval(fastPoll), kube.WithLogsTimeout(timeout)}
}

// TestLogsAnyFindsString passes once a running pod's logs contain the string.
func (s *ServiceSuite) TestLogsAnyFindsString() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.LogsContains(s.ctx, webPods, "fake", logsFast(kube.LogsMatchAny, longTimeout)...))
}

// TestLogsAllRequiresEveryPod passes when every resolved pod matches.
func (s *ServiceSuite) TestLogsAllRequiresEveryPod() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning), newPod("web-2", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.LogsContains(s.ctx, webPods, "fake", logsFast(kube.LogsMatchAll, longTimeout)...))
}

// TestLogsAnyTimesOutWithoutMatch fails with ErrLogsNotContain and keeps the
// deadline in the chain.
func (s *ServiceSuite) TestLogsAnyTimesOutWithoutMatch() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{})

	err := svc.LogsContains(s.ctx, webPods, "absent", logsFast(kube.LogsMatchAny, shortTimeout)...)

	s.Require().ErrorIs(err, interrors.ErrLogsNotContain)
	s.Require().ErrorIs(err, context.DeadlineExceeded)
}

// TestLogsSkipsFailedPods never reads a Failed pod, so with no other pod the
// check cannot pass.
func (s *ServiceSuite) TestLogsSkipsFailedPods() {
	s.addPods(newPod("web-1", appLabels, corev1.PodFailed))
	svc, _ := s.newService(interceptor.Funcs{})

	err := svc.LogsContains(s.ctx, webPods, "fake", logsFast(kube.LogsMatchAny, shortTimeout)...)

	s.Require().ErrorIs(err, interrors.ErrLogsNotContain)
}

// TestLogsNonePassesAfterCleanWindow passes only once the window elapses
// without the forbidden string.
func (s *ServiceSuite) TestLogsNonePassesAfterCleanWindow() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{})

	s.Require().NoError(svc.LogsContains(s.ctx, webPods, "absent", logsFast(kube.LogsMatchNone, shortTimeout)...))
}

// TestLogsNoneFailsOnForbiddenString fails as soon as the string shows up.
func (s *ServiceSuite) TestLogsNoneFailsOnForbiddenString() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{})

	err := svc.LogsContains(s.ctx, webPods, fakeLogLine, logsFast(kube.LogsMatchNone, longTimeout)...)

	s.Require().ErrorIs(err, interrors.ErrLogsContainForbidden)
}

// TestLogsNoneFailsWithoutPods never turns unread logs into a pass.
func (s *ServiceSuite) TestLogsNoneFailsWithoutPods() {
	svc, _ := s.newService(interceptor.Funcs{})

	err := svc.LogsContains(s.ctx, webPods, "absent", logsFast(kube.LogsMatchNone, longTimeout)...)

	s.Require().ErrorIs(err, interrors.ErrLogsUnavailable)
}

// TestLogsNoneFailsOnCancellation does not pass when the caller gives up
// before the window elapses.
func (s *ServiceSuite) TestLogsNoneFailsOnCancellation() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{})

	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	err := svc.LogsContains(ctx, webPods, "absent", logsFast(kube.LogsMatchNone, longTimeout)...)

	s.Require().ErrorIs(err, context.Canceled)
}

// TestLogsRejectsEmptyTarget needs an object or a kind with a selector.
func (s *ServiceSuite) TestLogsRejectsEmptyTarget() {
	svc, _ := s.newService(interceptor.Funcs{})

	s.Require().ErrorIs(svc.LogsContains(s.ctx, kube.PodTarget{}, "x"), interrors.ErrNilObject)
}

// TestGetVersionReportsServerVersion reads the version from discovery.
func (s *ServiceSuite) TestGetVersionReportsServerVersion() {
	disc, ok := s.clientset.Discovery().(*fakediscovery.FakeDiscovery)
	s.Require().True(ok)

	disc.FakedServerVersion = &version.Info{GitVersion: "v1.33.2"}

	svc, _ := s.newService(interceptor.Funcs{})

	got, err := svc.GetVersion(s.ctx)

	s.Require().NoError(err)
	s.Equal("v1.33.2", got)
}

// TestDryRunTouchesNothing runs every operation without a cluster: New must
// not need a rest config, and nothing may be written.
func (s *ServiceSuite) TestDryRunTouchesNothing() {
	svc, err := kube.New(nil, discardLogger(), kube.WithDryRun())
	s.Require().NoError(err)

	obj := newConfigMap("cfg", nil)

	s.Require().NoError(svc.Ensure(s.ctx, obj))
	s.Require().NoError(svc.Patch(s.ctx, obj, patch.Patches{{Op: "add", Path: "/data", Value: map[string]any{}}}))
	s.Require().NoError(svc.Wait(s.ctx, obj))
	s.Require().NoError(svc.Check(s.ctx, obj, []string{"false"}))
	s.Require().NoError(svc.LogsContains(s.ctx, webPods, "x"))
	s.Require().NoError(svc.Exec(s.ctx, webPods, []string{"true"}))
	s.Require().NoError(svc.Delete(s.ctx, obj))
	s.Require().NoError(svc.Cleanup(s.ctx))

	got, err := svc.GetVersion(s.ctx)
	s.Require().NoError(err)
	s.Equal("dev", got)
}
