package patch_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/ipaqsa/kube2e/internal/tools/patch"
)

// TestApply_ReplaceWithFalsyValue guards against omitempty dropping falsy
// values (false/0/""), which made the patch serialize without a value field
// and fail with "missing value".
func TestApply_ReplaceWithFalsyValue(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]any{"name": "d1"},
			"spec":       map[string]any{"paused": true},
		},
	}

	p := &patch.Patches{
		{
			Op:    "replace",
			Path:  "/spec/paused",
			Value: false,
		},
	}

	out, err := p.Apply(obj, nil)
	require.NoError(t, err)

	paused, found, err := unstructured.NestedBool(out.(*unstructured.Unstructured).Object, "spec", "paused")
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, paused)
}
