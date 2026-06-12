// Package step orchestrates action execution within test steps.
package step

import (
	"github.com/ipaqsa/kube2e/internal/engine/action"
)

const (
	stepAnnotation = "testing.kube2e.io/step"
)

// Step defines a single unit of work within a test case. Set one or more
// action fields; nil fields are skipped. Actions execute in a fixed order:
// Ensure → Patch → Wait → Assert → Logs → Exec → Delete.
type Step struct {
	// Name is a required identifier shown in log output.
	Name        string `yaml:"name" json:"name" validate:"required"`
	Description string `yaml:"description" json:"description"`
	// Optional suppresses errors from this step so execution continues.
	Optional bool `yaml:"optional" json:"optional"`

	Ensure *action.Ensure `yaml:"ensure" json:"ensure"`
	Patch  *action.Patch  `yaml:"patch" json:"patch"`
	Wait   *action.Wait   `yaml:"wait" json:"wait"`
	Assert *action.Assert `yaml:"assert" json:"assert"`
	Logs   *action.Logs   `yaml:"logs" json:"logs"`
	Exec   *action.Exec   `yaml:"exec" json:"exec"`
	Delete *action.Delete `yaml:"delete" json:"delete"`
}

// CountActions returns the number of configured actions on the step.
func (s *Step) CountActions() int {
	count := 0

	if s.Ensure != nil {
		count++
	}

	if s.Patch != nil {
		count++
	}

	if s.Wait != nil {
		count++
	}

	if s.Assert != nil {
		count++
	}

	if s.Logs != nil {
		count++
	}

	if s.Exec != nil {
		count++
	}

	if s.Delete != nil {
		count++
	}

	return count
}
