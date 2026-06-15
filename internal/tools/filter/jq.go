// Package filter evaluates JQ expressions against Kubernetes unstructured objects.
package filter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/itchyny/gojq"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Pass evaluates cond (a JQ expression) against obj and returns true when the
// expression produces a boolean true result. Non-boolean results are an error.
func Pass(ctx context.Context, obj *unstructured.Unstructured, cond string) (bool, error) {
	res, err := exec(ctx, obj, cond)
	if err != nil {
		return false, err
	}

	pass, ok := res.(bool)
	if !ok {
		return false, fmt.Errorf("condition must evaluate to boolean, got %T", res)
	}

	return pass, nil
}

// Filter evaluates exp (a JQ expression) against obj and returns the first
// result as a string. Numbers and booleans are converted via fmt.Sprintf.
func Filter(ctx context.Context, obj *unstructured.Unstructured, exp string) (string, error) {
	res, err := exec(ctx, obj, exp)
	if err != nil {
		return "", err
	}

	if res == nil {
		return "null", nil
	}

	return fmt.Sprintf("%v", res), nil
}

func exec(ctx context.Context, obj *unstructured.Unstructured, exp string) (any, error) {
	query, err := gojq.Parse(exp)
	if err != nil {
		return false, fmt.Errorf("parse the expression: %w", err)
	}

	val, err := normalize(obj.UnstructuredContent())
	if err != nil {
		return false, fmt.Errorf("normalize object: %w", err)
	}

	res, ok := query.RunWithContext(ctx, val).Next()
	if !ok {
		// No output at all (e.g. an empty stream); distinct from a JSON null
		// result, which gojq yields as (nil, true) and is a valid value.
		return false, nil
	}

	if err, ok = res.(error); ok {
		var haltErr *gojq.HaltError
		if errors.As(err, &haltErr) && haltErr.Value() == nil {
			return false, nil
		}

		return false, fmt.Errorf("evaluate the expression: %w", err)
	}

	return res, nil
}

// normalize converts Kubernetes unstructured values into standard JSON types
// so gojq sees the same representation it would get from decoded JSON.
func normalize(val any) (any, error) {
	raw, err := json.Marshal(val)
	if err != nil {
		return nil, fmt.Errorf("marshal value: %w", err)
	}

	var out any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal value: %w", err)
	}

	return out, nil
}
