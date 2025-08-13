// Package errors defines sentinel errors used across the kube2e engine.
package errors

import "errors"

// Sentinel errors returned by engine components. Callers should check with
// errors.Is rather than comparing directly.
var (
	// ErrNilStep is returned when a nil *step.Step is passed to the step runner.
	ErrNilStep = errors.New("step is nil")
	// ErrNilTest is returned when a nil *test.Test is passed to the test runner.
	ErrNilTest = errors.New("test is nil")
	// ErrTestNoName is returned when a test.yaml is missing the required name field.
	ErrTestNoName = errors.New("test has no name")
	// ErrCaseNoName is returned when a case YAML is missing the required name field.
	ErrCaseNoName = errors.New("case has no name")
	// ErrNilTestCase is returned when a nil *testcase.Case is passed to the case runner.
	ErrNilTestCase = errors.New("test case is nil")
	// ErrNilObject is returned when a nil client.Object is passed to a kube operation.
	ErrNilObject = errors.New("object is nil")
	// ErrObjectNoGVK is returned when an object has an empty GroupVersionKind.
	ErrObjectNoGVK = errors.New("object GVK is empty")
	// ErrNilRestConfig is returned when a nil *rest.Config is passed to kube.New.
	ErrNilRestConfig = errors.New("rest config is nil")
	// ErrObjectNotInMap is returned when a step references an object key that is
	// not present in the case-level objects map.
	ErrObjectNotInMap = errors.New("object not found in case objects map")
)
