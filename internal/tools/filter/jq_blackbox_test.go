package filter_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/ipaqsa/kube2e/internal/tools/filter"
)

// TestFilter_NullResult guards against a JSON-null result being reported as a
// spurious "no result" error instead of the string "null".
func TestFilter_NullResult(t *testing.T) {
	t.Parallel()

	obj := new(unstructured.Unstructured)
	obj.SetUnstructuredContent(map[string]any{
		"spec": map[string]any{},
	})

	got, err := filter.Filter(context.Background(), obj, ".spec.paused")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "null" {
		t.Fatalf("got %q, want %q", got, "null")
	}
}
