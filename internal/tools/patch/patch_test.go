package patch

import (
	"encoding/json"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// --- Parsing tests -----------------------------------------------------------

func TestParsePatchJSON(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"op":"add","path":"/metadata/labels/app","value":"demo"}`)
	parsed := new(Patch)
	require.NoError(t, json.Unmarshal(raw, parsed))
	require.Equal(t, "add", parsed.Op)
	require.Equal(t, "/metadata/labels/app", parsed.Path)
	require.Equal(t, "demo", parsed.Value)
}

func TestParsePatchesJSON(t *testing.T) {
	t.Parallel()

	raw := []byte(`[{"op":"add","path":"/metadata/labels/app","value":"demo"}]`)
	parsed := Patches{}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	require.Equal(t, "add", parsed[0].Op)
	require.Equal(t, "/metadata/labels/app", parsed[0].Path)
	require.Equal(t, "demo", parsed[0].Value)
}

func TestParsePatchYAML(t *testing.T) {
	t.Parallel()

	raw := []byte(`
op: replace
path: /spec/replicas
value: 3
`)
	parsed := new(Patch)
	require.NoError(t, yaml.Unmarshal(raw, parsed))
	require.Equal(t, "replace", parsed.Op)
	require.Equal(t, "/spec/replicas", parsed.Path)
	require.Equal(t, float64(3), parsed.Value)
}

// --- Apply tests -------------------------------------------------------------

func TestApply_AddAnnotationWithEscapedSlashKey(t *testing.T) {
	t.Parallel()

	// JSON Pointer: '/' in a key is escaped as "~1" (RFC 6901).
	// We'll add annotation with key "foo/bar".
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":        "p1",
				"annotations": map[string]interface{}{},
			},
		},
	}

	p := &Patches{
		{
			Op:    "add",
			Path:  "/metadata/annotations/foo~1bar",
			Value: "baz",
		},
	}
	out, err := p.Apply(obj, nil)
	require.NoError(t, err)

	got, found, err := unstructured.NestedString(out.(*unstructured.Unstructured).Object,
		"metadata", "annotations", "foo/bar")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "baz", got)
}

func TestApply_ReplaceReplicas(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "d1"},
			"spec":       map[string]interface{}{"replicas": int64(1)},
		},
	}

	p := &Patches{
		{
			Op:    "replace",
			Path:  "/spec/replicas",
			Value: 3,
		},
	}
	out, err := p.Apply(obj, nil)
	require.NoError(t, err)

	replicas, found, err := unstructured.NestedInt64(out.(*unstructured.Unstructured).Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(3), replicas)
}

func TestApply_RemoveMissingPath_DefaultErrors(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{"name": "svc"},
			"spec":       map[string]interface{}{"selector": map[string]interface{}{"app": "a"}},
		},
	}

	p := &Patches{
		{
			Op:   "remove",
			Path: "/spec/selector/does-not-exist",
		},
	}

	// By default, removing a non-existent path errors per library defaults.
	_, err := p.Apply(obj, nil)
	require.Error(t, err)
}

func TestApply_RemoveMissingPath_AllowedByOptions(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{"name": "svc"},
			"spec":       map[string]interface{}{"selector": map[string]interface{}{"app": "a"}},
		},
	}

	p := &Patches{
		{
			Op:   "remove",
			Path: "/spec/selector/does-not-exist",
		},
	}

	opts := &jsonpatch.ApplyOptions{AllowMissingPathOnRemove: true}
	out, err := p.Apply(obj, opts)
	require.NoError(t, err)
	require.NotNil(t, out) // unchanged object is still valid
}

func TestApply_MoveValue(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "cm"},
			"data": map[string]interface{}{
				"src": "v",
			},
		},
	}

	// Move data.src -> data.dst
	p := &Patches{
		{
			Op:   "move",
			From: "/data/src",
			Path: "/data/dst",
		},
	}
	out, err := p.Apply(obj, nil)
	require.NoError(t, err)

	u := out.(*unstructured.Unstructured)
	// dst present
	dst, ok, err := unstructured.NestedString(u.Object, "data", "dst")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "v", dst)
	// src removed
	_, ok, err = unstructured.NestedString(u.Object, "data", "src")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestApply_AddThenRemove_Label(t *testing.T) {
	t.Parallel()

	// Start with an object lacking labels, add then remove a label.
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": "p2", "labels": map[string]interface{}{}},
		},
	}

	// Add label
	add := &Patches{
		{
			Op:    "add",
			Path:  "/metadata/labels/app",
			Value: "x",
		},
	}
	out, err := add.Apply(obj, nil)
	require.NoError(t, err)

	// Remove it
	rm := &Patches{
		{
			Op:   "remove",
			Path: "/metadata/labels/app",
		},
	}
	out2, err := rm.Apply(out, nil)
	require.NoError(t, err)

	_, ok, err := unstructured.NestedString(out2.(*unstructured.Unstructured).Object, "metadata", "labels", "app")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestApply_InvalidOp(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": "p3"},
		},
	}

	p := &Patches{
		{
			Op:    "invalid",
			Path:  "/metadata/name",
			Value: "x",
		},
	}
	_, err := p.Apply(obj, nil)
	require.Error(t, err)
}
