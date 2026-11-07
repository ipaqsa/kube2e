package kube_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/ipaqsa/kube2e/internal/kube"
)

// testNamespace is the fallback namespace every test Service is built with.
const testNamespace = "e2e"

var (
	// configMapGVK is a built-in namespaced kind with no status semantics.
	configMapGVK = schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	// namespaceGVK is a built-in cluster-scoped kind.
	namespaceGVK = schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}
	// podGVK is the kind logs targets resolve to.
	podGVK = schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
	// deploymentGVK is a workload kind logs and exec resolve to its pods.
	deploymentGVK = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	// widgetGVK is a custom resource kind unknown to the client scheme.
	widgetGVK = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}
)

// ServiceSuite exercises kube.Service black-box against fake clients.
type ServiceSuite struct {
	suite.Suite

	ctx       context.Context
	clientset *k8sfake.Clientset
}

func TestServiceSuite(t *testing.T) {
	t.Parallel()

	suite.Run(t, new(ServiceSuite))
}

// SetupTest gives every test a fresh context and an empty fake clientset.
func (s *ServiceSuite) SetupTest() {
	s.ctx = context.Background()
	s.clientset = k8sfake.NewClientset()
}

// newService builds a Service over a fresh fake client seeded with objs, with
// funcs intercepting its calls, and over the suite's fake clientset. The fake
// client is returned for assertions.
func (s *ServiceSuite) newService(funcs interceptor.Funcs, objs ...client.Object) (*kube.Service, client.WithWatch) {
	cli := fake.NewClientBuilder().
		WithScheme(clientgoscheme.Scheme).
		WithRESTMapper(newMapper()).
		WithObjects(objs...).
		WithInterceptorFuncs(funcs).
		Build()

	return kube.NewForTest(cli, s.clientset, discardLogger(), kube.WithNamespace(testNamespace)), cli
}

// newMapper knows the kinds the tests use, with their real scopes.
func newMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(configMapGVK, meta.RESTScopeNamespace)
	mapper.Add(namespaceGVK, meta.RESTScopeRoot)
	mapper.Add(podGVK, meta.RESTScopeNamespace)
	mapper.Add(widgetGVK, meta.RESTScopeNamespace)
	mapper.Add(deploymentGVK, meta.RESTScopeNamespace)

	return mapper
}

// discardLogger keeps test output free of service debug logs.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// newConfigMap renders an unstructured ConfigMap the way templates do: no
// namespace, so the service fallback applies.
func newConfigMap(name string, data map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(configMapGVK)
	obj.SetName(name)

	if data != nil {
		obj.Object["data"] = data
	}

	return obj
}

// newWidget renders an unstructured custom resource with a status phase.
func newWidget(name, phase string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(widgetGVK)
	obj.SetName(name)
	obj.SetNamespace(testNamespace)
	obj.SetGeneration(1)
	obj.Object["status"] = map[string]any{"phase": phase}

	return obj
}

// newPod builds a typed pod in testNamespace with the given labels and phase.
func newPod(name string, labels map[string]string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace, Labels: labels},
		Status:     corev1.PodStatus{Phase: phase},
	}
}

// addPods seeds the fake clientset that serves pod lists and logs.
func (s *ServiceSuite) addPods(pods ...*corev1.Pod) {
	s.T().Helper()

	for _, pod := range pods {
		_, err := s.clientset.CoreV1().Pods(pod.Namespace).Create(s.ctx, pod, metav1.CreateOptions{})
		s.Require().NoError(err)
	}
}

// liveConfigMap reads the "cfg" ConfigMap from cli in testNamespace, failing
// the test when it is missing.
func (s *ServiceSuite) liveConfigMap(cli client.Client) *unstructured.Unstructured {
	s.T().Helper()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(configMapGVK)
	s.Require().NoError(cli.Get(s.ctx, client.ObjectKey{Namespace: testNamespace, Name: "cfg"}, obj))

	return obj
}

// counter counts intercepted calls; atomic because the poll loops call from
// their own goroutine.
type counter struct{ n atomic.Int32 }

// inc records a call and returns its 1-based ordinal.
func (c *counter) inc() int { return int(c.n.Add(1)) }

// count returns how many calls were recorded.
func (c *counter) count() int { return int(c.n.Load()) }
