package kube_test

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/ipaqsa/kube2e/internal/kube"
)

// newWebDeployment renders the unstructured "web" Deployment; nil matchLabels
// leaves the selector out entirely.
func newWebDeployment(labels map[string]string, matchLabels map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(deploymentGVK)
	obj.SetName("web")
	obj.SetNamespace(testNamespace)
	obj.SetLabels(labels)

	spec := map[string]any{}
	if matchLabels != nil {
		spec["selector"] = map[string]any{"matchLabels": matchLabels}
	}

	obj.Object["spec"] = spec

	return obj
}

// TestLogsResolveDeploymentObjectToItsPods reads the pods a named Deployment
// selects through spec.selector.matchLabels.
func (s *ServiceSuite) TestLogsResolveDeploymentObjectToItsPods() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{}, newWebDeployment(nil, map[string]any{"app": "web"}))

	target := kube.PodTarget{Object: newWebDeployment(nil, nil)}

	s.Require().NoError(svc.LogsContains(s.ctx, target, "fake", logsFast(kube.LogsMatchAny, longTimeout)...))
}

// TestLogsResolveDeploymentBySelector finds the workload by label first, then
// its pods.
func (s *ServiceSuite) TestLogsResolveDeploymentBySelector() {
	s.addPods(newPod("web-1", appLabels, corev1.PodRunning))
	svc, _ := s.newService(interceptor.Funcs{}, newWebDeployment(map[string]string{"tier": "front"}, map[string]any{"app": "web"}))

	target := kube.PodTarget{Kind: "Deployment", LabelSelector: "tier=front"}

	s.Require().NoError(svc.LogsContains(s.ctx, target, "fake", logsFast(kube.LogsMatchAny, longTimeout)...))
}

// TestLogsNoneReportsWorkloadWithoutSelector explains why the target could
// not be resolved instead of passing.
func (s *ServiceSuite) TestLogsNoneReportsWorkloadWithoutSelector() {
	svc, _ := s.newService(interceptor.Funcs{}, newWebDeployment(nil, nil))

	target := kube.PodTarget{Object: newWebDeployment(nil, nil)}

	err := svc.LogsContains(s.ctx, target, "absent", logsFast(kube.LogsMatchNone, longTimeout)...)

	s.Require().ErrorContains(err, "no spec.selector.matchLabels")
}

// TestLogsNoneReportsMissingWorkloadBySelector fails when no workload carries
// the selector's labels.
func (s *ServiceSuite) TestLogsNoneReportsMissingWorkloadBySelector() {
	svc, _ := s.newService(interceptor.Funcs{})

	target := kube.PodTarget{Kind: "Deployment", LabelSelector: "tier=front"}

	err := svc.LogsContains(s.ctx, target, "absent", logsFast(kube.LogsMatchNone, longTimeout)...)

	s.Require().ErrorContains(err, "no Deployment matching 'tier=front'")
}

// TestLogsNoneRejectsUnsupportedKind names the kinds a target may use.
func (s *ServiceSuite) TestLogsNoneRejectsUnsupportedKind() {
	svc, _ := s.newService(interceptor.Funcs{})

	target := kube.PodTarget{Kind: "Job", LabelSelector: "app=web"}

	err := svc.LogsContains(s.ctx, target, "absent", logsFast(kube.LogsMatchNone, longTimeout)...)

	s.Require().ErrorContains(err, `unsupported kind "Job"`)
}

// TestExecFailsWithoutRunningPod resolves only Running pods and retries the
// resolution within the attempt budget before giving up.
func (s *ServiceSuite) TestExecFailsWithoutRunningPod() {
	s.addPods(newPod("web-1", appLabels, corev1.PodPending))
	svc, _ := s.newService(interceptor.Funcs{})

	err := svc.Exec(s.ctx, webPods, []string{"true"}, kube.WithExecContainer("app"), kube.WithExecRetry(2, 0))

	s.Require().ErrorContains(err, "no running pod found for selector 'app=web'")
}
