// Package workerpool provides a simple bounded goroutine pool for concurrent
// processing of homogeneous workloads.
package workerpool

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// Func is the worker function type executed concurrently by Do.
type Func[Object any] func(context.Context, Object) error

// Do executes the provided function concurrently on all objects with controlled parallelism.
// The n parameter controls maximum concurrent goroutines.
func Do[Object any](ctx context.Context, n int, fn Func[Object], objects []Object) error {
	if len(objects) == 0 {
		return nil
	}

	wg, ctx := errgroup.WithContext(ctx)
	wg.SetLimit(n)

	for _, obj := range objects {
		wg.Go(func() error {
			return fn(ctx, obj)
		})
	}

	return wg.Wait()
}
