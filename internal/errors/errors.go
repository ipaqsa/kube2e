// Package errors defines sentinel errors used across the kube2e engine.
package errors

import "errors"

// Sentinel errors returned by engine components. Callers should check with
// errors.Is rather than comparing directly.
var (
	// ErrNilStep is returned when a nil *step.Step is passed to the step runner.
	ErrNilStep = errors.New("step is nil")
	// ErrNilTestCase is returned when a nil *suite.Case is passed to the case runner.
	ErrNilTestCase = errors.New("test case is nil")
	// ErrNilObject is returned when a nil client.Object is passed to a kube operation.
	ErrNilObject = errors.New("object is nil")
	// ErrLogsNotContain is returned when pod logs do not contain the expected string before the timeout.
	ErrLogsNotContain = errors.New("logs do not contain expected string")
	// ErrObjectNoGVK is returned when an object has an empty GroupVersionKind.
	ErrObjectNoGVK = errors.New("object GVK is empty")
	// ErrNilRestConfig is returned when a nil *rest.Config is passed to kube.New.
	ErrNilRestConfig = errors.New("rest config is nil")
	// ErrObjectNotInMap is returned when a step references an object key that is
	// not present in the case-level objects map.
	ErrObjectNotInMap = errors.New("object not found in case objects map")
)
