package filter

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name      string
		cond      string
		obj       map[string]interface{}
		pass      bool
		err       bool
		errSubstr string
	}

	tests := []testCase{
		// --- basics
		{
			name: "evaluates true",
			cond: ".a == 1",
			obj:  map[string]interface{}{"a": int64(1)},
			pass: true,
		},
		{
			name: "evaluates false",
			cond: ".a == 2",
			obj:  map[string]interface{}{"a": int64(1)},
			pass: false,
		},
		{
			name: "empty produces no result -> pass=false",
			cond: "empty",
			obj:  map[string]interface{}{"a": int64(1)},
			pass: false,
		},
		{
			name:      "parse error",
			cond:      ".[", // invalid jq
			obj:       map[string]interface{}{"a": int64(1)},
			err:       true,
			errSubstr: "parse",
		},
		{
			name:      "non-boolean result is error",
			cond:      ".a", // yields a number, not bool
			obj:       map[string]interface{}{"a": int64(1)},
			err:       true,
			errSubstr: "boolean",
		},

		// --- Pod filters
		{
			name: "pod ready true (Ready condition True)",
			cond: `any(.status.conditions[]?; .type=="Ready" and .status=="True")`,
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Initialized", "status": "True"},
						map[string]interface{}{"type": "Ready", "status": "True"},
						map[string]interface{}{"type": "ContainersReady", "status": "True"},
						map[string]interface{}{"type": "PodScheduled", "status": "True"},
					},
				},
			},
			pass: true,
		},
		{
			name: "pod not ready (Ready condition False)",
			cond: `any(.status.conditions[]?; .type=="Ready" and .status=="True")`,
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Initialized", "status": "True"},
						map[string]interface{}{"type": "Ready", "status": "False"},
					},
				},
			},
			pass: false,
		},
		{
			name: "pod missing conditions -> treated as false",
			cond: `any(.status.conditions[]?; .type=="Ready" and .status=="True")`,
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"status":     map[string]interface{}{}, // no conditions
			},
			pass: false,
		},

		// --- Deployment filters
		{
			name: "deployment available and fully ready",
			cond: `any(.status.conditions[]?; .type=="Available" and .status=="True") and (.status.readyReplicas == .spec.replicas)`,
			obj: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(3),
					"conditions": []interface{}{
						map[string]interface{}{"type": "Available", "status": "True"},
					},
				},
			},
			pass: true,
		},
		{
			name: "deployment not fully ready (readyReplicas < spec.replicas)",
			cond: `any(.status.conditions[]?; .type=="Available" and .status=="True") and (.status.readyReplicas == .spec.replicas)`,
			obj: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(1),
					"conditions": []interface{}{
						map[string]interface{}{"type": "Available", "status": "True"},
					},
				},
			},
			pass: false,
		},
		{
			name: "deployment not available",
			cond: `any(.status.conditions[]?; .type=="Available" and .status=="True")`,
			obj: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Available", "status": "False"},
					},
				},
			},
			pass: false,
		},

		// --- Service filters
		{
			name: "service exposes port 80",
			cond: `any(.spec.ports[]?; .port == 80)`,
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"spec": map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{"port": int64(80), "targetPort": int64(8080)},
					},
				},
			},
			pass: true,
		},
		{
			name: "service without ports -> false",
			cond: `any(.spec.ports[]?; .port == 80)`,
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"spec":       map[string]interface{}{}, // no ports
			},
			pass: false,
		},
		{
			name: "service ClusterIP assigned",
			cond: `(.spec.type=="ClusterIP" or .spec.type==null) and (.spec.clusterIP? // "") | tostring | length > 0 and . != "None"`,
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"spec": map[string]interface{}{
					"type":      "ClusterIP",
					"clusterIP": "10.96.0.10",
				},
			},
			pass: true,
		},
		{
			name: "service ClusterIP None -> false",
			cond: `(.spec.type? // "ClusterIP") == "ClusterIP" and ((.spec.clusterIP? // "") as $ip | ($ip | tostring | length) > 0 and $ip != "None")`,
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"spec": map[string]interface{}{
					"type":      "ClusterIP",
					"clusterIP": "None",
				},
			},
			pass: false,
		},

		// --- edge cases
		{
			name: "bad type for .status.conditions -> jq runtime error",
			cond: `any(.status.conditions[]; .type=="Ready" and .status=="True")`, // no ? to force iterate
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"status": map[string]interface{}{
					"conditions": map[string]interface{}{"bogus": "not-an-array"},
				},
			},
			err: true,
		},
	}

	ctx := context.Background()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			obj := new(unstructured.Unstructured)
			obj.SetUnstructuredContent(test.obj)

			pass, err := Pass(ctx, obj, test.cond)
			if test.err {
				if err == nil {
					t.Fatalf("expected error, got nil (pass=%v)", pass)
				}

				if test.errSubstr != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.errSubstr)) {
					t.Fatalf("expected error containing %q, got %q", test.errSubstr, err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if pass != test.pass {
				t.Fatalf("pass: got %v, should %v", pass, test.pass)
			}
		})
	}
}
