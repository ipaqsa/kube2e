package filter

import (
	"context"
	"errors"
	"fmt"

	"github.com/itchyny/gojq"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Filter(ctx context.Context, cond string, obj *unstructured.Unstructured) (bool, error) {
	query, err := gojq.Parse(cond)
	if err != nil {
		return false, fmt.Errorf("parse the '%s' condition: %w", cond, err)
	}

	res, ok := query.RunWithContext(ctx, obj.UnstructuredContent()).Next()
	if !ok {
		return false, nil
	}

	if err, ok = res.(error); ok {
		var haltErr *gojq.HaltError
		if errors.As(err, &haltErr) && haltErr.Value() == nil {
			return false, nil
		}

		return false, fmt.Errorf("evaluate the '%s' condition: %w", cond, err)
	}

	pass, ok := res.(bool)
	if !ok {
		return false, fmt.Errorf("the '%s' condition must evaluate to boolean, got %T", cond, res)
	}

	return pass, nil
}
