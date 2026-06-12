// Package engine defines shared execution model values for engine reports.
package engine

// State is the normalized execution outcome stored in reports.
type State string

const (
	// StatePassed indicates that execution completed successfully.
	StatePassed State = "passed"
	// StateFailed indicates that execution completed with an error.
	StateFailed State = "failed"
	// StateSkipped indicates that execution was intentionally skipped.
	StateSkipped State = "skipped"
)
