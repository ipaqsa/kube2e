package kube_test

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/ipaqsa/kube2e/internal/kube"
	"github.com/ipaqsa/kube2e/internal/tools/patch"
)

// TestPatchModifiesLiveObject patches the object on the cluster, keeping
// fields the rendered template does not carry.
func (s *ServiceSuite) TestPatchModifiesLiveObject() {
	live := newConfigMap("cfg", map[string]any{"set-by-earlier-step": "yes"})
	live.SetNamespace(testNamespace)

	svc, cli := s.newService(interceptor.Funcs{}, live)

	patches := patch.Patches{{Op: "add", Path: "/data/added", Value: "1"}}
	s.Require().NoError(svc.Patch(s.ctx, newConfigMap("cfg", nil), patches))

	s.Equal(map[string]any{"set-by-earlier-step": "yes", "added": "1"}, s.liveConfigMap(cli).Object["data"])
}

// TestPatchRetriesOnConflict re-reads and re-applies after an optimistic
// concurrency conflict.
func (s *ServiceSuite) TestPatchRetriesOnConflict() {
	live := newConfigMap("cfg", nil)
	live.SetNamespace(testNamespace)

	updates := new(counter)
	svc, cli := s.newService(interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if updates.inc() == 1 {
				return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, obj.GetName(), nil)
			}

			return c.Update(ctx, obj, opts...)
		},
	}, live)

	patches := patch.Patches{{Op: "add", Path: "/data", Value: map[string]any{"k": "v"}}}
	s.Require().NoError(svc.Patch(s.ctx, newConfigMap("cfg", nil), patches))

	s.Equal(2, updates.count())
	s.Equal(map[string]any{"k": "v"}, s.liveConfigMap(cli).Object["data"])
}

// TestPatchRetriesWithinAttemptBudget retries a failed read under the user
// retry policy.
func (s *ServiceSuite) TestPatchRetriesWithinAttemptBudget() {
	live := newConfigMap("cfg", nil)
	live.SetNamespace(testNamespace)

	gets := new(counter)
	svc, cli := s.newService(failGetFirst(gets, 1, errApply), live)

	patches := patch.Patches{{Op: "add", Path: "/data", Value: map[string]any{"k": "v"}}}
	s.Require().NoError(svc.Patch(s.ctx, newConfigMap("cfg", nil), patches, kube.WithPatchRetry(2, 0)))

	s.Equal(map[string]any{"k": "v"}, s.liveConfigMap(cli).Object["data"])
}

// TestPatchMissingObject fails when there is no live object to patch.
func (s *ServiceSuite) TestPatchMissingObject() {
	svc, _ := s.newService(interceptor.Funcs{})

	err := svc.Patch(s.ctx, newConfigMap("cfg", nil), patch.Patches{{Op: "add", Path: "/data", Value: map[string]any{}}})

	s.Require().Error(err)
	s.True(apierrors.IsNotFound(err))
}

// TestPatchInvalidOperation surfaces a patch that cannot apply to the object.
func (s *ServiceSuite) TestPatchInvalidOperation() {
	live := newConfigMap("cfg", nil)
	live.SetNamespace(testNamespace)

	svc, _ := s.newService(interceptor.Funcs{}, live)

	err := svc.Patch(s.ctx, newConfigMap("cfg", nil), patch.Patches{{Op: "remove", Path: "/data/missing"}})

	s.Require().ErrorContains(err, "apply patches")
}

// TestCheckPasses succeeds when every condition holds on the live object.
func (s *ServiceSuite) TestCheckPasses() {
	svc, _ := s.newService(interceptor.Funcs{}, newWidget("w", "Ready"))

	s.Require().NoError(svc.Check(s.ctx, newWidget("w", ""), []string{`.status.phase == "Ready"`, `.metadata.name == "w"`}))
}

// TestCheckReportsUnmetCondition names the first condition that fails.
func (s *ServiceSuite) TestCheckReportsUnmetCondition() {
	svc, _ := s.newService(interceptor.Funcs{}, newWidget("w", "Pending"))

	err := svc.Check(s.ctx, newWidget("w", ""), []string{`.status.phase == "Ready"`})

	s.Require().ErrorContains(err, `condition not satisfied: .status.phase == "Ready"`)
}

// TestCheckRetriesUntilConditionHolds re-reads the object on every attempt.
func (s *ServiceSuite) TestCheckRetriesUntilConditionHolds() {
	gets := new(counter)
	svc, _ := s.newService(readyOnGet(gets, 2), newWidget("w", "Pending"))

	s.Require().NoError(svc.Check(s.ctx, newWidget("w", ""), []string{`.status.phase == "Ready"`}, kube.WithCheckRetry(3, 0)))
	s.Equal(2, gets.count())
}

// TestCheckInvalidExpression fails on a JQ expression that does not compile.
func (s *ServiceSuite) TestCheckInvalidExpression() {
	svc, _ := s.newService(interceptor.Funcs{}, newWidget("w", "Ready"))

	s.Require().ErrorContains(svc.Check(s.ctx, newWidget("w", ""), []string{`.status.phase ==`}), "evaluate condition")
}

// TestCheckMissingObject fails when the object does not exist.
func (s *ServiceSuite) TestCheckMissingObject() {
	svc, _ := s.newService(interceptor.Funcs{})

	err := svc.Check(s.ctx, newWidget("w", ""), nil)

	s.Require().Error(err)
	s.True(apierrors.IsNotFound(err))
}

// TestFilterReturnsValue extracts a value from the live object.
func (s *ServiceSuite) TestFilterReturnsValue() {
	live := newConfigMap("cfg", map[string]any{"host": "db.local"})
	live.SetNamespace(testNamespace)

	svc, _ := s.newService(interceptor.Funcs{}, live)

	value, err := svc.Filter(s.ctx, newConfigMap("cfg", nil), ".data.host")

	s.Require().NoError(err)
	s.Equal("db.local", value)
}

// readyOnGet flips a Widget's status.phase to Ready from the readyAt-th Get on,
// simulating a controller writing status between polls without bumping
// metadata.generation.
func readyOnGet(gets *counter, readyAt int) interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}

			if gets.inc() >= readyAt {
				if u, ok := obj.(interface{ UnstructuredContent() map[string]any }); ok {
					u.UnstructuredContent()["status"] = map[string]any{"phase": "Ready"}
				}
			}

			return nil
		},
	}
}
